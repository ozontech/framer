package main

import (
	"context"
	"math"
	"net/http"
	_ "net/http/pprof" //nolint:gosec

	"github.com/alecthomas/kong"
	mangokong "github.com/alecthomas/mango-kong"
)

var CLI struct {
	Load        LoadCommand       `cmd:"" help:"Starting load generation."`
	Convert     ConvertCommand    `cmd:"" help:"Converting request files."`
	Man         mangokong.ManFlag `help:"Write man page." hidden:""`
	DebugServer bool              `help:"Enable debug server."`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// runtime.SetBlockProfileRate(1)
		http.ListenAndServe(":8081", nil) //nolint:errcheck,gosec
	}()

	// sigs := make(chan os.Signal, 1)
	// signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// go func() {
	// 	<-sigs
	// 	cancel()
	// }()

	kongCtx := kong.Parse(
		&CLI,
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(DurationLimit{Duration: math.MaxInt64}),
		kong.Groups(map[string]string{
			"reflection": `Reflection flags:`,
		}),
		kong.ConfigureHelp(kong.HelpOptions{
			Tree:    true,
			Compact: true,
		}),
		kong.Description(`the most performant grpc load generator

The framer is used to generate test requests to grpc servers and measure codes and response times in a most effective way.
		`),
	)
	err := kongCtx.Run()
	kongCtx.FatalIfErrorf(err)
}
