package datasource

import (
	"io"
	"os"
	"testing"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/loader/types"
	hpackwrapper "github.com/ozontech/framer/utils/hpack_wrapper"
)

type noopHpackFieldWriter struct{}

func (*noopHpackFieldWriter) WriteField(string, string) {}
func (*noopHpackFieldWriter) SetWriter(io.Writer)       {}

func BenchmarkFileDataSource(b *testing.B) {
	f, err := os.Open("../test_files/requests")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	ds := NewFileDataSource(NewCyclicReader(f))

	rr := make(chan types.Req, 100)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for r := range rr {
			_, err := r.SetUp(consts.DefaultMaxFrameSize, consts.DefaultMaxHeaderListSize, 0, &noopHpackFieldWriter{})
			if err != nil {
				b.Error(err)
				return
			}
			b.SetBytes(int64(r.Size()))
			r.Release()
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := ds.Fetch()
		if err != nil {
			b.Fatal(err)
		}
		rr <- r
	}
	close(rr)
	<-done
}

func BenchmarkRequestSetupNoop(b *testing.B) {
	f, err := os.Open("../test_files/requests")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	ds := NewFileDataSource(f)

	r, err := ds.Fetch()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.SetUp(consts.DefaultMaxFrameSize, consts.DefaultMaxHeaderListSize, 0, &noopHpackFieldWriter{})
		if err != nil {
			b.Error(err)
			return
		}
		b.SetBytes(int64(r.Size()))
	}
}

func BenchmarkRequestSetupHpack(b *testing.B) {
	f, err := os.Open("../test_files/requests")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	ds := NewFileDataSource(f)

	r, err := ds.Fetch()
	if err != nil {
		b.Fatal(err)
	}

	hpackwrapper := hpackwrapper.NewWrapper()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.SetUp(consts.DefaultMaxFrameSize, consts.DefaultMaxHeaderListSize, 0, hpackwrapper)
		if err != nil {
			b.Error(err)
			return
		}
		b.SetBytes(int64(r.Size()))
	}
}
