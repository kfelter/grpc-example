package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/grpc"
)

type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu        sync.Mutex
	events    []*pb.Event
	idCounter int64
}

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest, stream pb.EventStore_GetEventsServer) error {
	start := time.Now()
	var err error
	for _, event := range s.events {
		if hasAll(event.Tags, req.Tags) {
			fmt.Println("matched", req.Tags, "sending event", getID(event.GetTags()))
			if err = stream.Send(event); err != nil {
				return err
			}
		}
	}
	end := time.Now()
	s.newGetMetric(start, end)
	return nil
}

func hasAll(has, requested []string) bool {
	reqMap := map[string]int{}
	for _, s := range requested {
		reqMap[s] = 1
	}
	for _, s := range has {
		reqMap[s] = 0
	}

	for _, count := range reqMap {
		if count > 0 {
			return false
		}
	}
	return true
}

func getID(tags []string) string {
	for _, t := range tags {
		if strings.Contains(t, "id:") {
			return t
		}
	}
	return "no id"
}

func (s *eventStoreServer) StoreEvents(stream pb.EventStore_StoreEventsServer) error {
	start := time.Now()
	events := []*pb.Event{}
	for {
		newevent, err := stream.Recv()
		if err == io.EOF {
			end := time.Now()
			s.newStoreMetric(start, end)
			return stream.SendAndClose(&pb.StoreEventsResponse{
				Events: events,
				Status: fmt.Sprintf("added %d events", len(events)),
			})
		}
		if err != nil {
			return err
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

type Metric struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Dur   time.Duration `json:"duration"`
}

func (s *eventStoreServer) newStoreMetric(start, end time.Time) {
	e := createMetric(start, end, "internal:true", "metric:store")
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func (s *eventStoreServer) newGetMetric(start, end time.Time) {
	e := createMetric(start, end, "internal:true", "metric:get")
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func createMetric(start, end time.Time, tags ...string) *pb.Event {
	b, _ := json.Marshal(Metric{start, end, end.Sub(start)})
	return &pb.Event{
		Tags:    tags,
		Content: b,
	}
}

func (s *eventStoreServer) ServerMetrics(c context.Context, req *pb.ServerMestricsRequest) (*pb.ServerMetricsResponse, error) {
	res := &pb.ServerMetricsResponse{
		Status:              "OK",
		AvgGetQueryDuration: s.getAvgGetQueryDuration(),
		AvgStoreDuration:    s.getAvgStoreDuration(),
		LenEvents:           s.idCounter,
	}
	return res, nil
}

func (s *eventStoreServer) getAvgGetQueryDuration() string {
	return ""
}

func (s *eventStoreServer) getAvgStoreDuration() string {
	return ""
}

var (
	port = flag.String("port", "10000", "server port")
)

func newServer() *eventStoreServer {
	s := &eventStoreServer{events: make([]*pb.Event, 0)}
	return s
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterEventStoreServer(grpcServer, newServer())
	grpcServer.Serve(lis)
}
