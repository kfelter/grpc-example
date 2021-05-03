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

	"github.com/google/uuid"
	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/grpc"
)

var (
	getMetricTags   = []string{"metric:get"}
	storeMetricTags = []string{"metric:store"}

	port = flag.String("port", "10000", "server port")
)

type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu          sync.Mutex
	muLocal     sync.Mutex
	events      []*pb.Event
	local       []*pb.Event
	defragCount int
	defragLimit int
}

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest, stream pb.EventStore_GetEventsServer) error {
	start := time.Now()
	var err error
	events := s.getEventsWithTags(req.Tags, []string{"deleted:true"})
	end := time.Now()
	s.newGetMetric(start, end)
	for _, event := range events {
		if err = stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

func (s *eventStoreServer) getEventsWithTags(have, haveNone []string) []*pb.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := []*pb.Event{}
	for i, event := range s.events {
		if hasAll(event.Tags, have) && hasNone(event.Tags, haveNone) {
			// check expired
			if expired(event) {
				s.events[i] = &pb.Event{Tags: []string{"deleted:true"}}
				go s.defrag()
			} else {
				events = append(events, event)
			}
		}
	}
	return events
}

func (s *eventStoreServer) defrag() {
	if s.defragCount < s.defragLimit {
		s.defragCount++
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.defragCount == 0 {
		return
	}
	events := []*pb.Event{}
	for _, e := range s.events {
		if notDeleted(e) {
			events = append(events, e)
		}
	}
	if len(s.events) == len(events) {
		fmt.Println("no deleted items to defrag")
		s.defragCount = -s.defragLimit
		return
	}

	prevLen := len(s.events)
	s.events = events
	s.defragCount = 0
	fmt.Println("defragged", prevLen-len(events), len(events))
}

func notDeleted(e *pb.Event) bool {
	for _, t := range e.Tags {
		if t == "deleted:true" {
			return false
		}
	}
	return true
}

func (s *eventStoreServer) getLocalEventsWithTags(have, haveNone []string) []*pb.Event {
	events := []*pb.Event{}
	for i, event := range s.local {
		if hasAll(event.Tags, have) && hasNone(event.Tags, haveNone) {
			// check expired
			if expired(event) {
				s.mu.Lock()
				s.local[i] = &pb.Event{Tags: []string{"deleted:true"}}
				s.mu.Unlock()
			} else {
				events = append(events, event)
			}
		}
	}
	return events
}

func expired(e *pb.Event) bool {
	var ttl time.Duration
	var createdAt time.Time
	hasTTL := false
	for _, t := range e.GetTags() {
		if strings.Contains(t, "ttl:") {
			hasTTL = true
			ttl = getTTL(t)
		}
		if strings.Contains(t, "created_at:") {
			createdAt = getTime(t)
		}
	}
	expired := time.Now().UTC().After(createdAt.Add(ttl))
	return hasTTL && expired
}

func getTTL(t string) time.Duration {
	ss := strings.Split(t, "ttl:")
	if len(ss) < 2 {
		return 0
	}
	dur, err := time.ParseDuration(ss[1])
	if err != nil {
		return 0
	}
	return dur
}

func getTime(t string) time.Time {

	ss := strings.Split(t, "created_at:")
	if len(ss) < 2 {
		return time.Now().UTC()
	}
	tagTime, err := time.Parse(time.RFC3339Nano, ss[1])
	if err != nil {
		return time.Now().UTC()
	}
	return tagTime
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
	for {
		newevent, err := stream.Recv()
		if err == io.EOF {
			end := time.Now()
			s.newStoreMetric(start, end)
			return stream.SendAndClose(&pb.StoreEventsResponse{
				Status: fmt.Sprintf("added events"),
			})
		}
		if err != nil {
			return err
		}
		s.AppendEvent(newevent)
	}
}

func (s *eventStoreServer) AppendEvent(e *pb.Event) *pb.Event {
	s.mu.Lock()
	e.Tags = append(e.GetTags(), getDefaultTags(e.Tags)...)
	s.events = append(s.events, e)
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
	s.AppendLocalEvent(e)
}

func (s *eventStoreServer) newGetMetric(start, end time.Time) {
	e := createMetric(start, end, getMetricTags...)
	s.AppendLocalEvent(e)
}

func (s *eventStoreServer) AppendLocalEvent(e *pb.Event) *pb.Event {
	s.muLocal.Lock()
	e.Tags = append(e.GetTags(), getDefaultTags(e.Tags)...)
	s.local = append(s.local, e)
	s.muLocal.Unlock()
	return e
}

func getDefaultTags(tags []string) []string {
	defaultTags := []string{}
	if getID(tags) == "no id" {
		defaultTags = append(defaultTags, fmt.Sprintf("id:%s", uuid.New().String()))
	}
	if getCreatedAt(tags) == "no date" {
		defaultTags = append(defaultTags, fmt.Sprintf("created_at:%v", time.Now().UTC().Format(time.RFC3339Nano)))
	} else {
		defaultTags = append(defaultTags, fmt.Sprintf("updated_at:%v", time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return defaultTags
}

func getCreatedAt(tags []string) string {
	for _, t := range tags {
		if strings.Contains(t, "created_at:") {
			return t
		}
	}
	return "no date"
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
		LenEvents:           int64(len(s.events)),
		Size:                s.getSize(),
	}
	return res, nil
}

func (s *eventStoreServer) getSize() string {
	ecount := countEventList(s.events)
	lcount := countEventList(s.local)

	return fmt.Sprintf(
		"events=%s local=%s total=%s",
		ByteCountSI(ecount),
		ByteCountSI(lcount),
		ByteCountSI(ecount+lcount),
	)

}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func countEventList(events []*pb.Event) int64 {
	bcount := int64(0)
	for _, e := range events {
		for _, t := range e.GetTags() {
			bcount += int64(len([]byte(t)))
		}
		bcount += int64(len(e.Content))
	}
	return bcount
}

func (s *eventStoreServer) getAvgGetQueryDuration() (string, error) {
	events := s.getLocalEventsWithTags(getMetricTags, nil)
	return getAvgDuration(events)
}

func (s *eventStoreServer) getAvgStoreDuration() (string, error) {
	events := s.getLocalEventsWithTags(storeMetricTags, nil)
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

func newServer() *eventStoreServer {
	s := &eventStoreServer{events: make([]*pb.Event, 0), defragLimit: 1000}
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
