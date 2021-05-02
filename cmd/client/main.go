package main

import (
	"fmt"

	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/protobuf/proto"
)

func main() {
	e := pb.Event{
		Content: []byte(`new event`),
		Tags:    []string{"environment:test"},
	}
	fmt.Println(proto.Marshal(&e))
}
