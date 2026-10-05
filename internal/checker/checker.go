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
	formattedTime := start.Format("2006-01-02 15:04:05")

	resp, err := c.client.Get(address)
	if err != nil {
		return domain.CheckResult{
			Status:    domain.StatusDown,
			ErrMsg:    err.Error(),
			LatencyMs: time.Since(start).Round(time.Millisecond).String(),
			CheckedAt: formattedTime,
		}, nil
	}
	defer resp.Body.Close()

	latencyStr := time.Since(start).Round(time.Millisecond).String()
	status := domain.StatusUp
	if resp.StatusCode >= 400 {
		status = domain.StatusDown
	}
	return domain.CheckResult{
		Status:     status,
		StatusCode: resp.StatusCode,
		LatencyMs:  latencyStr,
		CheckedAt:  formattedTime,
	}, nil
}

func normalizeURL(rawAdr string) string {
	rawAdr = strings.TrimSpace(strings.ToLower(rawAdr))
	if !strings.HasPrefix(rawAdr, "http://") && !strings.HasPrefix(rawAdr, "https://") {
		return "http://" + rawAdr
	}
	return rawAdr
}
