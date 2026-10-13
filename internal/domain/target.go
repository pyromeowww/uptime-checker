package domain

import (
	"time"
)

type TargetStatus string

const (
	StatusUp      TargetStatus = "UP"
	StatusDown    TargetStatus = "DOWN"
	StatusPending TargetStatus = "PENDING"
)

// Target — это отслеживаемый ресурс (сайт, IP или домен)
type Target struct {
	ID        int64         `json:"id" db:"id"`
	URL       string        `json:"url" db:"url"`
	Interval  time.Duration `json:"interval" db:"interval"`     // Интервал проверки
	Status    TargetStatus  `json:"status" db:"status"`         // Текущий статус
	LastCheck time.Time     `json:"last_check" db:"last_check"` // Время последней проверки
	CreatedAt time.Time     `json:"created_at" db:"created_at"` // Время добавления в систему
}

// CheckResult — результат отдельной проверки ресурса.
type CheckResult struct {
	ID         int64        `json:"id" db:"id"`
	TargetID   int64        `json:"target_id" db:"target_id"`       // Связь с Target
	Status     TargetStatus `json:"status" db:"status"`             // UP или DOWN
	StatusCode int          `json:"status_code" db:"status_code"`   // HTTP-код ответа
	LatencyMs  string       `json:"latency_ms" db:"latency_ms"`     // Время отклика в миллисекундах
	ErrMsg     string       `json:"err_msg,omitempty" db:"err_msg"` // Текст ошибки, если ресурс недоступен
	CheckedAt  string       `json:"checked_at" db:"checked_at"`     // Точное время проверки
}

// SubscriptionRequest — то, что клиент шлет в POST-запросе
type SubscriptionRequest struct {
	TargetIDs   []int64  `json:"target_id,omitempty" db:"target_id"`
	URLs        []string `json:"url,omitempty" db:"url"`
	ContactInfo string   `json:"contact_info" db:"contact_info"`
	Period      string   `json:"period,omitempty" db:"period"`
}

// Subscription — модель подписки из базы данных
type Subscription struct {
	ID          int64     `json:"id" db:"id"`
	TargetID    int64     `json:"target_id" db:"target_id"`
	ContactInfo string    `json:"contact_info" db:"contact_info"`
	Period      string    `json:"period,omitempty" db:"period"`
	IsActive    int       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// CheckRequest — структура для проверки списка адресов через API.
type CheckRequest struct {
	Addresses []string `json:"addresses"` // Список URL / IP / доменов для быстрой проверки
}

// CheckResponse — результат выполнения проверки для всех запрошенных адресов.
type CheckResponse struct {
	Results []CheckResult `json:"results"`
}

type CheckResultWithUrl struct {
	CheckResult        // встраиваем базовую структуру
	URL         string `json:"url" db:"url"` // добавляем поле с адресом
}

type SubscriptionListResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}
