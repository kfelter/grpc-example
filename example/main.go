package main

import (
	"encoding/json"
	"fmt"
	"os"

	pb "github.com/kfelter/grpc-example/eventstore"
	"google.golang.org/protobuf/proto"
)

func main() {
	// Encode Data
	// - protobuf
	event := pb.Event{
		Tags:    []string{"tag1", "tag2"},
		Content: []byte(`Hello, World`),
	}
	byteBuffer, _ := proto.Marshal(&event)
	fmt.Println("size of protobuf file", len(byteBuffer))
	os.WriteFile("event.protobuf", byteBuffer, os.ModePerm)

	// - json
	byteBuffer, _ = json.Marshal(&event)
	fmt.Println("size of json file", len(byteBuffer))
	os.WriteFile("event.json", byteBuffer, os.ModePerm)

	// Decode Data
	// - protobuf
	event = pb.Event{}
	byteBuffer, _ = os.ReadFile("event.protobuf")
	proto.Unmarshal(byteBuffer, &event)
	fmt.Println(event.String())

	// - json
	event = pb.Event{}
	byteBuffer, _ = os.ReadFile("event.json")
	json.Unmarshal(byteBuffer, &event)
	fmt.Println(event.String())
}
