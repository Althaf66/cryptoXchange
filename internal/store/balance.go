package store

import (
	"context"
	"database/sql"
)

type BalanceStore struct {
	db *sql.DB
}

type Balance struct {
	UserId string  `json:"userId"`
	Asset  string  `json:"asset"`
	Amount float64 `json:"amount"`
}

func (s *BalanceStore) GetBalanceById(ctx context.Context, userId string) (*Balance, error) {
	query := `SELECT asset, amount FROM balances WHERE userId = $1`
	ctx, cancel := context.WithTimeout(ctx, QueryTimeOutDuration)
	defer cancel()

	b := &Balance{}
	err := s.db.QueryRowContext(ctx, query, userId).Scan(&b.UserId, &b.Asset, &b.Amount)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return nil, ErrUserNotFound
		default:
			return nil, err
		}
	}
	return b, nil
}
