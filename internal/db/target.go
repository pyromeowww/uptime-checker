package db

import (
	"context"
	"fmt"
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
	defer tx.Rollback()

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
	now := time.Now().UTC()
	cutoff := now.Add(-period).Format("2006-01-02 15:04:05")
	query := `SELECT target_id, status, status_code, latency_ms, err_msg, checked_at 
		FROM check_results 
		WHERE target_id = ? AND checked_at >= ? 
		ORDER BY checked_at DESC`
	rows, err := s.db.QueryContext(ctx, query, targetID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("error retrieving rows: %w", err)
	}

	defer rows.Close()
	results := []domain.CheckResult{}
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
	now := time.Now().UTC()
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

func (s *Store) GetHistoryByUrl(ctx context.Context, url string, period time.Duration) ([]domain.CheckResult, error) {
	now := time.Now().UTC()
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
	defer rows.Close()

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

	// Возвращаем пустой слайс вместо nil, чтобы в JSON красиво отдавался []
	if history == nil {
		history = []domain.CheckResult{}
	}

	return history, nil
}

func (s *Store) GetAllHistory(ctx context.Context, period time.Duration) ([]domain.CheckResultWithUrl, error) {
	now := time.Now().UTC()
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
	defer rows.Close()

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
