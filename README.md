# grpc-example using golang
simple grpc client and server using golang

# Summary
gRPC - Remote Proceedure Call

In this project I will create a simple event publishing service using grpc and generate the client and server code. Then I will write some code that uses the client and server.

Working with protobuf and gRPC servers was suprisingly easy. 
The proto3 syntax is very readable and actually easier that just writing standard go code. 
Once the gRPC service is defined in the proto file, you can generate a lot of the boiler plate code that you would have to write yourself otherwise.

The example service I created is an in memory events database that can return results that are tagged with certain strings.

Stream events to the server over gRPC and then request the data back with tag filters.

# Quickstart running this code
`term 1` - Start the server
```sh
make server
```

`term 2` - Add many events to the server
```sh
make add-many
```

`term 3` - Query events
```sh
make get-prod
```

`term 3` - Get Server Metrics
```
make metric
```



# Resources
| title | link|
| - | - |
| grpc introduction   |   https://grpc.io/docs/what-is-grpc/introduction/ |
| grpc golang quickstart | https://grpc.io/docs/languages/go/quickstart/ |
| grpc golang basics    | https://grpc.io/docs/languages/go/basics/ |

# Creating the proto file
This proto file contains the definitions of the grpc server.

The EventStore service exposes the `GetEvents`, `StoreEvents`, and `ServerMetrics` Remote Proceedure Calls. Each `rpc` has inputs and outputs that are defined in the same proto file.

`eventstore/event_store.proto`
```proto3
syntax = "proto3";

option go_package = "github.com/kfelter/grpc-example/eventstore";

package eventstore;

// Interface exported by the server.
service EventStore {
  // Get all events that match a set of tags, an event must have all the tags that are requested to be returned
  rpc GetEvents(GetEventRequest) returns (stream Event) {}

  // Stores events that come in on the stream
  rpc StoreEvents(stream Event) returns (StoreEventsResponse) {}

  // Returns the server metrics
  rpc ServerMetrics(ServerMestricsRequest) returns (ServerMetricsResponse) {}
}

message Event {
    repeated string tags = 2;
    bytes content = 3;
}

message GetEventRequest {
    repeated string tags = 2;
}

message StoreEventsResponse {
    repeated Event events = 1;
    string status = 2;
}

message ServerMestricsRequest {}

message ServerMetricsResponse{
    string status = 1;
    string avg_get_query_duration = 2;
    string avg_store_duration = 3;
    int64 len_events = 4;
}
```

# Generating the go code
Using the protoc binary we generate the go package code
```sh
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    eventstore/event_store.proto
```

# Implementing the EventStore gRPC server
Now in a seperate file we need to implement the interface that was generated by `protoc`

`cmd/server/main.go`
```go
type eventStoreServer struct {
	pb.UnimplementedEventStoreServer
	mu        sync.Mutex
	events    []*pb.Event
	idCounter int64
}

func (s *eventStoreServer) GetEvents(req *pb.GetEventRequest, stream pb.EventStore_GetEventsServer) error {
...
}

func (s *eventStoreServer) StoreEvents(stream pb.EventStore_StoreEventsServer) error {
...
}


func (s *eventStoreServer) ServerMetrics(c context.Context, req *pb.ServerMestricsRequest) (*pb.ServerMetricsResponse, error) {
...
}
```

# Starting the tcp listener for the gRPC service
We need to expose the Remote Proceedure Calls over a network connection.

`cmd/server/main.go`
```go
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
```

# Writing the gRPC client
The gRPC client could be writen in many other languages easily by generating the protobuf code in another language.

This feature makes it very easy to write a service that integrates with many other languages with no extra developer effort.

Since we already generated the go code we will use go to write the client.

`cmd/client/main.go`
```go
func store(c *cli.Context) error {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(c.String("server-addr"), opts...)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := pb.NewEventStoreClient(conn)
	stream, err := client.StoreEvents(context.Background())
	if err != nil {
		panic(err)
	}
	var content []byte
	if cf := c.String("content-file"); cf == "stdin" {
		content, err = io.ReadAll(os.Stdin)
	} else {
		content, err = os.ReadFile(cf)
	}
	if err != nil {
		return err
	}
	err = stream.Send(&pb.Event{
		Content: content,
		Tags:    c.StringSlice("tags"),
	})
	if err != nil {
		return err
	}
	reply, err := stream.CloseAndRecv()
	if err != nil {
		return err
	}
	fmt.Println(reply.String())
	return nil
}

func get(c *cli.Context) error {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(c.String("server-addr"), opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewEventStoreClient(conn)
	getStream, err := client.GetEvents(context.Background(), &pb.GetEventRequest{
		Tags: c.StringSlice("tags"),
	})

	for {
		e, err := getStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Println(e.String())
	}
	return nil
}

func metric(c *cli.Context) error {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(c.String("server-addr"), opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewEventStoreClient(conn)
	res, err := client.ServerMetrics(context.Background(), &pb.ServerMestricsRequest{})
	if err != nil {
		return err
	}
	fmt.Println(res.String())
	return nil
}
```

# Putting it all together
Lets send some events to the server
```Makefile
server:
	go run cmd/server/main.go

add-many: # add many events
	@go build -o client cmd/client/main.go
	@for number in $(shell seq 1 10000); do \
    	printf "example event $$number $$(date)" | ./client store -t "env:test";\
		printf "example event $$number $$(date)" | ./client store -t "env:stage";\
		printf "example event $$number $$(date)" | ./client store -t "env:prod"; \
	done
```
```sh
$ make add-many
events:{tags:"env:test" tags:"id:0" content:"example event 1 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:stage" tags:"id:1" content:"example event 1 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:prod" tags:"id:2" content:"example event 1 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:test" tags:"id:3" content:"example event 2 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:stage" tags:"id:4" content:"example event 2 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:prod" tags:"id:5" content:"example event 2 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
events:{tags:"env:test" tags:"id:6" content:"example event 3 Sun May  2 18:11:01 EDT 2021"} status:"added 1 events"
...
```
Now lets pull out the events using GetEvents client code
```sh
$ go run cmd/client/main.go get -t "env:prod"
tags:"env:prod" tags:"id:2" content:"example event 1 Sun May  2 18:11:01 EDT 2021"
tags:"env:prod" tags:"id:5" content:"example event 2 Sun May  2 18:11:01 EDT 2021"
tags:"env:prod" tags:"id:8" content:"example event 3 Sun May  2 18:11:01 EDT 2021"
tags:"env:prod" tags:"id:11" content:"example event 4 Sun May  2 18:11:02 EDT 2021"
tags:"env:prod" tags:"id:14" content:"example event 5 Sun May  2 18:11:02 EDT 2021"
tags:"env:prod" tags:"id:17" content:"example event 6 Sun May  2 18:11:02 EDT 2021"
...
```