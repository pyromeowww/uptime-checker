package worker

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/db"
	"github.com/pyromeowww/uptime-checker/internal/domain"
)

type Worker struct {
	store    *db.Store
	client   *http.Client
	interval time.Duration
}

func NewWorker(store *db.Store, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	client := &http.Client{Timeout: 10 * time.Second}

	return &Worker{
		store:    store,
		client:   client,
		interval: interval,
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Printf("Worker started with interval: %v", w.interval)

	// Сразу запускаем одну проверку при старте, не дожидаясь первого тика
	w.RunCheckCycle(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker stopped.")
			return
		case <-ticker.C:
			w.RunCheckCycle(ctx)
		}
	}
}

func (w *Worker) RunCheckCycle(ctx context.Context) {
	targets, err := w.store.GetDueTargets(ctx)
	if err != nil {
		log.Printf("[Worker] Failed to fetch targets: %v", err)
		return
	}
	if len(targets) == 0 {
		return
	}

	// Создаем канал для передачи целей воркерам
	jobs := make(chan domain.Target, len(targets))

	// Заполняем канал целями
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)

	// Определяем количество параллельных воркеров
	numWorkers := min(5, len(targets))

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Запускаем воркеры
	for range numWorkers {
		go func() {
			defer wg.Done()

			for target := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				w.checkTarget(ctx, target)
			}
		}()
	}

	// Ждем, пока все воркеры завершат проверки
	wg.Wait()
}

func (w *Worker) checkTarget(ctx context.Context, target domain.Target) {
	start := time.Now()

	reqCtx, cancel := context.WithTimeout(ctx, w.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		log.Printf("[Worker] Failed to create request for %s: %v", target.URL, err)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) UptimeChecker/1.0")

	var res domain.CheckResult
	res.TargetID = target.ID

	res.CheckedAt = start.Format("2006-01-02 15:04:05")

	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("[Worker] response error for %s: %v", target.URL, err)
		res.Status = domain.StatusDown
		res.ErrMsg = err.Error()
		res.LatencyMs = time.Since(start).Round(time.Millisecond).String()

		if saveErr := w.store.SaveResult(ctx, res); saveErr != nil {
			log.Printf("[Worker] Failed to save error result for %s: %v", target.URL, saveErr)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Успешный ответ
	res.LatencyMs = time.Since(start).Round(time.Millisecond).String()
	res.StatusCode = resp.StatusCode

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		res.Status = domain.StatusUp
	} else {
		res.Status = domain.StatusDown
	}

	if saveErr := w.store.SaveResult(ctx, res); saveErr != nil {
		log.Printf("[Worker] Failed to save result for %s: %v", target.URL, saveErr)
	}
}
