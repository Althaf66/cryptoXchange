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

	// The market maker prints ~20 trades a minute forever, so old chunks have to
	// go. This is Timescale's own background job - the plain tables are pruned by
	// the cron in cmd/api/cron.go instead. Consequence worth knowing: the 1w
	// candle view can never show more than two days.
	retentionQuery := `SELECT add_retention_policy('sol_prices', INTERVAL '2 days', if_not_exists => TRUE);`
	if _, err := db.Exec(retentionQuery); err != nil {
		return fmt.Errorf("failed to add retention policy: %v", err)
	}

	// Create materialized views, bucketed per market.
	//
	// These are dropped first, not created IF NOT EXISTS: an earlier version
	// grouped by currency_code, a column the insert path never writes, so every
	// market collapsed into one NULL-keyed candle series. IF NOT EXISTS would
	// silently keep that definition on an existing database. Dropping is free -
	// they are pure aggregates over sol_prices and the cron in cmd/api/cron.go
	// rebuilds them within 10 seconds.
	materializedViews := []string{
		`DROP MATERIALIZED VIEW IF EXISTS klines_1m;
		CREATE MATERIALIZED VIEW klines_1m AS
		SELECT
			time_bucket('1 minute', time) AS bucket,
			first(price, time) AS open,
			max(price) AS high,
			min(price) AS low,
			last(price, time) AS close,
			sum(volume) AS volume,
			market
		FROM sol_prices
		GROUP BY bucket, market;`,

		`DROP MATERIALIZED VIEW IF EXISTS klines_1h;
		CREATE MATERIALIZED VIEW klines_1h AS
		SELECT
			time_bucket('1 hour', time) AS bucket,
			first(price, time) AS open,
			max(price) AS high,
			min(price) AS low,
			last(price, time) AS close,
			sum(volume) AS volume,
			market
		FROM sol_prices
		GROUP BY bucket, market;`,

		// No klines_1w. sol_prices keeps two days (see the retention policy
		// above), so a one-week bucket could never be more than a fragment —
		// the chart rendered it as a single candle covering everything, which
		// reads as a broken chart rather than a week view. Dropped here so an
		// existing database loses the stale view too.
		`DROP MATERIALIZED VIEW IF EXISTS klines_1w;`,
	}

	for _, query := range materializedViews {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create materialized view: %v", err)
		}
	}

	log.Println("TimescaleDB initialized successfully")
	return nil
}

// InitializeExchangeTables creates the durable records the matching engine cannot
// keep in memory: every order, every movement of value, and every deposit.
//
// None of these carry a foreign key to users. Engine user IDs are demo strings
// ("1", "2", "5") seeded by ensureMarkets in cmd/engine/engine.go and were never
// rows in the users table - adding the FK would reject every seeded order.
//
// Money is NUMERIC(38,18) even though the engine works in float64: the durable
// side is where precision has to survive. Converting the engine off floats is a
// separate change.
func InitializeExchangeTables(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS orders (
			order_id     TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL,
			market       TEXT NOT NULL,
			side         TEXT NOT NULL,
			price        NUMERIC(38,18) NOT NULL,
			quantity     NUMERIC(38,18) NOT NULL,
			executed_qty NUMERIC(38,18) NOT NULL DEFAULT 0,
			status       TEXT NOT NULL DEFAULT 'open',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
		`CREATE INDEX IF NOT EXISTS orders_user_market_idx
			ON orders (user_id, market, created_at DESC);`,
		// The hourly prune filters on created_at alone, which cannot use the
		// index above — its leading column is user_id.
		`CREATE INDEX IF NOT EXISTS orders_created_at_idx ON orders (created_at);`,

		// Append-only. A balance is SUM(delta), never an UPDATE.
		`CREATE TABLE IF NOT EXISTS ledger (
			id         BIGSERIAL PRIMARY KEY,
			user_id    TEXT NOT NULL,
			asset      TEXT NOT NULL,
			delta      NUMERIC(38,18) NOT NULL,
			reason     TEXT NOT NULL,
			ref_id     TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
		`CREATE INDEX IF NOT EXISTS ledger_user_asset_idx ON ledger (user_id, asset);`,
		// Same reason as orders_created_at_idx: compactLedger selects and
		// deletes by created_at, which the (user_id, asset) index cannot serve.
		`CREATE INDEX IF NOT EXISTS ledger_created_at_idx ON ledger (created_at);`,
		// Seed credits are re-emitted whenever the engine boots without a
		// snapshot. This makes those inserts idempotent so a wiped snapshot
		// against a surviving database does not double the demo balances in
		// the ledger. Only seed rows are constrained; real deposits and trades
		// legitimately repeat a ref_id across their legs.
		`CREATE UNIQUE INDEX IF NOT EXISTS ledger_seed_uniq
			ON ledger (ref_id) WHERE reason = 'seed';`,

		// id is the idempotency key, so a replayed deposit collides instead of
		// crediting twice.
		`CREATE TABLE IF NOT EXISTS transfers (
			id         TEXT PRIMARY KEY,
			user_id    TEXT NOT NULL,
			asset      TEXT NOT NULL,
			amount     NUMERIC(38,18) NOT NULL,
			direction  TEXT NOT NULL,
			status     TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
		`CREATE INDEX IF NOT EXISTS transfers_user_idx ON transfers (user_id, created_at DESC);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to create exchange tables: %v", err)
		}
	}

	log.Println("exchange tables initialized successfully")
	return nil
}
