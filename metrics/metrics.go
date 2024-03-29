package metrics

import (
	"context"
	"net"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/metric/unit"
	"go.opentelemetry.io/otel/sdk/metric"
)

var (
	log = logging.Logger("metrics")
)

type Metrics struct {
	exporter    *prometheus.Exporter
	httpLatency syncint64.Histogram
	s           *http.Server
}

func New(metricsAddr string) (*Metrics, error) {
	var m Metrics
	var err error
	if m.exporter, err = prometheus.New(prometheus.WithoutUnits()); err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(metric.WithReader(m.exporter))
	meter := provider.Meter("ipni/dhfind")

	if m.httpLatency, err = meter.SyncInt64().Histogram("ipni/dhfind/http_latency",
		instrument.WithUnit(unit.Milliseconds),
		instrument.WithDescription("Latency of DHFind HTTP API")); err != nil {
		return nil, err
	}

	m.s = &http.Server{
		Addr:    metricsAddr,
		Handler: metricsMux(),
	}

	return &m, nil
}

func (m *Metrics) RecordHttpLatency(ctx context.Context, t time.Duration, method, path string, status int, ttfr bool) {
	m.httpLatency.Record(ctx, t.Milliseconds(),
		attribute.String("method", method), attribute.String("path", path), attribute.Int("status", status), attribute.Bool("ttfr", ttfr))
}

func (m *Metrics) Start(_ context.Context) error {
	mln, err := net.Listen("tcp", m.s.Addr)
	if err != nil {
		return err
	}

	go func() { _ = m.s.Serve(mln) }()

	log.Infow("Metrics server started", "addr", mln.Addr())
	return nil
}

func (s *Metrics) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}

func metricsMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}
