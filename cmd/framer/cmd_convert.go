package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ozontech/framer/formats/converter"
	formatsGRPC "github.com/ozontech/framer/formats/grpc"
	ozonBinary "github.com/ozontech/framer/formats/grpc/ozon/binary"
	ozonJson "github.com/ozontech/framer/formats/grpc/ozon/json"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	pandoryJson "github.com/ozontech/framer/formats/grpc/pandora/json"
	"github.com/ozontech/framer/formats/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type format string

const (
	formatOzonJSON    format = "ozon.json"
	formatOzonBinary  format = "ozon.binary"
	formatPandoraJSON format = "pandora.json"
)

type ConvertCommand struct {
	In  *os.File `arg:"" required:"" default:"-" help:"Input file (default is stdin)"`
	Out string   `arg:"" required:"" default:"-" help:"Input file (default is stdout)" type:"path"`

	From format `enum:"ozon.json, ozon.binary, pandora.json" required:"" placeholder:"ozon/json"   help:"Input format.  Available types: ${enum}"`
	To   format `enum:"ozon.json, ozon.binary" required:"" placeholder:"ozon/binary" help:"Output format. Available types: ${enum}"`

	ReflectionAddr       string   `group:"reflection" xor:"reflection" placeholder:"my-service:9090" help:"Address of reflection api"`
	ReflectionProto      []string `group:"reflection" xor:"reflection" placeholder:"service1.proto,service2.proto" help:"Proto files"`
	ReflectionImportPath []string `group:"reflection" type:"existingdir" placeholder:"./api/,./vendor/" help:"Proto import paths"`
}

func (c *ConvertCommand) Validate() error {
	if c.From == c.To {
		return errors.New("--from and --to flags must have different values")
	}
	hasReflect := len(c.ReflectionProto) != 0 || c.ReflectionAddr != ""
	if (c.From != formatOzonBinary || c.To != formatOzonBinary) && !hasReflect {
		return errors.New("--reflection-addr or --reflection-proto is required for this conversion type")
	}
	return nil
}

func (c *ConvertCommand) Run(ctx context.Context) error {
	var outF *os.File
	if c.Out == "-" {
		outF = os.Stdout
	} else {
		var err error
		outF, err = os.Create(c.Out)
		if err != nil {
			return fmt.Errorf("output file creation: %w", err)
		}
	}

	var r8n reflection.DynamicMessagesStore
	if c.ReflectionAddr != "" {
		conn, err := grpc.Dial(
			c.ReflectionAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUserAgent("framer"),
		)
		if err != nil {
			return fmt.Errorf("create reflection conn: %w", err)
		}

		fetchCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		fetcher := reflection.NewRemoteFetcher(conn)
		r8n, err = fetcher.Fetch(fetchCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("remote reflection fetching: %w", err)
		}
		for _, warn := range fetcher.Warnings() {
			log.Println("remote reflection fetching: ", warn)
		}
	} else {
		fetchCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		var err error
		r8n, err = reflection.NewLocalFetcher(c.ReflectionProto, c.ReflectionImportPath).Fetch(fetchCtx)
		if err != nil {
			return fmt.Errorf("local reflection fetching: %w", err)
		}
	}

	var inputFormat *model.InputFormat
	switch c.From {
	case formatOzonBinary:
		inputFormat = ozonBinary.NewInput(c.In)
	case formatOzonJSON:
		inputFormat = ozonJson.NewInput(c.In, r8n)
	case formatPandoraJSON:
		inputFormat = pandoryJson.NewInput(c.In, r8n)
	default:
		panic("assertion error")
	}

	var outputFormat *model.OutputFormat
	switch c.To {
	case formatOzonBinary:
		outputFormat = ozonBinary.NewOutput(outF)
	case formatOzonJSON:
		outputFormat = ozonJson.NewOutput(outF, r8n)
	default:
		panic("assertion error")
	}

	return converter.NewProcessor(formatsGRPC.NewConvertStrategy(
		inputFormat,
		outputFormat,
	)).Process(ctx)
}
