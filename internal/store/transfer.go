package store

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type TransferStore struct {
	db *sql.DB
}

type Transfer struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Asset     string    `json:"asset"`
	Amount    float64   `json:"amount"`
	Direction string    `json:"direction"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// Create claims the idempotency key. It reports false when the key already
// exists, which is the caller's signal that this is a replay and the engine
// must not be told to credit anything a second time.
func (t *TransferStore) Create(transfer *Transfer) (bool, error) {
	const query = `
		INSERT INTO transfers (id, user_id, asset, amount, direction, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		ON CONFLICT (id) DO NOTHING`

	result, err := t.db.Exec(query, transfer.ID, transfer.UserID, transfer.Asset,
		transfer.Amount, transfer.Direction)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (t *TransferStore) MarkStatus(id, status string) error {
	const query = `UPDATE transfers SET status = $2, updated_at = now() WHERE id = $1`
	_, err := t.db.Exec(query, id, status)
	return err
}

func (t *TransferStore) ListByUser(userID string, limit int) ([]Transfer, error) {
	const query = `
		SELECT id, user_id, asset, amount, direction, status, created_at
		FROM transfers
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := t.db.Query(query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transfers := []Transfer{}
	for rows.Next() {
		var tr Transfer
		if err := rows.Scan(&tr.ID, &tr.UserID, &tr.Asset, &tr.Amount,
			&tr.Direction, &tr.Status, &tr.CreatedAt); err != nil {
			return nil, err
		}
		transfers = append(transfers, tr)
	}
	return transfers, rows.Err()
}
