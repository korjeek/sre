package chaos

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"time"
)

type Config struct {
	ErrorRate float64
	Latency   time.Duration
}

func (c Config) Enabled() bool { return c.ErrorRate > 0 || c.Latency > 0 }

func Wrap(c Config, next http.Handler) http.Handler {
	if !c.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.Latency > 0 {
			t := time.NewTimer(c.Latency)
			select {
			case <-t.C:
			case <-r.Context().Done():
				t.Stop()
				return
			}
		}
		if c.ErrorRate > 0 && rand.Float64() < c.ErrorRate {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "chaos: injected failure"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
