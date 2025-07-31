package main

import (
	"context"
	"os"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	grpc_opentracing "github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/repository/orders_storage"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/server"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/services/warehouses_management_system"
	transaction_manager "github.com/moguchev/microservices_courcse/orders_management_system/internal/app/transaction_manager/postgres"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/usecases/orders_management_system"
	middleware_errors "github.com/moguchev/microservices_courcse/orders_management_system/internal/middleware/errors"
	middleware_logging "github.com/moguchev/microservices_courcse/orders_management_system/internal/middleware/logging"
	middleware_metrics "github.com/moguchev/microservices_courcse/orders_management_system/internal/middleware/metrics"
	middleware_recovery "github.com/moguchev/microservices_courcse/orders_management_system/internal/middleware/recovery"
	middleware_tracing "github.com/moguchev/microservices_courcse/orders_management_system/internal/middleware/tracing"
	"github.com/moguchev/microservices_courcse/orders_management_system/pkg/logger"
	"github.com/moguchev/microservices_courcse/orders_management_system/pkg/postgres"
	jaeger_tracing "github.com/moguchev/microservices_courcse/orders_management_system/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.SetLevel(zapcore.InfoLevel)

	logger.Info(ctx, "start app init")
	if err := jaeger_tracing.Init("orders-management-system"); err != nil {
		logger.Fatal(ctx, err)
	}

	dsn := os.Getenv("DB_DSN")
	// repository
	pool, err := postgres.NewConnectionPool(ctx, dsn,
		postgres.WithMaxConnIdleTime(5*time.Minute),
		postgres.WithMaxConnLifeTime(time.Hour),
		postgres.WithMaxConnectionsCount(10),
		postgres.WithMinConnectionsCount(5),
	)
	if err != nil {
		logger.ErrorKV(ctx, "can't connect to database", "error", err.Error(), "dsn", dsn)
	}

	txManager := transaction_manager.New(pool)

	storage := orders_storage.New(txManager)

	// services

	wmsClient := warehouses_management_system.NewClient()

	// usecases

	omsUsecase := orders_management_system.NewUsecase(orders_management_system.Deps{ // Dependency injection
		WarehouseManagementSystem: wmsClient,
		OrdersStorage:             storage,
		TransactionManager:        txManager,
	})

	// Setup metrics.
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	reg := prometheus.NewRegistry()
	reg.MustRegister(srvMetrics)

	exemplarFromContext := func(ctx context.Context) prometheus.Labels {
		if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
			return prometheus.Labels{"traceID": span.TraceID().String()}
		}
		return nil
	}

	// Setup metric for panic recoveries.
	panicsTotal := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
	grpcPanicRecoveryHandler := func(p any) (err error) {
		panicsTotal.Inc()
		return status.Errorf(codes.Internal, "%s", p)
	}

	// delivery
	config := server.Config{
		GRPCPort:        os.Getenv("GRPC_PORT"), // ":8082"
		GRPCGatewayPort: os.Getenv("HTTP_PORT"), // ":8080"
		DebugPort:       ":8084",
		ChainUnaryInterceptors: []grpc.UnaryServerInterceptor{
			// Open Source: https://github.com/grpc-ecosystem/go-grpc-middleware?tab=readme-ov-file#middleware
			// https://github.com/grpc-ecosystem/go-grpc-middleware/blob/main/examples/server/main.go

			// LOGGING
			// logging.UnaryServerInterceptor(interceptorLogger(rpcLogger), logging.WithFieldsFromContext(logTraceID)),

			// TRACING

			// open telemetry
			// otelgrpc.UnaryServerInterceptor(),

			// open tracing
			grpc_opentracing.OpenTracingServerInterceptor(opentracing.GlobalTracer(), grpc_opentracing.LogPayloads()),

			// METRICS
			srvMetrics.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(exemplarFromContext)),

			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),

			// Ручные
			middleware_logging.LogErrorUnaryInterceptor(),
			middleware_tracing.DebugOpenTracingUnaryServerInterceptor(true, true), // расширение для grpc_opentracing.OpenTracingServerInterceptor
			middleware_metrics.MetricsUnaryInterceptor(),
			middleware_recovery.RecoverUnaryInterceptor(), // можно использовать grpc_recovery
		},
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			middleware_errors.ErrorsUnaryInterceptor(), // далее наши остальные middleware
		},
	}

	srv, err := server.New(ctx, config, server.Deps{ // Dependency injection (DI)
		OMSUsecase: omsUsecase,
	})
	if err != nil {
		logger.Fatalf(ctx, "failed to create server: %v", err)
	}

	if err = srv.Run(ctx); err != nil {
		logger.Errorf(ctx, "run: %v", err)
	}
}
