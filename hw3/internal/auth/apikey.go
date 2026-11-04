package auth

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const apiKeyHeader = "api-key"

func ClientInterceptor(key string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn,
		invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, apiKeyHeader, key)
		return invoke(ctx, method, req, reply, conn, opts...)
	}
}

func ServerInterceptor(key string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, apiKeyHeader)
		if key == "" || len(values) != 1 || values[0] != key {
			return nil, status.Error(codes.Unauthenticated, "missing or invalid API key")
		}
		return handler(ctx, req)
	}
}
