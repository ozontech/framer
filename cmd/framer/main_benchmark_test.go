package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/datasource"
	"github.com/ozontech/framer/loader"
	"github.com/ozontech/framer/report/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
)

// func BenchmarkDataSource(b *testing.B) {
// 	f, err := os.Open("../test_files/requests")
// 	if err != nil {
// 		b.Fatal(err)
// 	}
//
// 	if false {
// 		go func() {
// 			log.Println(http.ListenAndServe("localhost:6060", nil))
// 		}()
// 	}
//
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()
//
// 	b.ResetTimer()
// 	err = (&LoadCommand{
// 		Clients:       1,
// 		RequestsCount: b.N,
// 		RequestsFile:  f,
// 		Addr:          "localhost:9090",
// 	}).Run(ctx)
// 	if err != nil {
// 		b.Fatal(err)
// 	}
// }

func BenchmarkE2E(b *testing.B) {
	f, err := os.Open("../../test_files/requests")
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := createConn(ctx, consts.DefaultTimeout, "localhost:9090")
	if err != nil {
		b.Fatal(err)
	}
	reporter := simple.New()
	a := assert.New(b)
	go func() { a.NoError(reporter.Run()) }()
	defer func() { a.NoError(reporter.Close()) }()

	l, err := loader.NewLoader(
		conn,
		reporter,
		consts.DefaultTimeout,
		false,
		zaptest.NewLogger(b),
	)
	if err != nil {
		b.Fatal(fmt.Errorf("loader setup: %w", err))
	}

	b.ResetTimer()

	g, ctx := errgroup.WithContext(ctx)
	runCtx, runCancel := context.WithCancel(ctx)
	g.Go(func() error {
		return l.Run(runCtx)
	})

	dataSource := datasource.NewFileDataSource(datasource.NewCyclicReader(f))
	g.Go(func() error {
		r, err := dataSource.Fetch()
		if err != nil {
			return err
		}
		l.DoRequest(r)
		b.ResetTimer()

		r, err = dataSource.Fetch()
		if err != nil {
			return err
		}

		for i := 0; i < b.N; i++ {
			l.DoRequest(r)

			r, err = dataSource.Fetch()
			if err != nil {
				return err
			}
		}
		l.WaitResponses(ctx)
		runCancel()
		return nil
	})

	err = g.Wait()
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkE2EInMemDatasource(b *testing.B) {
	a := assert.New(b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := createConn(ctx, 5*time.Second, "localhost:9090")
	a.NoError(err)
	reporter := simple.New()

	reportErr := make(chan error)
	go func() { reportErr <- reporter.Run() }()

	l, err := loader.NewLoader(
		conn,
		reporter,
		consts.DefaultTimeout,
		false,
		zaptest.NewLogger(b),
	)
	a.NoError(err)

	b.ResetTimer()

	g, ctx := errgroup.WithContext(ctx)
	runCtx, runCancel := context.WithCancel(ctx)
	g.Go(func() error {
		return l.Run(runCtx)
	})

	requestsFile, err := os.Open("../test_files/requests")
	if err != nil {
		require.NoError(b, err)
	}
	dataSource := datasource.NewInmemDataSource(requestsFile)
	require.NoError(b, dataSource.Init())
	g.Go(func() error {
		r, err := dataSource.Fetch()
		if err != nil {
			return err
		}
		l.DoRequest(r)
		b.ResetTimer()

		r, err = dataSource.Fetch()
		if err != nil {
			return err
		}

		for i := 0; i < b.N; i++ {
			l.DoRequest(r)

			r, err = dataSource.Fetch()
			if err != nil {
				return err
			}
		}
		l.WaitResponses(ctx)
		runCancel()
		return nil
	})

	a.NoError(g.Wait())
	a.NoError(reporter.Close())
	a.NoError(<-reportErr)
}
