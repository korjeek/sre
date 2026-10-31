package prober

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Result string

const (
	ResultOK    Result = "ok"
	ResultSlow  Result = "slow"
	ResultError Result = "error"
)

var results = []Result{ResultOK, ResultSlow, ResultError}

type Target struct {
	Name       string
	Path       string
	ExpectList bool
}

func DefaultTargets() []Target {
	return []Target{
		{Name: "healthz", Path: "/healthz"},
		{Name: "services_list", Path: "/api/services", ExpectList: true},
		{Name: "incidents_list", Path: "/api/incidents", ExpectList: true},
	}
}

type Config struct {
	BaseURL       string
	Interval      time.Duration
	Timeout       time.Duration
	SlowThreshold time.Duration
	Targets       []Target
}

type Prober struct {
	cfg      Config
	client   *http.Client
	runs     *prometheus.CounterVec
	duration *prometheus.HistogramVec
	success  *prometheus.GaugeVec
}

func New(cfg Config, client *http.Client, reg prometheus.Registerer) *Prober {
	if client == nil {
		client = &http.Client{}
	}
	p := &Prober{
		cfg:    cfg,
		client: client,
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "sre", Subsystem: "probe", Name: "runs_total",
			Help: "Количество проверок по результатам: ok, slow, error.",
		}, []string{"probe", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "sre", Subsystem: "probe", Name: "duration_seconds",
			Help:    "Длительность проверки.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"probe"}),
		success: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "sre", Subsystem: "probe", Name: "success",
			Help: "Результат последней проверки: 1 — ok, 0 — slow или error.",
		}, []string{"probe"}),
	}
	reg.MustRegister(p.runs, p.duration, p.success)

	for _, t := range cfg.Targets {
		for _, r := range results {
			p.runs.WithLabelValues(t.Name, string(r))
		}
		p.duration.WithLabelValues(t.Name)
		p.success.WithLabelValues(t.Name)
	}
	return p
}

func (p *Prober) Check(ctx context.Context, t Target) (Result, time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+t.Path, nil)
	if err != nil {
		return ResultError, 0
	}
	req.Header.Set("User-Agent", "sre-prober/1.0")

	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return ResultError, time.Since(start)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	elapsed := time.Since(start)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ResultError, elapsed
	}
	if t.ExpectList {
		var list []json.RawMessage
		if err := json.Unmarshal(body, &list); err != nil {
			return ResultError, elapsed
		}
	}
	if elapsed > p.cfg.SlowThreshold {
		return ResultSlow, elapsed
	}
	return ResultOK, elapsed
}

func (p *Prober) RunOnce(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range p.cfg.Targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, d := p.Check(ctx, t)
			p.runs.WithLabelValues(t.Name, string(res)).Inc()
			p.duration.WithLabelValues(t.Name).Observe(d.Seconds())
			if res == ResultOK {
				p.success.WithLabelValues(t.Name).Set(1)
			} else {
				p.success.WithLabelValues(t.Name).Set(0)
			}
		}()
	}
	wg.Wait()
}

func (p *Prober) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	p.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.RunOnce(ctx)
		}
	}
}
