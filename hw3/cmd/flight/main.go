package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "hw3/gen/flight/v1"
	"hw3/internal/auth"
	"hw3/internal/flight"
	"hw3/internal/platform"

	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	apiKey := os.Getenv("FLIGHT_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("FLIGHT_API_KEY is required")
	}

	db, err := platform.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	listener, err := net.Listen("tcp", platform.GetEnv("GRPC_ADDR", ":50051"))
	if err != nil {
		return err
	}
	defer listener.Close()

	server := grpc.NewServer(grpc.UnaryInterceptor(auth.ServerInterceptor(apiKey)))
	pb.RegisterFlightServiceServer(server, flight.NewServer(db))

	go func() {
		<-ctx.Done()
		timer := time.AfterFunc(5*time.Second, server.Stop)
		defer timer.Stop()
		server.GracefulStop()
	}()
	return server.Serve(listener)
}
