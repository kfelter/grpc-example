proto: # generates the golang code from the .proto file
	protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    eventstore/event_store.proto

mod:
	go mod tidy
	go mod vendor

server:
	go run cmd/server/main.go

client:
	go run cmd/client/main.go