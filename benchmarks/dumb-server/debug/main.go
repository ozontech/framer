package main

import (
	"os"

	"golang.org/x/net/http2"
)

func main() {
	f, _ := os.Open("/tmp/debug")
	framer := http2.NewFramer(nil, f)
	for {
		fr, err := framer.ReadFrame()
		if err != nil {
			panic(err)
		}
		println(fr.Header().String())
	}
}

// func main2() {
// 	conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	client := pb.NewTestApiClient(conn)
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
// 	defer cancel()
//
// 	for i := 0; i < 10_000; i++ {
// 		_, err := client.Test(ctx, &pb.TestRequest{})
// 		if err != nil {
// 			panic(err)
// 		}
// 	}
// }
