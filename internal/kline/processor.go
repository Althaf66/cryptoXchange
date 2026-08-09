package kline

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/rediscfg"
	"github.com/go-redis/redis/v8"
)

func StartDataProcessor(db *sql.DB) {
	rdb := redis.NewClient(rediscfg.Options())

	ctx := context.Background()

	// Test Redis connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	log.Println("Connected to Redis")

	for {
		// Block and wait for messages from Redis list
		result, err := rdb.BRPop(ctx, 0, "db_processor").Result()
		if err != nil {
			log.Printf("Error reading from Redis: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if len(result) < 2 {
			continue
		}

		message := result[1] // BRPop returns [key, value]

		var dbMessage DbMessage
		if err := json.Unmarshal([]byte(message), &dbMessage); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		// Re-marshal once: every branch below decodes dbMessage.Data, which
		// arrived as a generic map.
		dataBytes, err := json.Marshal(dbMessage.Data)
		if err != nil {
			log.Printf("Error marshaling message data: %v", err)
			continue
		}

		switch dbMessage.Type {
		case "TRADE_ADDED":
			handleTradeAdded(db, dataBytes)
		case "ORDER_UPDATE":
			handleOrderUpdate(db, dataBytes)
		case "LEDGER_ENTRY":
			handleLedgerEntry(db, dataBytes)
		}
	}
}

func handleTradeAdded(db *sql.DB, dataBytes []byte) {
	log.Println("Adding trade data")

	var tradeData TradeData
	if err := json.Unmarshal(dataBytes, &tradeData); err != nil {
		log.Printf("Error unmarshaling trade data: %v", err)
		return
	}

	price, err := strconv.ParseFloat(tradeData.Price, 64)
	if err != nil {
		log.Printf("Error parsing price: %v", err)
		return
	}

	volume, err := strconv.ParseFloat(tradeData.Quantity, 64)
	if err != nil {
		log.Printf("Error parsing volume: %v", err)
		return
	}

	timestamp := time.Unix(tradeData.Timestamp/1000, (tradeData.Timestamp%1000)*1000000)

	if err := insertTrade(db, tradeData, price, volume); err != nil {
		log.Printf("Error inserting trade: %v", err)
	} else {
		log.Printf("Inserted trade: price=%.2f, volume=%.2f, time=%s",
			price, volume, timestamp.Format(time.RFC3339))
	}
}

// handleOrderUpdate applies one delta to an order row. A message carrying
// UserID is the order's create and inserts the row; one without it is an
// increment (a maker fill, or a cancellation carrying only a status).
func handleOrderUpdate(db *sql.DB, dataBytes []byte) {
	var data OrderUpdateData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		log.Printf("Error unmarshaling order update: %v", err)
		return
	}
	if data.OrderID == "" {
		return
	}

	if data.UserID == nil {
		// Increment against an existing row. $3 lets a cancel force the status
		// while a plain fill leaves the derivation to the CASE.
		const q = `
			UPDATE orders SET
				executed_qty = executed_qty + $2,
				status = CASE
					WHEN $3::text IS NOT NULL THEN $3::text
					WHEN executed_qty + $2 >= quantity THEN 'filled'
					ELSE status
				END,
				updated_at = now()
			WHERE order_id = $1`
		if _, err := db.Exec(q, data.OrderID, data.ExecutedQty, data.Status); err != nil {
			log.Printf("Error updating order %s: %v", data.OrderID, err)
		}
		return
	}

	price, quantity := 0.0, 0.0
	if data.Price != nil {
		price, _ = strconv.ParseFloat(*data.Price, 64)
	}
	if data.Quantity != nil {
		quantity, _ = strconv.ParseFloat(*data.Quantity, 64)
	}

	status := "open"
	if data.Status != nil {
		status = *data.Status
	}

	// ON CONFLICT rather than a plain INSERT: the engine can be restarted from a
	// snapshot that still holds an order id already written here.
	const q = `
		INSERT INTO orders (order_id, user_id, market, side, price, quantity, executed_qty, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (order_id) DO UPDATE SET
			executed_qty = orders.executed_qty + EXCLUDED.executed_qty,
			status = CASE
				WHEN orders.executed_qty + EXCLUDED.executed_qty >= orders.quantity THEN 'filled'
				-- The engine's own verdict wins when it is terminal. Without
				-- this branch EXCLUDED.status was never consulted at all, so a
				-- market order replayed after a snapshot restore came back as
				-- 'open' and showed in open orders despite not being on the book.
				WHEN EXCLUDED.status IN ('filled', 'partially_filled', 'cancelled') THEN EXCLUDED.status
				ELSE orders.status
			END,
			updated_at = now()`
	_, err := db.Exec(q, data.OrderID, *data.UserID, derefOr(data.Market), derefOr(data.Side),
		price, quantity, data.ExecutedQty, status)
	if err != nil {
		log.Printf("Error inserting order %s: %v", data.OrderID, err)
	}
}

// handleLedgerEntry appends one movement of value. ON CONFLICT DO NOTHING is
// there for the seed credits, which the engine re-emits on any boot without a
// snapshot and which carry a deterministic ref_id (see ledger_seed_uniq).
func handleLedgerEntry(db *sql.DB, dataBytes []byte) {
	var data LedgerEntryData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		log.Printf("Error unmarshaling ledger entry: %v", err)
		return
	}

	const q = `
		INSERT INTO ledger (user_id, asset, delta, reason, ref_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`
	if _, err := db.Exec(q, data.UserID, data.Asset, data.Delta, data.Reason, data.RefID); err != nil {
		log.Printf("Error inserting ledger entry for %s/%s: %v", data.UserID, data.Asset, err)
	}
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func insertTrade(db *sql.DB, tradeData TradeData, price float64, volume float64) error {
	timestamp := time.Unix(tradeData.Timestamp/1000, (tradeData.Timestamp%1000)*1000000)

	query := `INSERT INTO sol_prices (id, time, price, volume, market, is_buyer_maker) 
			  VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.Exec(query, tradeData.ID, timestamp, price, volume, tradeData.Market, tradeData.IsBuyerMaker)
	return err
}
