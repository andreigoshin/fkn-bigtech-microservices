package metrics

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// MetricsUnaryInterceptor - ...
func MetricsUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (_ interface{}, err error) {
		start := time.Now()
		defer func() {
			responseTimeHistogramObserve(info.FullMethod, err, time.Since(start))
		}()

		return handler(ctx, req)
	}
}
