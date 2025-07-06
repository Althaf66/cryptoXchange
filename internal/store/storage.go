package store

import (
	"context"
	"database/sql"
)

type Storage struct {
	Users interface {
		Create(context.Context, *sql.Tx, *User) error
		GetByID(context.Context, string) (*User, error)
		GetByEmail(context.Context, string) (*User, error)
		Delete(context.Context, string) error
		CreateAndInvite(ctx context.Context, user *User, token string) error
	}
	Balances interface {
		GetBalanceById(ctx context.Context, userId string) (*Balance, error)
	}
	Trades interface {
		GetRecentTrades(limit int, market string) ([]Trade, error)
		GetKlines(interval string) ([]Kline, error)
		GetLatestPrice() (float64, error)
	}
}

func NewPostgresStorage(db *sql.DB) Storage {
	return Storage{
		Users:    &UserStore{db},
		Balances: &BalanceStore{db},
		Trades:   &TradeStore{db},
	}
}

func withTx(db *sql.DB, ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
