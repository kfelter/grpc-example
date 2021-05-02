package main

import (
	"fmt"
	"sync"

	pb "github.com/kfelter/grpc-example/eventstore"
)

type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu     sync.Mutex
	events []*pb.Event
}

func main() {
	s := &eventStoreServer{}
	fmt.Println(s)
}
