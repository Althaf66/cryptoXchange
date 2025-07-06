package dbase

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// var db *sql.DB

func New(addr string, maxOpenConns, maxIdleConns int, maxIdleTime string) (*sql.DB, error) {
	db, err := sql.Open("postgres", addr)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	duration, err := time.ParseDuration(maxIdleTime)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func InitializeKlineDB(db *sql.DB) error {
	createTableQuery := `
		CREATE TABLE IF NOT EXISTS "sol_prices"(
			id              VARCHAR(50) NOT NULL,
			time            TIMESTAMP WITH TIME ZONE NOT NULL,
			price           DOUBLE PRECISION,
			volume          DOUBLE PRECISION,
			currency_code   VARCHAR (10),
			market          VARCHAR (20),
			is_buyer_maker  BOOLEAN DEFAULT FALSE,
			PRIMARY KEY (id, time)
		);`
	if _, err := db.Exec(createTableQuery); err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// Check if hypertable exists
	var exists bool
	checkHypertableQuery := `
		SELECT EXISTS (
			SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'sol_prices'
		);`
	if err := db.QueryRow(checkHypertableQuery).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check if hypertable exists: %v", err)
	}

	// Create hypertable if not exists
	if !exists {
		hypertableQuery := `SELECT create_hypertable('sol_prices', 'time');`
		if _, err := db.Exec(hypertableQuery); err != nil {
			return fmt.Errorf("failed to create hypertable: %v", err)
		}
	}

	// Create materialized views
	materializedViews := []string{
		`CREATE MATERIALIZED VIEW IF NOT EXISTS klines_1m AS
		SELECT
			time_bucket('1 minute', time) AS bucket,
			first(price, time) AS open,
			max(price) AS high,
			min(price) AS low,
			last(price, time) AS close,
			sum(volume) AS volume,
			currency_code
		FROM sol_prices
		GROUP BY bucket, currency_code;`,

		`CREATE MATERIALIZED VIEW IF NOT EXISTS klines_1h AS
		SELECT
			time_bucket('1 hour', time) AS bucket,
			first(price, time) AS open,
			max(price) AS high,
			min(price) AS low,
			last(price, time) AS close,
			sum(volume) AS volume,
			currency_code
		FROM sol_prices
		GROUP BY bucket, currency_code;`,

		`CREATE MATERIALIZED VIEW IF NOT EXISTS klines_1w AS
		SELECT
			time_bucket('1 week', time) AS bucket,
			first(price, time) AS open,
			max(price) AS high,
			min(price) AS low,
			last(price, time) AS close,
			sum(volume) AS volume,
			currency_code
		FROM sol_prices
		GROUP BY bucket, currency_code;`,
	}

	for _, query := range materializedViews {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create materialized view: %v", err)
		}
	}

	log.Println("TimescaleDB initialized successfully")
	return nil
}
