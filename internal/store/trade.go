package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type TradeStore struct {
	db *sql.DB
}

// Kline is one candle. Start/End bound the bucket; the chart keys candles off
// End, so it must be present.
type Kline struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
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

// Ticker is the 24h rollup the markets page and market bar render. Field names
// match the frontend's Ticker type in frontend/app/utils/types.ts.
type Ticker struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	High               string `json:"high"`
	Low                string `json:"low"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	Trades             string `json:"trades"`
	FirstPrice         string `json:"firstPrice"`
}

// GetTicker rolls up the last 24h of trades for a market. Returns a zeroed
// ticker (not an error) when there are no trades yet, so an empty demo database
// still renders a row instead of a crash.
func (t *TradeStore) GetTicker(market string) (*Ticker, error) {
	query := `
		SELECT
			COALESCE(MAX(high.price), 0),
			COALESCE(MIN(high.price), 0),
			COALESCE(SUM(high.volume), 0),
			COALESCE(SUM(high.price * high.volume), 0),
			COUNT(*),
			COALESCE((SELECT price FROM sol_prices WHERE market = $1 AND time > NOW() - INTERVAL '24 hours' ORDER BY time DESC LIMIT 1), 0),
			COALESCE((SELECT price FROM sol_prices WHERE market = $1 AND time > NOW() - INTERVAL '24 hours' ORDER BY time ASC  LIMIT 1), 0)
		FROM sol_prices high
		WHERE high.market = $1 AND high.time > NOW() - INTERVAL '24 hours'`

	var high, low, volume, quoteVolume, last, first float64
	var trades int64
	err := t.db.QueryRow(query, market).Scan(
		&high, &low, &volume, &quoteVolume, &trades, &last, &first)
	if err != nil {
		return nil, err
	}

	change := last - first
	changePercent := 0.0
	if first > 0 {
		changePercent = change / first * 100
	}

	// Round at 8 decimals and trim: summing quantities in SQL accumulates float
	// noise, and a DOGE volume reads as 4000.0001999999995 without this.
	f := func(v float64) string {
		s := strconv.FormatFloat(v, 'f', 8, 64)
		if strings.Contains(s, ".") {
			s = strings.TrimRight(s, "0")
			s = strings.TrimSuffix(s, ".")
		}
		return s
	}
	return &Ticker{
		Symbol:             market,
		LastPrice:          f(last),
		High:               f(high),
		Low:                f(low),
		Volume:             f(volume),
		QuoteVolume:        f(quoteVolume),
		PriceChange:        f(change),
		PriceChangePercent: f(changePercent),
		Trades:             strconv.FormatInt(trades, 10),
		FirstPrice:         f(first),
	}, nil
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

func (t *TradeStore) GetKlines(interval, market string) ([]Kline, error) {
	var tableName string
	var bucketSize time.Duration
	switch interval {
	case "1m":
		tableName, bucketSize = "klines_1m", time.Minute
	case "1h":
		tableName, bucketSize = "klines_1h", time.Hour
	default:
		return nil, fmt.Errorf("invalid interval: %s", interval)
	}

	query := fmt.Sprintf(`
		SELECT bucket, open, high, low, close, volume
		FROM %s
		WHERE market = $1
		ORDER BY bucket DESC
		LIMIT 100`, tableName)

	rows, err := t.db.Query(query, market)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klines := []Kline{}
	for rows.Next() {
		var k Kline
		err := rows.Scan(&k.Start, &k.Open, &k.High, &k.Low, &k.Close, &k.Volume)
		if err != nil {
			return nil, err
		}
		k.End = k.Start.Add(bucketSize)
		klines = append(klines, k)
	}

	return klines, rows.Err()
}

func (t *TradeStore) GetLatestPrice() (float64, error) {
	var price float64
	query := `SELECT price FROM sol_prices ORDER BY time DESC LIMIT 1`
	err := t.db.QueryRow(query).Scan(&price)
	return price, err
}
