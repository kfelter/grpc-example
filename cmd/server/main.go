package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/kfelter/grpc-example/eventstore"
	"github.com/kfelter/grpc-example/internal/metric"
	"github.com/kfelter/grpc-example/internal/tag"
	"google.golang.org/grpc"
)

var (
	port = flag.String("port", "10000", "server port")
)

type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu          sync.Mutex
	muLocal     sync.Mutex
	muChat      sync.Mutex
	events      []*pb.Event
	local       []*pb.Event
	chat        map[string][]*pb.Event
	defragDelay time.Duration
}

func (s *eventStoreServer) Join(stream pb.EventStore_JoinServer) error {
	e, err := stream.Recv()
	if err != nil {
		return err
	}
	channel := tag.GetChan(e.GetTags())
	var userIDTag string
	if uid, found := tag.GetUserID(e.GetTags()); found {
		userIDTag = uid
	} else {
		userIDTag = fmt.Sprintf("user_id:%s", uuid.New().String())
	}

	cur, err := s.SendChatHist(stream, channel)
	if err != nil {
		return err
	}

	errChan := make(chan error)

	go func() {
		for {
			e, err := stream.Recv()
			if err != nil {
				errChan <- err
				return
			}
			fmt.Println("recv", e.String())
			s.RecordChat(channel, userIDTag, e)
		}
	}()

	go s.SendChatUpdates(stream, channel, userIDTag, cur, errChan)
	err = <-errChan
	if err == io.EOF {
		return nil
	}
	return err
}

func (s *eventStoreServer) SendChatUpdates(stream pb.EventStore_JoinServer, channel, userIDTag string, cur int, errChan chan error) {
	var err error
	ticker := time.NewTicker(1 * time.Second)
	for {
		<-ticker.C
		s.muChat.Lock()
		chatMessages := s.chat[channel]
		s.muChat.Unlock()

		for ; cur < len(chatMessages); cur++ {
			if err = stream.Send(chatMessages[cur]); err != nil {
				errChan <- err
			}
		}
	}
}

func (s *eventStoreServer) SendChatHist(stream pb.EventStore_JoinServer, channel string) (int, error) {
	cur := 0
	s.muChat.Lock()
	chatMessages := s.chat[channel]
	s.muChat.Unlock()
	for ; cur < len(chatMessages); cur++ {
		if err := stream.Send(chatMessages[cur]); err != nil {
			return 0, err
		}
	}
	return cur, nil
}

func (s *eventStoreServer) RecordChat(channel, userIDTag string, e *pb.Event) {
	e.Tags = append(e.GetTags(), getDefaultTags(e.GetTags(), userIDTag)...)
	s.muChat.Lock()
	defer s.muChat.Unlock()
	s.chat[channel] = append(s.chat[channel], e)
}

func (s *eventStoreServer) getLastID() string {
	e := s.getLastEvent()
	curID, _ := tag.GetID(e.GetTags())
	return curID
}

func (s *eventStoreServer) getIndex(id string) int {
	for i, e := range s.events {
		if v, _ := tag.GetID(e.GetTags()); v == id {
			return i
		}
	}
	return 0
}

func (s *eventStoreServer) getLastEvent() *pb.Event {
	if len(s.events) > 0 {
		return s.events[len(s.events)-1]
	}
	return &pb.Event{}
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
		if tag.Q(event.GetTags(), have, haveNone) {
			// check expired
			if expired(event) {
				s.events[i] = &pb.Event{Tags: []string{tag.DeletedTag}}
			} else {
				events = append(events, event)
			}
		}
	}
	return events
}

func (s *eventStoreServer) defrag() {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := []*pb.Event{}
	for _, e := range s.events {
		if notDeleted(e) {
			events = append(events, e)
		}
	}
	if len(s.events) == len(events) {
		fmt.Println("no deleted items to defrag")
		return
	}

	prevLen := len(s.events)
	s.events = events
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
		if tag.Q(event.GetTags(), have, haveNone) {
			// check expired
			if expired(event) {
				s.mu.Lock()
				s.local[i] = &pb.Event{Tags: []string{tag.DeletedTag}}
				s.mu.Unlock()
			} else {
				events = append(events, event)
			}
		}
	}
	return events
}

func expired(e *pb.Event) bool {
	tags := e.GetTags()
	ttl, hasTTL := tag.GetTTL(tags)
	createdAt, _ := tag.GetCreatedAt(tags)
	expired := time.Now().UTC().After(createdAt.Add(ttl))
	return hasTTL && expired
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

func (s *eventStoreServer) AppendEvent(e *pb.Event, tags ...string) *pb.Event {
	if e == nil {
		return &pb.Event{}
	}
	e.Tags = append(e.GetTags(), getDefaultTags(e.Tags)...)
	e.Tags = append(e.GetTags(), tags...)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return e
}

func (s *eventStoreServer) newStoreMetric(start, end time.Time) {
	e := createMetric(start, end, tag.StoreMetricTag)
	s.AppendLocalEvent(e)
}

func (s *eventStoreServer) newGetMetric(start, end time.Time) {
	e := createMetric(start, end, tag.GetMetricTag)
	s.AppendLocalEvent(e)
}

func (s *eventStoreServer) AppendLocalEvent(e *pb.Event) *pb.Event {
	s.muLocal.Lock()
	e.Tags = append(e.GetTags(), getDefaultTags(e.Tags)...)
	s.local = append(s.local, e)
	s.muLocal.Unlock()
	return e
}

func getDefaultTags(tags []string, extra ...string) []string {
	defaultTags := []string{}
	if _, found := tag.GetID(tags); !found {
		defaultTags = append(defaultTags, tag.NewID())
	}
	if _, found := tag.GetCreatedAt(tags); !found {
		defaultTags = append(defaultTags, tag.NewCreatedAtTag(time.Now()))
	} else {
		defaultTags = append(defaultTags, tag.NewUpdatedAtTag(time.Now()))
	}
	return append(defaultTags, extra...)
}

func createMetric(start, end time.Time, tags ...string) *pb.Event {
	return &pb.Event{
		Tags:    tags,
		Content: metric.Metric{start, end, end.Sub(start)}.Buf(),
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
	events := s.getLocalEventsWithTags([]string{tag.GetMetricTag}, nil)
	return getAvgDuration(events)
}

func (s *eventStoreServer) getAvgStoreDuration() (string, error) {
	events := s.getLocalEventsWithTags([]string{tag.StoreMetricTag}, nil)
	return getAvgDuration(events)
}

func getAvgDuration(events []*pb.Event) (string, error) {
	if len(events) == 0 {
		return "no metrics yet", nil
	}
	var err error
	totalSeconds := float64(0)
	for _, e := range events {
		m := &metric.Metric{}
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

func (s *eventStoreServer) startDefragger() {
	ticker := time.NewTicker(s.defragDelay)
	for {
		<-ticker.C
		s.defrag()
	}
}

func newServer() *eventStoreServer {
	s := &eventStoreServer{
		events:      make([]*pb.Event, 0),
		defragDelay: 10 * time.Minute,
		local:       make([]*pb.Event, 0),
		chat:        make(map[string][]*pb.Event, 0),
	}
	go s.startDefragger()
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
