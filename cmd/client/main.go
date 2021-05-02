package main

import (
	"context"
	"flag"
	"fmt"

	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/grpc"
)

var (
	serverAddr = flag.String("server-addr", "localhost:10000", "the address of the server host:port")
)

func main() {
	flag.Parse()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := pb.NewEventStoreClient(conn)
	stream, err := client.StoreEvents(context.Background())
	if err != nil {
		panic(err)
	}
	err = stream.Send(&pb.Event{
		Content: []byte(`new event`),
		Tags:    []string{"environment:test"},
	})
	if err != nil {
		panic(err)
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		panic(err)
	}

	fmt.Println(reply.String())
}
