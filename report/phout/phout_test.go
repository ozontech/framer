package phout

import (
	"bytes"
	"errors"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
)

func TestPhout(t *testing.T) {
	t.Parallel()
	a := assert.New(t)
	const timeout = 11 * time.Second

	b := new(bytes.Buffer)
	r := New(b, timeout)
	errChan := make(chan error)
	go func() {
		errChan <- r.Run()
	}()

	var expected string

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("tag1")
		state.SetSize(111)
		state.OnHeader(":status", "200")
		state.OnHeader("grpc-status", "0")

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d	tag1	%d	0	0	0	0	0	111	0	0	grpc_0\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("tag2")
		state.SetSize(222)
		state.OnHeader(":status", "200")
		state.OnHeader("grpc-status", "0")
		state.IoError(fmt.Errorf("read error: %w", syscall.Errno(123)))

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d	tag2	%d	0	0	0	0	0	222	0	123	grpc_0\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("tag2")
		state.SetSize(222)
		state.OnHeader(":status", "200")
		state.OnHeader("grpc-status", "0")
		state.IoError(fmt.Errorf("read error: %w", errors.New("unknown error")))

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d	tag2	%d	0	0	0	0	0	222	0	999	grpc_0\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")
		state.RSTStream(http2.ErrCodeInternal)

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	rst_2\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")
		state.GoAway(http2.ErrCodeInternal)

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	goaway_2\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")

		endTime := startTime.Add(timeout + 1)
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	grpc_4\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")

		endTime := time.Now()
		now = func() time.Time { return endTime }
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	http2_1\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")

		endTime := time.Now()
		now = func() time.Time { return endTime }

		state.OnHeader("grpc-status", "0")
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	http2_1\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	{
		startTime := time.Now()
		now = func() time.Time { return startTime }

		state := r.Acquire("")

		endTime := time.Now()
		now = func() time.Time { return endTime }

		state.OnHeader(":status", "200")
		state.End()

		expected += fmt.Sprintf(
			"%d.%d		%d	0	0	0	0	0	0	0	0	http2_1\n",
			startTime.UnixMilli()/1e3, startTime.UnixMilli()%1e3,
			endTime.Sub(startTime).Microseconds(),
		)
	}

	r.Close()
	a.NoError(<-errChan)

	a.Equal(expected, b.String())
}
