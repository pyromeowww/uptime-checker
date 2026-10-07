package checker

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

// Checker представляет собой сервис для проверки доступности сайтов.
type Checker struct {
	client *http.Client
}

// NewChecker создаёт новый экземпляр Checker с предустановленным таймаутом HTTP-клиента.
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

// Check выполняет проверку одного или нескольких сайтов.
func (c *Checker) Check(address ...string) ([]domain.CheckResult, error) {
	switch len(address) {
	case 0:
		return []domain.CheckResult{}, errors.New("address is empty")
	case 1:
		url := address[0]
		res, err := c.checkSingle(url)
		if err != nil {
			return nil, err
		}
		return []domain.CheckResult{res}, nil
	default:
		return c.checkParallel(address)
	}
}

// normalizeURL приводит URL к единому формату
func normalizeURL(rawAdr string) string {
	rawAdr = strings.TrimSpace(strings.ToLower(rawAdr))
	if !strings.HasPrefix(rawAdr, "http://") && !strings.HasPrefix(rawAdr, "https://") {
		return "http://" + rawAdr
	}
	return rawAdr
}

// checkSingle выполняет HTTP GET-запрос к указанному URL и измеряет время отклика.
func (c *Checker) checkSingle(url string) (domain.CheckResult, error) {
	url = normalizeURL(url)
	start := time.Now()
	formattedTime := start.Format("2006-01-02 15:04:05")

	resp, err := c.client.Get(url)
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

// checkParallel выполняет параллельную проверку списка URL с использованием горутин и sync.WaitGroup.
func (c *Checker) checkParallel(address []string) ([]domain.CheckResult, error) {
	results := make([]domain.CheckResult, len(address))
	var wg sync.WaitGroup

	for i, url := range address {
		wg.Add(1)
		go func(idx int, targetURL string) {
			defer wg.Done()

			cleanURL := normalizeURL(targetURL)
			res, _ := c.checkSingle(cleanURL)
			results[idx] = res
		}(i, url)
	}
	wg.Wait()
	return results, nil
}
