package reflection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
)

type Fetcher interface {
	Fetch(ctx context.Context) (DynamicMessagesStore, error)
}

type ErrFetcher struct {
	err error
}

func NewErrFetcher(err error) *ErrFetcher {
	return &ErrFetcher{err}
}

func (f *ErrFetcher) Fetch(context.Context) (DynamicMessagesStore, error) {
	return nil, f.err
}

type CachedFetcher struct {
	next  Fetcher
	once  *sync.Once
	store DynamicMessagesStore
	err   error
}

func NewCachedFetcher(next Fetcher) *CachedFetcher {
	return &CachedFetcher{next: next, once: new(sync.Once)}
}

func (f *CachedFetcher) Fetch(ctx context.Context) (DynamicMessagesStore, error) {
	f.once.Do(func() {
		f.store, f.err = f.next.Fetch(ctx)
	})
	return f.store, f.err
}

type LocalFetcher struct {
	filenames, importPaths []string
}

func NewLocalFetcher(filenames, importPaths []string) LocalFetcher {
	return LocalFetcher{filenames, importPaths}
}

func (f LocalFetcher) Fetch(context.Context) (DynamicMessagesStore, error) {
	fds, err := protoparse.Parser{
		LookupImport: desc.LoadFileDescriptor,
		ImportPaths:  f.importPaths,
	}.ParseFiles(f.filenames...)
	if err != nil {
		return nil, fmt.Errorf("can't parse proto files: %w", err)
	}

	var methods []*desc.MethodDescriptor
	for _, fs := range fds {
		services := fs.GetServices()
		for _, service := range services {
			methods = append(methods, service.GetMethods()...)
		}
	}
	return NewDynamicMessagesStore(methods), nil
}

type WarnLogger interface {
	Println(string)
}

type RemoteFetcher struct {
	conn    *grpc.ClientConn
	timeout time.Duration
	warns   []string
}

func NewRemoteFetcher(conn *grpc.ClientConn) *RemoteFetcher {
	return &RemoteFetcher{conn, 5 * time.Second, nil}
}

func (f *RemoteFetcher) Warnings() []string {
	return f.warns
}

func (f *RemoteFetcher) Fetch(ctx context.Context) (DynamicMessagesStore, error) {
	refClient := grpcreflect.NewClientAuto(ctx, f.conn)
	listServices, err := refClient.ListServices()
	if err != nil {
		return nil, fmt.Errorf("reflection fetching: %w", err)
	}

	var methods []*desc.MethodDescriptor
	for _, s := range listServices {
		service, err := refClient.ResolveService(s)
		if err != nil {
			f.warns = append(f.warns, "service not found: "+s)
			continue
		}
		methods = append(methods, service.GetMethods()...)
	}

	return NewDynamicMessagesStore(methods), nil
}
