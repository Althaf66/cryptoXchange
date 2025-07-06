package kline

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

func StartDataProcessor(db *sql.DB) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

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

		if dbMessage.Type == "TRADE_ADDED" {
			log.Println("Adding trade data")

			// Parse the data as TradeData
			dataBytes, err := json.Marshal(dbMessage.Data)
			if err != nil {
				log.Printf("Error marshaling trade data: %v", err)
				continue
			}

			var tradeData TradeData
			if err := json.Unmarshal(dataBytes, &tradeData); err != nil {
				log.Printf("Error unmarshaling trade data: %v", err)
				continue
			}

			price, err := strconv.ParseFloat(tradeData.Price, 64)
			if err != nil {
				log.Printf("Error parsing price: %v", err)
				continue
			}

			volume, err := strconv.ParseFloat(tradeData.Quantity, 64)
			if err != nil {
				log.Printf("Error parsing volume: %v", err)
				continue
			}

			timestamp := time.Unix(tradeData.Timestamp/1000, (tradeData.Timestamp%1000)*1000000)

			if err := insertTrade(db, tradeData, price, volume); err != nil {
				log.Printf("Error inserting trade: %v", err)
			} else {
				log.Printf("Inserted trade: price=%.2f, volume=%.2f, time=%s",
					price, volume, timestamp.Format(time.RFC3339))
			}
		}
	}
}

func insertTrade(db *sql.DB, tradeData TradeData, price float64, volume float64) error {
	timestamp := time.Unix(tradeData.Timestamp/1000, (tradeData.Timestamp%1000)*1000000)

	query := `INSERT INTO sol_prices (id, time, price, volume, market, is_buyer_maker) 
			  VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.Exec(query, tradeData.ID, timestamp, price, volume, tradeData.Market, tradeData.IsBuyerMaker)
	return err
}
