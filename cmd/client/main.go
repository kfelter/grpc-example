package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	pb "github.com/kfelter/grpc-example/eventstore"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var (
	tagsFlag = &cli.StringSliceFlag{
		Name:    "tags",
		Aliases: []string{"t"},
		Usage:   "array of tags",
	}

	serverAddrFlag = &cli.StringFlag{
		Name:    "server-addr",
		Aliases: []string{"u"},
		Value:   "localhost:10000",
		Usage:   "grpc address server host:port",
	}

	inFileFlag = &cli.StringFlag{
		Name:    "in-file",
		Aliases: []string{"i"},
		Value:   "server.dump",
		Usage:   "file path to load db from",
	}

	outFileFlag = &cli.StringFlag{
		Name:    "out-file",
		Aliases: []string{"o"},
		Value:   "server.dump",
		Usage:   "file path to dump db",
	}

	getFlags = []cli.Flag{
		tagsFlag,
		serverAddrFlag,
	}

	storeFlags = []cli.Flag{
		serverAddrFlag,
		tagsFlag,
		&cli.StringFlag{
			Name:    "content-file",
			Aliases: []string{"i"},
			Value:   "stdin",
			Usage:   "file name for message content up to 4GB",
		},
	}

	storeCommand = &cli.Command{
		Name:   "store",
		Usage:  "store an event using the grpc client",
		Flags:  storeFlags,
		Action: store,
	}

	getCommand = &cli.Command{
		Name:   "get",
		Usage:  "get events that match tags",
		Flags:  getFlags,
		Action: get,
	}

	metricCommand = &cli.Command{
		Name:   "metric",
		Usage:  "get server metrics",
		Flags:  []cli.Flag{serverAddrFlag},
		Action: metric,
	}

	saveCommand = &cli.Command{
		Name:   "save",
		Usage:  "save the db to a file",
		Flags:  []cli.Flag{serverAddrFlag, outFileFlag},
		Action: save,
	}

	loadCommand = &cli.Command{
		Name:   "load",
		Usage:  "load the db from a file",
		Flags:  []cli.Flag{serverAddrFlag, inFileFlag},
		Action: load,
	}
)

func main() {
	app := &cli.App{
		Name:  "client",
		Usage: "send events to the grpc server",
		Commands: []*cli.Command{
			storeCommand,
			getCommand,
			metricCommand,
			saveCommand,
			loadCommand,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

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

func save(c *cli.Context) error {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(c.String("server-addr"), opts...)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewEventStoreClient(conn)
	getStream, err := client.GetEvents(context.Background(), &pb.GetEventRequest{})

	eventList := pb.EventList{}
	for {
		e, err := getStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		eventList.Events = append(eventList.Events, e)
	}
	b, err := proto.Marshal(&eventList)
	if err != nil {
		return err
	}

	return os.WriteFile(c.String("out-file"), b, os.ModePerm)
}

func load(c *cli.Context) error {
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

	eventList := &pb.EventList{}
	b, err := os.ReadFile(c.String("in-file"))
	err = proto.Unmarshal(b, eventList)
	if err != nil {
		return err
	}
	for _, e := range eventList.Events {
		err = stream.Send(e)
		if err != nil {
			return err
		}
	}

	reply, err := stream.CloseAndRecv()
	if err != nil {
		return err
	}
	fmt.Println(reply.String())
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
