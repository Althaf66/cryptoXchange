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
	// The engine still owns live balances; the ledger is the durable record of
	// how they got that way, and what a reconcile compares them against.
	Trades interface {
		GetRecentTrades(limit int, market string) ([]Trade, error)
		GetTicker(market string) (*Ticker, error)
		GetKlines(interval, market string) ([]Kline, error)
		GetLatestPrice() (float64, error)
	}
	Orders interface {
		ListByUser(userID, market string, limit int) ([]Order, error)
	}
	Transfers interface {
		Create(transfer *Transfer) (bool, error)
		MarkStatus(id, status string) error
		ListByUser(userID string, limit int) ([]Transfer, error)
	}
	Ledger interface {
		Balances() ([]LedgerBalance, error)
	}
}

func NewPostgresStorage(db *sql.DB) Storage {
	return Storage{
		Users:     &UserStore{db},
		Trades:    &TradeStore{db},
		Orders:    &OrderStore{db},
		Transfers: &TransferStore{db},
		Ledger:    &LedgerStore{db},
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
