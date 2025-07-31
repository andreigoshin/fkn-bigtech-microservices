package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	appName = "orders_management_system"
)

var ms struct {
	responseTimeHistogram *prometheus.HistogramVec
}

func init() {
	ms.responseTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "balun_courses",
			Subsystem: "grpc",
			Name:      "histogram_response_time_seconds",
			Help:      "Время ответа от сервера",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 16), //
			// Buckets: []float64{0.001, 0.005, 0.5, 1.0},
		},
		[]string{"service", "method", "is_error"},
	)
}

func responseTimeHistogramObserve(method string, err error, d time.Duration) {
	isError := strconv.FormatBool(err != nil)
	ms.responseTimeHistogram.WithLabelValues(appName, method, isError).Observe(d.Seconds())
}
