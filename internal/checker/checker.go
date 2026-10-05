package checker

import (
	"net/http"
	"strings"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

type Checker struct {
	client *http.Client
}

func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Checker) Check(address string) (domain.CheckResult, error) {
	address = normalizeURL(address)

	start := time.Now()

	resp, err := c.client.Get(address)
	if err != nil {
		return domain.CheckResult{
			Status:    domain.StatusDown,
			ErrMsg:    err.Error(),
			LatencyMs: int64(time.Since(start).Milliseconds()),
			CheckedAt: time.Now(),
		}, nil
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()
	status := domain.StatusUp
	if resp.StatusCode >= 400 {
		status = domain.StatusDown
	}
	return domain.CheckResult{
		Status:     status,
		StatusCode: resp.StatusCode,
		LatencyMs:  int64(latency),
		CheckedAt:  time.Now(),
	}, nil
}

func normalizeURL(rawAdr string) string {
	rawAdr = strings.TrimSpace(strings.ToLower(rawAdr))
	if !strings.HasPrefix(rawAdr, "http://") && !strings.HasPrefix(rawAdr, "https://") {
		return "http://" + rawAdr
	}
	return rawAdr
}
