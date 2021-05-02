proto: # generates the golang code from the .proto file
	protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    eventstore/event_store.proto

mod:
	go mod tidy
	go mod vendor

server:
	go run cmd/server/main.go

client-help:
	go run cmd/client/main.go

add-many: # add many events
	@echo "example event 0" | go run cmd/client/main.go store -t "env:test"
	@echo "example event 1" | go run cmd/client/main.go store -t "env:test"
	@echo "example event 2" | go run cmd/client/main.go store -t "env:stage"
	@echo "example event 3" | go run cmd/client/main.go store -t "env:stage"
	@echo "example event 4" | go run cmd/client/main.go store -t "env:prod"
	@echo "example event 5" | go run cmd/client/main.go store -t "env:prod"

get-prod:
	go run cmd/client/main.go get -t "env:prod"

get-stage:
	go run cmd/client/main.go get -t "env:stage"

get-test:
	go run cmd/client/main.go get -t "env:test"