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

var (
	getMetricTags   = []string{"internal:true", "metric:get"}
	storeMetricTags = []string{"internal:true", "metric:store"}
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
	events := s.getEventsWithTags(req.Tags)
	end := time.Now()
	s.newGetMetric(start, end)
	for _, event := range events {
		fmt.Println("matched", req.Tags, "sending event", getID(event.GetTags()))
		if err = stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

func (s *eventStoreServer) getEventsWithTags(t []string) []*pb.Event {
	events := []*pb.Event{}
	for _, event := range s.events {
		if hasAll(event.Tags, t) {
			events = append(events, event)
		}
	}
	return events
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
	e := createMetric(start, end, storeMetricTags...)
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
}

func (s *eventStoreServer) newGetMetric(start, end time.Time) {
	e := createMetric(start, end, getMetricTags...)
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
	avgGet, err := s.getAvgGetQueryDuration()
	if err != nil {
		return nil, err
	}

	avgStore, err := s.getAvgStoreDuration()
	if err != nil {
		return nil, err
	}
	res := &pb.ServerMetricsResponse{
		Status:              "OK",
		AvgGetQueryDuration: avgGet,
		AvgStoreDuration:    avgStore,
		LenEvents:           s.idCounter,
	}
	return res, nil
}

func (s *eventStoreServer) getAvgGetQueryDuration() (string, error) {
	events := s.getEventsWithTags(getMetricTags)
	return getAvgDuration(events)
}

func (s *eventStoreServer) getAvgStoreDuration() (string, error) {
	events := s.getEventsWithTags(storeMetricTags)
	return getAvgDuration(events)
}

func getAvgDuration(events []*pb.Event) (string, error) {
	if len(events) == 0 {
		return "no metrics yet", nil
	}
	var err error
	totalSeconds := float64(0)
	for _, e := range events {
		m := &Metric{}
		if err = json.Unmarshal(e.Content, m); err != nil {
			return "", err
		}
		totalSeconds += m.Dur.Seconds()
	}

	avg := totalSeconds / float64(len(events))
	avgDuration, err := time.ParseDuration(fmt.Sprintf("%.10fs", avg))
	if err != nil {
		return "", err
	}
	return avgDuration.String(), nil
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
