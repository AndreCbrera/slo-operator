package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"service", "code"},
	)

	httpRequestDurationCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_duration_seconds_count",
			Help: "Total request count for duration histogram",
		},
		[]string{"service"},
	)

	httpRequestDurationBucket = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_duration_seconds_bucket",
			Help: "Request duration bucket",
		},
		[]string{"service", "le"},
	)

	paymentTransactionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_transactions_total",
			Help: "Total payment transactions",
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDurationCount)
	prometheus.MustRegister(httpRequestDurationBucket)
	prometheus.MustRegister(paymentTransactionsTotal)
}

func main() {
	go generateTraffic()

	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("traffic-gen listening on :8080")
	http.ListenAndServe(":8080", nil)
}

func generateTraffic() {
	for {
		// api-gateway: ~0.05% error rate (well within 99.9% SLO)
		for i := 0; i < 100; i++ {
			httpRequestsTotal.WithLabelValues("api-gateway", "200").Inc()
		}
		// Always emit at least some 5xx for the series to exist
		if rand.Float64() < 0.05 {
			httpRequestsTotal.WithLabelValues("api-gateway", "500").Inc()
		}

		// checkout-service: latency - 98% within 500ms
		for i := 0; i < 50; i++ {
			httpRequestDurationCount.WithLabelValues("checkout").Inc()
			if rand.Float64() < 0.98 {
				httpRequestDurationBucket.WithLabelValues("checkout", "0.5").Inc()
			}
		}

		// payments: ~0.5% error rate for visible data
		for i := 0; i < 80; i++ {
			paymentTransactionsTotal.WithLabelValues("success").Inc()
		}
		if rand.Float64() < 0.3 {
			paymentTransactionsTotal.WithLabelValues("error").Inc()
		}

		time.Sleep(1 * time.Second)
	}
}
