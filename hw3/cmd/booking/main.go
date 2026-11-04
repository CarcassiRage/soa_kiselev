package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "hw3/gen/flight/v1"
	"hw3/internal/auth"
	"hw3/internal/booking"
	"hw3/internal/platform"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	conn, err := grpc.NewClient(platform.GetEnv("FLIGHT_GRPC_ADDR", "localhost:50051"),
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auth.ClientInterceptor(apiKey)))
	if err != nil {
		return err
	}
	defer conn.Close()

	server := &http.Server{
		Addr:              platform.GetEnv("HTTP_ADDR", ":8080"),
		Handler:           booking.NewHandler(db, pb.NewFlightServiceClient(conn)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}()
	err = server.ListenAndServe()
	stop()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
