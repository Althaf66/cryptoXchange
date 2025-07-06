package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type TradeStore struct {
	db *sql.DB
}

type Kline struct {
	Bucket       time.Time `json:"bucket" db:"bucket"`
	Open         float64   `json:"open" db:"open"`
	High         float64   `json:"high" db:"high"`
	Low          float64   `json:"low" db:"low"`
	Close        float64   `json:"close" db:"close"`
	Volume       float64   `json:"volume" db:"volume"`
	CurrencyCode string    `json:"currency_code" db:"currency_code"`
}

type Trade struct {
	ID           string    `json:"id" db:"id"`
	Price        float64   `json:"price" db:"price"`
	Volume       float64   `json:"volume" db:"volume"`
	Timestamp    time.Time `json:"timestamp" db:"time"`
	Market       string    `json:"market" db:"market"`
	IsBuyerMaker bool      `json:"is_buyer_maker" db:"is_buyer_maker"`
	// QuoteQuantity float64   `json:"quote_quantity" db:"quote_quantity"`
}

func (t *TradeStore) GetRecentTrades(limit int, market string) ([]Trade, error) {
	var query string
	var args []interface{}

	if market != "" {
		query = `
			SELECT id, price, volume, time, market, is_buyer_maker
			FROM sol_prices 
			WHERE market = $1
			ORDER BY time DESC 
			LIMIT $2`
		args = []interface{}{market, limit}
	} else {
		query = `
			SELECT id, price, volume, time, market, is_buyer_maker
			FROM sol_prices 
			ORDER BY time DESC 
			LIMIT $1`
		args = []interface{}{limit}
	}

	rows, err := t.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []Trade
	for rows.Next() {
		var trade Trade
		err := rows.Scan(&trade.ID, &trade.Price, &trade.Volume, &trade.Timestamp,
			&trade.Market, &trade.IsBuyerMaker)
		if err != nil {
			return nil, err
		}
		trades = append(trades, trade)
	}

	return trades, nil
}

func (t *TradeStore) GetKlines(interval string) ([]Kline, error) {
	var tableName string
	switch interval {
	case "1m":
		tableName = "klines_1m"
	case "1h":
		tableName = "klines_1h"
	case "1w":
		tableName = "klines_1w"
	default:
		return nil, fmt.Errorf("invalid interval: %s", interval)
	}

	query := fmt.Sprintf(`
		SELECT bucket, open, high, low, close, volume
		FROM %s 
		ORDER BY bucket DESC 
		LIMIT 100`, tableName)

	rows, err := t.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var klines []Kline
	for rows.Next() {
		var k Kline
		err := rows.Scan(&k.Bucket, &k.Open, &k.High, &k.Low, &k.Close, &k.Volume)
		if err != nil {
			return nil, err
		}
		klines = append(klines, k)
	}

	return klines, nil
}

func (t *TradeStore) GetLatestPrice() (float64, error) {
	var price float64
	query := `SELECT price FROM sol_prices ORDER BY time DESC LIMIT 1`
	err := t.db.QueryRow(query).Scan(&price)
	return price, err
}
