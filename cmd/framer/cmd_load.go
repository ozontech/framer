package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alecthomas/kong"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/ozontech/framer/datasource"
	"github.com/ozontech/framer/loader"
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/report/multi"
	phoutReporter "github.com/ozontech/framer/report/phout"
	supersimpleReporter "github.com/ozontech/framer/report/supersimple"
	"github.com/ozontech/framer/scheduler"
)

type RPSConst struct {
	Freq     uint64        `arg:"" required:"" help:"Value req/s."`
	Duration time.Duration `help:"Limit duration (10s, 2h...)."`
}

func (r RPSConst) AfterApply(kongCtx *kong.Context) error {
	var sched scheduler.Scheduler
	sched, err := scheduler.NewConstant(r.Freq)
	if err != nil {
		return err
	}
	kongCtx.BindTo(sched, (*scheduler.Scheduler)(nil))
	if r.Duration != 0 {
		kongCtx.Bind(DurationLimit{r.Duration})
	}
	return nil
}

type RPSLine struct {
	From     float64       `arg:"" required:"" help:"Starting req/s."`
	To       float64       `arg:"" required:"" help:"Ending req/s."`
	Duration time.Duration `arg:"" required:"" help:"Duration (10s, 2h...)."`
}

func (r RPSLine) AfterApply(kongCtx *kong.Context) error {
	sched := scheduler.NewLine(r.From, r.To, r.Duration)
	kongCtx.BindTo(sched, (*scheduler.Scheduler)(nil))
	if r.Duration != 0 {
		kongCtx.Bind(DurationLimit{r.Duration})
	}
	return nil
}

type RPSUnlimited struct {
	Duration time.Duration `help:"Limit duration (10s, 2h...)."`
	Count    uint64        `help:"Limit requests count"`
}

func (r RPSUnlimited) AfterApply(kongCtx *kong.Context) error {
	var sched scheduler.Scheduler = scheduler.Unlimited{}
	if r.Count != 0 {
		sched = scheduler.NewCountLimiter(sched, int64(r.Count))
	}
	if r.Duration != 0 {
		kongCtx.Bind(DurationLimit{r.Duration})
	}
	kongCtx.BindTo(sched, (*scheduler.Scheduler)(nil))
	return nil
}

type RPS struct {
	Const     RPSConst     `cmd:"" group:"rps" help:"Const rps."`
	Line      RPSLine      `cmd:"" group:"rps" help:"Linear rps."`
	Unlimited RPSUnlimited `cmd:"" group:"rps" help:"Unlimited rps (default one)." default:""`
}

type DurationLimit struct {
	Duration time.Duration
}

type LoadCommand struct {
	Addr          string   `required:"" help:"Address of system under test"`
	RequestsFile  *os.File `required:"" help:"File of requests in ozon.binary format (see convert command to generate)"`
	InmemRequests bool     `help:"Load whole requests file in memory."`

	Clients int    `default:"1" help:"Clients count."`
	Phout   string `help:"Phout report file." type:"path"`

	Verbose bool `help:"Verbose output"`

	RPS
}

func (c *LoadCommand) Run(
	ctx context.Context,
	scheduler scheduler.Scheduler,
	d DurationLimit,
) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		clients = c.Clients
		addr    = c.Addr
	)

	var dataSource types.DataSource = datasource.NewFileDataSource(datasource.NewCyclicReader(c.RequestsFile))
	if c.InmemRequests {
		inmemDS := datasource.NewInmemDataSource(c.RequestsFile)
		err = inmemDS.Init()
		if err != nil {
			return fmt.Errorf("inmem datasource init: %w", err)
		}
		dataSource = inmemDS
	}
	// dataSource := inmem.NewDataSource()

	var creationWG sync.WaitGroup
	creationWG.Add(c.Clients)

	log := zap.NewNop()
	if c.Verbose {
		log = zap.Must(zap.NewDevelopment())
	}

	g, ctx := errgroup.WithContext(ctx)

	loaderConfig := loader.DefaultConfig()

	var reporter types.Reporter = supersimpleReporter.New(loaderConfig.Timeout)
	if c.Phout != "" {
		f, err := os.Create(c.Phout)
		if err != nil {
			return fmt.Errorf("creating phout file(%s): %w", c.Phout, err)
		}
		phoutReporter := phoutReporter.New(f, loaderConfig.Timeout)

		reporter = multi.NewMutli(phoutReporter, reporter)
	}
	g.Go(reporter.Run)

	loaders := make([]*loader.Loader, clients)
	for i := 0; i < clients; i++ {
		conn, err := createConn(ctx, loaderConfig.Timeout, addr)
		// conn, err := createUnixConn(addr)
		if err != nil {
			return fmt.Errorf("dialing: %w", err)
		}
		l, err := loader.NewLoader(
			ctx,
			conn,
			reporter,
			log,
			loader.DefaultConfig(),
		)
		if err != nil {
			return fmt.Errorf("loader setup: %w", err)
		}
		loaders[i] = l
	}

	var lgWG sync.WaitGroup
	lgWG.Add(len(loaders))
	var n atomic.Int64
	begin := time.Now()
	for i := range loaders {
		i := i
		l := loaders[i]

		lgCtx, lgCancel := context.WithCancel(ctx)
		g.Go(func() error {
			return l.Run(lgCtx)
		})

		g.Go(func() error {
			defer lgWG.Done()
			defer lgCancel()

			r, err := dataSource.Fetch()
			if err != nil {
				return err
			}

			for {
				at, ok := scheduler.Next(n.Add(1))
				if !ok || at > d.Duration {
					break
				}
				time.Sleep(at - time.Since(begin))
				l.DoRequest(r)

				r, err = dataSource.Fetch()
				if err != nil {
					return err
				}
			}
			l.Flush(ctx)
			l.WaitResponses(ctx)
			return nil
		})
	}

	lgWG.Wait()
	cancel()

	err = reporter.Close()
	if err != nil {
		return fmt.Errorf("close multireporter: %w", err)
	}

	defer memStats(log)

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func memStats(log *zap.Logger) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Info(
		"memory stats",
		zap.Uint64("Alloc (MiB)", bToMb(m.Alloc)),
		zap.Uint64("TotalAlloc (MiB)", bToMb(m.TotalAlloc)),
		zap.Uint64("Sys (MiB)", bToMb(m.Sys)),

		zap.Uint64("HeapSys (MiB)", bToMb(m.HeapSys)),
		zap.Uint64("HeapIdle (MiB)", bToMb(m.HeapIdle)),
		zap.Uint64("HeapInuse (MiB)", bToMb(m.HeapInuse)),
		zap.Uint64("HeapReleased (MiB)", bToMb(m.HeapReleased)),
		zap.Uint64("HeapObjects (MiB)", bToMb(m.HeapObjects)),

		zap.Uint64("StackSys (MiB)", bToMb(m.StackSys)),
		zap.Uint64("StackInuse (MiB)", bToMb(m.StackInuse)),

		zap.Uint32("NumGC (count)", m.NumGC),
		zap.Uint64("Mallocs (count)", m.Mallocs),
		zap.Uint64("Frees (count)", m.Frees),
	)
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func createConn(ctx context.Context, timeout time.Duration, addr string) (net.Conn, error) {
	dialer := net.Dialer{}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	tcpConn := conn.(*net.TCPConn)
	// tcpConn.SetKeepAlive(false)
	// tcpConn.SetNoDelay(false)
	// tcpConn.SetReadBuffer(6291456)
	// tcpConn.SetWriteBuffer(6291456)
	return tcpConn, nil
}

//nolint:unused
func createUnixConn(addr string) (net.Conn, error) {
	raddr, err := net.ResolveUnixAddr("unix", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUnix("unix", nil, raddr)
	if err != nil {
		return nil, err
	}
	// return conn, nil
	return conn, multierr.Combine(
		conn.SetWriteBuffer(1024*1024*10),
		conn.SetReadBuffer(1024*1024*10),
	)
}
