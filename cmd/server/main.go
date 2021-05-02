package main

import (
	"fmt"
	"io"
	"sync"

	pb "github.com/kfelter/grpc-example/eventstore"
)

type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu        sync.Mutex
	events    []*pb.Event
	idCounter int64
}

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest) error {
	fmt.Println(req.String())
	return nil
}

func (s *eventStoreServer) StoreEvents(stream pb.EventStore_StoreEventsServer) (*pb.StoreEventsResponse, error) {
	events := []*pb.Event{}
	for {
		newevent, err := stream.Recv()
		if err == io.EOF {
			return &pb.StoreEventsResponse{
				Events: events,
				Status: fmt.Sprintf("added %d events", len(events)),
			}, nil
		}
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		event := &pb.Event{
			Id:      s.idCounter,
			Tags:    newevent.GetTags(),
			Content: newevent.GetContent(),
		}
		events = append(events, event)
		s.events = append(s.events, event)
		s.idCounter++
		s.mu.Unlock()
	}
}

func main() {
	s := &eventStoreServer{}
	fmt.Println(s)
}
