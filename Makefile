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
	@go build -o client cmd/client/main.go
	for number in $(shell seq 1 10000); do \
    	printf "example event $$number $$(date)" | ./client store -t "env:test"; \
		printf "example event $$number $$(date)" | ./client store -t "env:stage"; \
		printf "example event $$number $$(date)" | ./client store -t "env:prod"; \
	done

get-prod:
	go run cmd/client/main.go get -t "env:prod"

get-stage:
	go run cmd/client/main.go get -t "env:stage"

get-test:
	go run cmd/client/main.go get -t "env:test"

metric:
	go run cmd/client/main.go metric
	