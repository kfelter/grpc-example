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

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest) error {
	fmt.Println(req.String())
	return nil
}

func (s *eventStoreServer) StoreEvents(stream pb.EventStore_StoreEventsServer) (*pb.StoreEventsResponse, error) {
	return nil, nil
}

func main() {
	s := &eventStoreServer{}
	fmt.Println(s)
}
