package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"sre-incident-tracker/internal/models"
)

const namespace = "sre"

var (
	Severities = []string{"low", "medium", "high", "critical"}
	Statuses   = []string{"open", "investigating", "resolved"}
)

type StateSource interface {
	IncidentCounts(ctx context.Context) ([]models.IncidentCount, error)
	ServicesCount(ctx context.Context) (int64, error)
}

type Metrics struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInFlight prometheus.Gauge

	servicesCreated     prometheus.Counter
	servicesDeleted     prometheus.Counter
	incidentsCreated    *prometheus.CounterVec
	incidentsResolved   *prometheus.CounterVec
	incidentsDeleted    prometheus.Counter
	incidentTransitions *prometheus.CounterVec
	resolutionDuration  *prometheus.HistogramVec

	chaosErrorRate prometheus.Gauge
	chaosLatency   prometheus.Gauge
}

func New(src StateSource, log *slog.Logger) *Metrics {
	reg := prometheus.NewRegistry()
	f := promauto.With(reg)

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)

	m := &Metrics{
		registry: reg,

		httpRequests: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_total",
			Help: "Количество обработанных HTTP-запросов.",
		}, []string{"method", "route", "code"}),
		httpDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help:    "Длительность обработки HTTP-запросов.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		httpInFlight: f.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_in_flight",
			Help: "Количество HTTP-запросов, обрабатываемых прямо сейчас.",
		}),

		servicesCreated: f.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "services_created_total",
			Help: "Сколько сервисов добавлено в каталог.",
		}),
		servicesDeleted: f.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "services_deleted_total",
			Help: "Сколько сервисов удалено из каталога.",
		}),
		incidentsCreated: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "incidents_created_total",
			Help: "Сколько инцидентов заведено, по критичности.",
		}, []string{"severity"}),
		incidentsResolved: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "incidents_resolved_total",
			Help: "Сколько инцидентов закрыто (переход в resolved), по критичности.",
		}, []string{"severity"}),
		incidentsDeleted: f.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "incidents_deleted_total",
			Help: "Сколько инцидентов удалено.",
		}),
		incidentTransitions: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "incident_status_transitions_total",
			Help: "Переходы инцидентов между статусами.",
		}, []string{"from", "to"}),
		resolutionDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "incident_resolution_duration_seconds",
			Help: "Время от создания инцидента до перехода в resolved.",
			Buckets: []float64{
				60, 5 * 60, 15 * 60, 30 * 60, 3600,
				4 * 3600, 12 * 3600, 24 * 3600, 3 * 24 * 3600, 7 * 24 * 3600,
			},
		}, []string{"severity"}),

		chaosErrorRate: f.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "chaos_error_rate",
			Help: "Доля запросов /api, на которые намеренно отвечаем 503 (0 — режим выключен).",
		}),
		chaosLatency: f.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "chaos_latency_seconds",
			Help: "Искусственная задержка запросов /api в секундах (0 — режим выключен).",
		}),
	}

	for _, sev := range Severities {
		m.incidentsCreated.WithLabelValues(sev)
		m.incidentsResolved.WithLabelValues(sev)
		m.resolutionDuration.WithLabelValues(sev)
	}
	for _, from := range Statuses {
		for _, to := range Statuses {
			if from != to {
				m.incidentTransitions.WithLabelValues(from, to)
			}
		}
	}

	if src != nil {
		reg.MustRegister(newStateCollector(src, log))
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry:          m.registry,
		EnableOpenMetrics: true,
	})
}

func (m *Metrics) SetChaos(errorRate float64, latency time.Duration) {
	m.chaosErrorRate.Set(errorRate)
	m.chaosLatency.Set(latency.Seconds())
}

func (m *Metrics) HTTPInFlightInc() { m.httpInFlight.Inc() }
func (m *Metrics) HTTPInFlightDec() { m.httpInFlight.Dec() }

func (m *Metrics) ObserveHTTP(method, route string, status int, d time.Duration) {
	m.httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

func (m *Metrics) ServiceCreated() { m.servicesCreated.Inc() }
func (m *Metrics) ServiceDeleted() { m.servicesDeleted.Inc() }

func (m *Metrics) IncidentCreated(severity string) {
	m.incidentsCreated.WithLabelValues(severity).Inc()
}

func (m *Metrics) IncidentDeleted() { m.incidentsDeleted.Inc() }

func (m *Metrics) IncidentStatusChanged(from, to, severity string, age time.Duration) {
	m.incidentTransitions.WithLabelValues(from, to).Inc()
	if to == "resolved" {
		m.incidentsResolved.WithLabelValues(severity).Inc()
		m.resolutionDuration.WithLabelValues(severity).Observe(age.Seconds())
	}
}

type stateCollector struct {
	src  StateSource
	log  *slog.Logger
	inc  *prometheus.Desc
	svcs *prometheus.Desc
}

func newStateCollector(src StateSource, log *slog.Logger) *stateCollector {
	if log == nil {
		log = slog.Default()
	}
	return &stateCollector{
		src: src,
		log: log,
		inc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "incidents_current"),
			"Текущее число инцидентов по статусу и критичности.",
			[]string{"status", "severity"}, nil),
		svcs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "services_current"),
			"Текущее число сервисов в каталоге.",
			nil, nil),
	}
}

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.inc
	ch <- c.svcs
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	counts, err := c.src.IncidentCounts(ctx)
	if err != nil {
		c.log.Error("metrics: incident counts failed", "err", err)
	} else {
		byKey := make(map[[2]string]int64, len(counts))
		for _, row := range counts {
			byKey[[2]string{row.Status, row.Severity}] += row.Count
		}
		for _, st := range Statuses {
			for _, sev := range Severities {
				ch <- prometheus.MustNewConstMetric(c.inc, prometheus.GaugeValue,
					float64(byKey[[2]string{st, sev}]), st, sev)
			}
		}
	}

	n, err := c.src.ServicesCount(ctx)
	if err != nil {
		c.log.Error("metrics: services count failed", "err", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.svcs, prometheus.GaugeValue, float64(n))
}
