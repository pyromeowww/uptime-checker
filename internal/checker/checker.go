package checker

import (
	"errors"
	"net/http"
	"strings"
	"sync"
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
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Checker) Check(address ...string) ([]domain.CheckResult, error) {
	switch len(address) {
	case 0:
		return []domain.CheckResult{}, errors.New("address is empty")
	case 1:
		return []domain.CheckResult{c.checkSingle(address[0])}, nil
	default:
		return c.checkParallel(address), nil
	}
}

func NormalizeURL(rawAdr string) string {
	rawAdr = strings.TrimSpace(strings.ToLower(rawAdr))
	if !strings.HasPrefix(rawAdr, "http://") && !strings.HasPrefix(rawAdr, "https://") {
		return "http://" + rawAdr
	}
	return rawAdr
}

func (c *Checker) checkSingle(url string) domain.CheckResult {
	url = NormalizeURL(url)
	start := time.Now()
	formattedTime := start.Format("2006-01-02 15:04:05")

	resp, err := c.client.Get(url)
	if err != nil {
		return domain.CheckResult{
			Status:    domain.StatusDown,
			ErrMsg:    err.Error(),
			LatencyMs: time.Since(start).Round(time.Millisecond).String(),
			CheckedAt: formattedTime,
		}
	}
	defer func() { _ = resp.Body.Close() }()

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
	}
}

func (c *Checker) checkParallel(address []string) []domain.CheckResult {
	results := make([]domain.CheckResult, len(address))
	var wg sync.WaitGroup
	for i, url := range address {
		wg.Add(1)
		go func(idx int, targetURL string) {
			defer wg.Done()
			results[idx] = c.checkSingle(targetURL)
		}(i, url)
	}
	wg.Wait()
	return results
}
