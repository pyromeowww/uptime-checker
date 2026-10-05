package domain

import (
	"time"
)

type TargetStatus string

const (
	StatusUp      TargetStatus = "UP"
	StatusDown    TargetStatus = "DOWN"
	StatusUnknown TargetStatus = "UNKNOWN"
)

// Target — это отслеживаемый ресурс (сайт, IP или домен)
type Target struct {
	ID        int64         `json:"id" db:"id"`
	URL       string        `json:"url" db:"url"`
	Interval  time.Duration `json:"interval" db:"interval"`     // Интервал проверки
	Status    TargetStatus  `json:"status" db:"status"`         // Текущий статус
	LastCheck time.Time     `json:"last_check" db:"last_check"` // Время последней проверки
	CreateAt  time.Time     `json:"create_at" db:"create_at"`   // Время добавления в систему
}

// CheckResult — результат отдельной проверки ресурса.
type CheckResult struct {
	ID         int64        `json:"id" db:"id"`
	TargetID   int64        `json:"target_id" db:"target_id"`       // Связь с Target
	Status     TargetStatus `json:"status" db:"status"`             // UP или DOWN
	StatusCode int          `json:"status_code" db:"status_code"`   // HTTP-код ответа
	LatencyMs  int64        `json:"latency_ms" db:"latency_ms"`     // Время отклика в миллисекундах
	ErrMsg     string       `json:"err_msg,omitempty" db:"err_msg"` // Текст ошибки, если ресурс недоступен
	CheckedAt  time.Time    `json:"checked_at" db:"checked_at"`     // Точное время проверки
}

// CheckRequest — структура для проверки списка адресов через API.
type CheckRequest struct {
	Addresses []string `json:"addresses"` // Список URL / IP / доменов для быстрой проверки
}
