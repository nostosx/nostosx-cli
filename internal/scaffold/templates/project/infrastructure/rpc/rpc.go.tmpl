// Package rpc applies authentication, deadlines and safe errors at the RPC boundary.
package rpc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const MaxMessageBytes = 16 << 10

// LoopbackAddress confines a plaintext listener to this host.
func LoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback()
}

func Authenticate(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (_ any, err error) {
		defer func() {
			if recover() != nil {
				slog.Error("RPC handler panicked", "method", info.FullMethod)
				err = status.Error(codes.Internal, "INTERNAL")
			}
		}()
		if err := authenticate(ctx, token); err != nil {
			return nil, err
		}
		timeout := 5 * time.Second
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return next(ctx, request)
	}
}

// AuthenticateStream also protects built-in streaming health probes. Application
// streaming methods must define their own deadlines and per-message policies.
func AuthenticateStream(token string) grpc.StreamServerInterceptor {
	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) (err error) {
		defer func() {
			if recover() != nil {
				slog.Error("RPC stream panicked", "method", info.FullMethod)
				err = status.Error(codes.Internal, "INTERNAL")
			}
		}()
		if err := authenticate(stream.Context(), token); err != nil {
			return err
		}
		return next(server, stream)
	}
}

func authenticate(ctx context.Context, token string) error {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return status.Error(codes.Unauthenticated, "UNAUTHENTICATED")
	}
	value, ok := strings.CutPrefix(values[0], "Bearer ")
	expected, actual := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(value))
	if token == "" || !ok || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
		return status.Error(codes.Unauthenticated, "UNAUTHENTICATED")
	}
	return nil
}

// Error keeps backend details in local logs and exposes only stable reasons.
func Error(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "DEADLINE_EXCEEDED")
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "CANCELED")
	}
	slog.ErrorContext(ctx, "RPC request failed", "error", err)
	return status.Error(codes.Internal, "INTERNAL")
}
