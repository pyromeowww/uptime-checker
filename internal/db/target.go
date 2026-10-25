package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

func (s *Store) GetOrCreateTarget(ctx context.Context, url string) (int64, error) {
	queryInsert := `INSERT INTO targets (url, status) VALUES (?, ?) ON CONFLICT(url) DO NOTHING`
	_, err := s.db.ExecContext(ctx, queryInsert, url, domain.StatusPending)
	if err != nil {
		return 0, fmt.Errorf("failed to insert target: %w", err)
	}

	var id int64
	querySelect := `SELECT id FROM targets WHERE url = ?`
	err = s.db.QueryRowContext(ctx, querySelect, url).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to select target id: %w", err)
	}

	return id, nil
}

func (s *Store) SaveResult(ctx context.Context, res domain.CheckResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("transaction error: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	queryInsert := `
    INSERT INTO check_results (target_id, status, status_code, latency_ms, err_msg, checked_at) 
    VALUES (?, ?, ?, ?, ?, ?)
`
	_, err = tx.ExecContext(ctx, queryInsert,
		res.TargetID,
		res.Status,
		res.StatusCode,
		res.LatencyMs,
		res.ErrMsg,
		res.CheckedAt)
	if err != nil {
		return fmt.Errorf("error adding to the table: %w", err)
	}

	queryUpdate := `UPDATE targets SET status = ?, last_check = ? WHERE id = ?`
	_, err = tx.ExecContext(ctx, queryUpdate, res.Status, res.CheckedAt, res.TargetID)
	if err != nil {
		return fmt.Errorf("table update error: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("transaction fixation error: %w", err)
	}
	return nil
}

func (s *Store) GetTargetHistory(ctx context.Context, targetID int64, period time.Duration) ([]domain.CheckResult, error) {
	now := time.Now()
	cutoff := now.Add(-period).Format("2006-01-02 15:04:05")
	query := `SELECT target_id, status, status_code, latency_ms, err_msg, checked_at 
       FROM check_results 
       WHERE target_id = ? AND checked_at >= ? 
       ORDER BY checked_at DESC`
	rows, err := s.db.QueryContext(ctx, query, targetID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("error retrieving rows: %w", err)
	}

	defer func() { _ = rows.Close() }()
	results := make([]domain.CheckResult, 0)
	for rows.Next() {
		var res domain.CheckResult
		err := rows.Scan(
			&res.TargetID,
			&res.Status,
			&res.StatusCode,
			&res.LatencyMs,
			&res.ErrMsg,
			&res.CheckedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan the test result: %w", err)
		}
		results = append(results, res)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unexpected reading error: %w", err)
	}
	return results, nil
}

func (s *Store) DeleteOldResults(ctx context.Context, retentionPeriod time.Duration) (int64, error) {
	now := time.Now()
	cutoff := now.Add(-retentionPeriod).Format("2006-01-02 15:04:05")
	query := `DELETE FROM check_results WHERE checked_at < ?`
	res, err := s.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old results: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve the number of deleted rows: %w", err)
	}

	return rowsAffected, nil
}

func (s *Store) GetHistoryByURL(ctx context.Context, url string, period time.Duration) ([]domain.CheckResult, error) {
	now := time.Now()
	cutoff := now.Add(-period).Format("2006-01-02 15:04:05")

	query := `
        SELECT cr.target_id, cr.status, cr.status_code, cr.latency_ms, cr.err_msg, cr.checked_at
        FROM check_results cr
        JOIN targets t ON cr.target_id = t.id
        WHERE t.url = ? AND cr.checked_at >= ?
        ORDER BY cr.checked_at DESC
    `

	rows, err := s.db.QueryContext(ctx, query, url, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var history []domain.CheckResult
	for rows.Next() {
		var r domain.CheckResult
		if err := rows.Scan(
			&r.TargetID,
			&r.Status,
			&r.StatusCode,
			&r.LatencyMs,
			&r.ErrMsg,
			&r.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan check result: %w", err)
		}
		history = append(history, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	if history == nil {
		history = []domain.CheckResult{}
	}

	return history, nil
}

func (s *Store) GetAllHistory(ctx context.Context, period time.Duration) ([]domain.CheckResultWithUrl, error) {
	now := time.Now()
	cutoff := now.Add(-period).Format("2006-01-02 15:04:05")

	query := `
        SELECT t.url, cr.target_id, cr.status, cr.status_code, cr.latency_ms, cr.err_msg, cr.checked_at
        FROM check_results cr
        JOIN targets t ON cr.target_id = t.id
        WHERE cr.checked_at >= ?
        ORDER BY cr.checked_at DESC
    `

	rows, err := s.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query all history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var history []domain.CheckResultWithUrl
	for rows.Next() {
		var r domain.CheckResultWithUrl
		if err := rows.Scan(
			&r.URL,
			&r.TargetID,
			&r.Status,
			&r.StatusCode,
			&r.LatencyMs,
			&r.ErrMsg,
			&r.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan check result: %w", err)
		}
		history = append(history, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	if history == nil {
		history = []domain.CheckResultWithUrl{}
	}

	return history, nil
}

// CreateSubscriptions сохраняет сайты/ID и подписки.
func (s *Store) CreateSubscriptions(ctx context.Context, req domain.SubscriptionRequest) ([]domain.Subscription, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	period := req.Period
	if period == "" {
		period = "1m"
	}

	targetIDs := make([]int64, 0, len(req.URLs)+len(req.TargetIDs))

	for _, rawURL := range req.URLs {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO targets (url, status) VALUES (?, ?)
			ON CONFLICT(url) DO NOTHING`,
			rawURL, domain.StatusPending,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert target %s: %w", rawURL, err)
		}

		var targetID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM targets WHERE url = ?`, rawURL).Scan(&targetID); err != nil {
			return nil, fmt.Errorf("failed to get target ID for %s: %w", rawURL, err)
		}
		targetIDs = append(targetIDs, targetID)
	}
	for _, id := range req.TargetIDs {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM targets WHERE id = ?`, id).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("target with id %d does not exist", id)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to check target %d: %w", id, err)
		}
		targetIDs = append(targetIDs, id)
	}

	var subscriptions []domain.Subscription
	for _, targetID := range targetIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO subscriptions (target_id, contact_info, period, is_active)
			VALUES (?, ?, ?, 1)
			ON CONFLICT(target_id, contact_info, period) DO UPDATE SET is_active = 1`,
			targetID, req.ContactInfo, period,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert subscription: %w", err)
		}

		var sub domain.Subscription
		err = tx.QueryRowContext(ctx, `
			SELECT id, target_id, contact_info, period, is_active, created_at
			FROM subscriptions
			WHERE target_id = ? AND contact_info = ? AND period = ?`,
			targetID, req.ContactInfo, period,
		).Scan(&sub.ID, &sub.TargetID, &sub.ContactInfo, &sub.Period, &sub.IsActive, &sub.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch subscription: %w", err)
		}
		subscriptions = append(subscriptions, sub)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return subscriptions, nil
}

// GetDueTargets возвращает таргеты, которым пора на проверку.
func (s *Store) GetDueTargets(ctx context.Context) ([]domain.Target, error) {
	const timeLayout = "2006-01-02 15:04:05"

	query := `
		SELECT t.id, t.url, t.status, t.last_check, t.created_at, s.period
		FROM targets t
		JOIN subscriptions s ON t.id = s.target_id
		WHERE s.is_active = 1
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type targetInfo struct {
		target    domain.Target
		minPeriod time.Duration
	}
	infos := make(map[int64]*targetInfo)
	var order []int64

	for rows.Next() {
		var (
			t            domain.Target
			lastCheckStr sql.NullString
			createdAtStr sql.NullString // ← вот это
			periodStr    string
		)
		if err := rows.Scan(&t.ID, &t.URL, &t.Status, &lastCheckStr, &createdAtStr, &periodStr); err != nil {
			return nil, err
		}

		if createdAtStr.Valid && createdAtStr.String != "" {
			if ca, err := time.ParseInLocation(timeLayout, createdAtStr.String, time.UTC); err == nil {
				t.CreatedAt = ca
			}
		}

		period, err := time.ParseDuration(periodStr)
		if err != nil || period <= 0 {
			period = time.Minute
		}

		info, ok := infos[t.ID]
		if !ok {
			info = &targetInfo{target: t, minPeriod: period}
			infos[t.ID] = info
			order = append(order, t.ID)
		}
		if period < info.minPeriod {
			info.minPeriod = period
		}

		if lastCheckStr.Valid && lastCheckStr.String != "" {
			if lc, err := time.ParseInLocation(timeLayout, lastCheckStr.String, time.Local); err == nil {
				info.target.LastCheck = lc
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now()
	due := make([]domain.Target, 0, len(order))
	for _, id := range order {
		info := infos[id]
		if info.target.LastCheck.IsZero() || now.Sub(info.target.LastCheck) >= info.minPeriod {
			due = append(due, info.target)
		}
	}
	return due, nil
}

// GetAllSubscriptions Возвращает все значения из subscriptions
func (s *Store) GetAllSubscriptions(ctx context.Context, limit, offset int) ([]domain.SubscriptionWithURL, error) {
	query := `
		SELECT s.id, s.target_id, s.contact_info, s.period, s.is_active, s.created_at, t.url
		FROM subscriptions s
		JOIN targets t ON t.id = s.target_id
		ORDER BY s.id
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	subs := make([]domain.SubscriptionWithURL, 0)
	for rows.Next() {
		var sub domain.SubscriptionWithURL
		if err := rows.Scan(
			&sub.ID,
			&sub.TargetID,
			&sub.ContactInfo,
			&sub.Period,
			&sub.IsActive,
			&sub.CreatedAt,
			&sub.URL,
		); err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return subs, nil
}

// DeleteAllSubscriptions удаляет все строки из subscriptions.
func (s *Store) DeleteAllSubscriptions(ctx context.Context) (int64, error) {
	query := `DELETE FROM subscriptions`
	res, err := s.db.ExecContext(ctx, query)

	if err != nil {
		return 0, fmt.Errorf("failed to delete all subscriptions: %w", err)
	}
	n, err := res.RowsAffected()

	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return n, nil
}

// DeleteSubscriptionsByURLs удаляет все подписки на target с указанным url.
func (s *Store) DeleteSubscriptionsByURLs(ctx context.Context, urls []string) (int64, error) {
	if len(urls) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(urls))
	args := make([]any, len(urls))
	for i, u := range urls {
		placeholders[i] = "?"
		args[i] = u
	}

	query := `
		DELETE FROM subscriptions
		WHERE target_id IN (
			SELECT id FROM targets WHERE url IN (` + strings.Join(placeholders, ",") + `)
		)
	`
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete subscriptions by urls: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return n, nil
}
