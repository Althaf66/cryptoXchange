package store

import (
	"database/sql"

	_ "github.com/lib/pq"
)

type LedgerStore struct {
	db *sql.DB
}

// LedgerBalance is what the ledger believes a user holds in an asset: the sum
// of every recorded movement. It should equal the engine's Available+Locked.
type LedgerBalance struct {
	UserID string  `json:"userId"`
	Asset  string  `json:"asset"`
	Total  float64 `json:"total"`
}

// Balances folds the whole ledger into one row per (user, asset). Only useful
// at demo scale - a real deployment would keep rolling checkpoints rather than
// re-summing every row on each call.
func (l *LedgerStore) Balances() ([]LedgerBalance, error) {
	const query = `
		SELECT user_id, asset, SUM(delta)
		FROM ledger
		GROUP BY user_id, asset
		ORDER BY user_id, asset`

	rows, err := l.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	balances := []LedgerBalance{}
	for rows.Next() {
		var b LedgerBalance
		if err := rows.Scan(&b.UserID, &b.Asset, &b.Total); err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}
	return balances, rows.Err()
}
