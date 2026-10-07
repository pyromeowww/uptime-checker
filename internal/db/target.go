package db

import (
	"context"
	"fmt"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

func (s *Store) GetOrCreateTarget(ctx context.Context, url string) (int64, error) {
	queryInsert := `INSERT INTO targets (url, status) VALUES (?, ?) ON CONFLICT(url) DO NOTHING`
	res, err := s.db.ExecContext(ctx, queryInsert, url, domain.StatusPending)
	if err != nil {
		return 0, fmt.Errorf("failed to insert target: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id > 0 {
		return id, nil
	}

	querySelect := `SELECT id FROM targets WHERE url = ?`
	err = s.db.QueryRowContext(ctx, querySelect, url).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to select existing target id: %w", err)
	}

	return id, nil
}

func (s *Store) SaveResult(ctx context.Context, res domain.CheckResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка транзакции: %w", err)
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
		return fmt.Errorf("ошибка добавления в таблицу: %w", err)
	}

	queryUpdate := `UPDATE targets SET status = ?, last_check = ? WHERE id = ?`
	_, err = tx.ExecContext(ctx, queryUpdate, res.Status, res.CheckedAt, res.TargetID)
	if err != nil {
		return fmt.Errorf("ошибка обновления таблицы: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}
	return nil
}
