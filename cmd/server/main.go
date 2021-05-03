package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/grpc"
)

var (
	getMetricTags   = []string{"internal:true", "metric:get"}
	storeMetricTags = []string{"internal:true", "metric:store"}
	internalTags    = []string{"internal:true"}
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
	// exit early if there is an id tag
	for _, t := range req.Tags {
		if strings.Contains(t, "id:") {
			return stream.Send(s.getEventWithIDTag(t))
		}
	}

	// if no id tag, run query with all tags
	events := s.getEventsWithTags(req.Tags, internalTags)
	end := time.Now()
	s.newGetMetric(start, end)
	for _, event := range events {
		if err = stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

func (s *eventStoreServer) getEventWithIDTag(t string) *pb.Event {
	var ss []string
	if ss = strings.Split(t, ":"); len(ss) < 2 {
		return &pb.Event{}
	}

	id, err := strconv.Atoi(ss[1])
	if err != nil {
		fmt.Println(err)
		return &pb.Event{}
	}
	if s.idCounter < int64(id) {
		fmt.Println("invalid id")
		return &pb.Event{}
	}
	if hasNone(s.events[id].GetTags(), internalTags) {
		return s.events[id]
	}

	return &pb.Event{}
}

func (s *eventStoreServer) getEventsWithTags(have, haveNone []string) []*pb.Event {
	events := []*pb.Event{}
	for _, event := range s.events {
		if hasAll(event.Tags, have) && hasNone(event.Tags, haveNone) {
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

func hasNone(has, mustNotHave []string) bool {
	noMap := map[string]int{}
	for _, s := range mustNotHave {
		noMap[s] = 1
	}

	for _, s := range has {
		if noMap[s] > 0 {
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
		e := s.AppendEvent(newevent)
		events = append(events, e)
	}
}

func (s *eventStoreServer) AppendEvent(e *pb.Event) *pb.Event {
	s.mu.Lock()
	e.Tags = append(e.GetTags(), fmt.Sprintf("id:%d", s.idCounter))
	s.events = append(s.events, e)
	s.idCounter++
	s.mu.Unlock()
	return e
}

type Metric struct {
	Start time.Time     `json:"start"`
	End   time.Time     `json:"end"`
	Dur   time.Duration `json:"duration"`
}

func (s *eventStoreServer) newStoreMetric(start, end time.Time) {
	e := createMetric(start, end, storeMetricTags...)
	s.AppendEvent(e)
}

func (s *eventStoreServer) newGetMetric(start, end time.Time) {
	e := createMetric(start, end, getMetricTags...)
	s.AppendEvent(e)
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
	events := s.getEventsWithTags(getMetricTags, nil)
	return getAvgDuration(events)
}

func (s *eventStoreServer) getAvgStoreDuration() (string, error) {
	events := s.getEventsWithTags(storeMetricTags, nil)
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
