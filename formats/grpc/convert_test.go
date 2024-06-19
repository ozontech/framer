package grpc_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ozontech/framer/formats/converter"
	formatsGRPC "github.com/ozontech/framer/formats/grpc"
	ozonBinary "github.com/ozontech/framer/formats/grpc/ozon/binary"
	ozonJson "github.com/ozontech/framer/formats/grpc/ozon/json"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	pandoryJson "github.com/ozontech/framer/formats/grpc/pandora/json"
	model "github.com/ozontech/framer/formats/model"
)

func testGRPCConvert(
	ctx context.Context,
	t *testing.T,
	pathFrom, pathTo string,
	from grpcIN, to grpcOUT,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	f, err := os.Open(filepath.Join("./test_files/", filepath.Clean(pathFrom)))
	if err != nil {
		t.Fatal(err)
	}

	outBuf := new(bytes.Buffer)

	processor := converter.NewProcessor(formatsGRPC.NewConvertStrategy(
		from.Factory(f),
		to.Factory(outBuf),
	))

	r := require.New(t)
	r.NoError(processor.Process(ctx))

	expectedB, err := os.ReadFile(filepath.Join("./test_files/", filepath.Clean(pathTo)))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, string(expectedB), outBuf.String())
}

func TestGRPCConverts(t *testing.T) {
	reflection := makeReflection(t)

	t.Parallel()

	inFormats := []grpcIN{
		{
			Ext: "ozon.json",
			Factory: func(r io.Reader) *model.InputFormat {
				return ozonJson.NewInput(r, reflection)
			},
		},
		{
			Ext: "pandora.json",
			Factory: func(r io.Reader) *model.InputFormat {
				return pandoryJson.NewInput(r, reflection)
			},
		},
		{
			Ext:     "ozon.binary",
			Factory: ozonBinary.NewInput,
		},
	}
	outFormats := []grpcOUT{
		{
			Ext: "ozon.json",
			Factory: func(w io.Writer) *model.OutputFormat {
				return ozonJson.NewOutput(w, reflection)
			},
		},
		{
			Ext:     "ozon.binary",
			Factory: ozonBinary.NewOutput,
		},
	}

	for _, inFormat := range inFormats {
		for _, outFormat := range outFormats {
			for _, fileName := range []string{"requests"} {
				inFormat := inFormat
				outFormat := outFormat

				t.Run(fmt.Sprintf("convert_%s_2_%s(%s)", inFormat.Ext, outFormat.Ext, fileName), func(t *testing.T) {
					var (
						inFile  = fmt.Sprintf("%s.%s", fileName, inFormat.Ext)
						outFile = fmt.Sprintf("%s.%s", fileName, outFormat.Ext)
					)
					t.Parallel()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()
					testGRPCConvert(ctx, t, inFile, outFile, inFormat, outFormat)
				})
			}
		}
	}
}

type grpcIN struct {
	Ext     string
	Factory func(io.Reader) *model.InputFormat
}

type grpcOUT struct {
	Ext     string
	Factory func(io.Writer) *model.OutputFormat
}

func makeReflection(t *testing.T) reflection.DynamicMessagesStore {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	store, err := reflection.NewLocalFetcher([]string{"./ozon/json/encoding/testproto/service.proto"}, nil).Fetch(ctx)
	if err != nil {
		t.Fatalf("can't get reflection: %s", err)
	}

	return store
}
