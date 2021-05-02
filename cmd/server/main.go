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

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest, stream pb.EventStore_GetEventsServer) error {
	fmt.Println(req.String())
	// for _, event := range s.events {
	// }
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
			Tags:    append(newevent.GetTags(), fmt.Sprintf("id:%d", s.idCounter)),
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
