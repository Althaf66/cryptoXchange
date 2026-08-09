package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// retention is how much history the demo keeps. The market maker prints ~20
// trades a minute, so without this the tables grow forever.
const retention = "2 days"

func startCronJob(db *sql.DB) {
	// These refresh without CONCURRENTLY (which would need a unique index on
	// each view), so each one takes an ACCESS EXCLUSIVE lock and blocks every
	// concurrent /klines read. At 10s that was three stalls per 10s window.
	views := time.NewTicker(60 * time.Second)
	defer views.Stop()
	// Hourly is plenty for a two-day window, and keeps the delete off the 10s path.
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()

	pruneOldData(db)

	for {
		select {
		case <-views.C:
			refreshMaterializedViews(db)
		case <-prune.C:
			pruneOldData(db)
		}
	}
}

// pruneOldData drops demo history older than the retention window.
//
// sol_prices is not handled here: it is a hypertable with a native retention
// policy (see dbase.InitializeKlineDB), which Timescale's own background worker
// enforces by dropping whole chunks.
func pruneOldData(db *sql.DB) {
	if _, err := db.Exec(
		`DELETE FROM orders WHERE created_at < now() - $1::interval`, retention); err != nil {
		log.Printf("Error pruning orders: %v", err)
	}

	if err := compactLedger(db); err != nil {
		log.Printf("Error compacting ledger: %v", err)
	}
}

// compactLedger folds expiring ledger rows into one carry-forward row per
// (user, asset) instead of deleting them outright.
//
// A balance is SUM(delta) over the whole table (internal/store/ledger.go), so a
// plain delete would make /admin/reconcile report every user as drifting by
// exactly the amount removed. Summing first keeps that total identical while
// collapsing the row count - the "rolling checkpoint" the ledger store's own
// comment calls for.
func compactLedger(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Seed rows are exempt, and must stay that way. They carry a deterministic
	// ref_id protected by the ledger_seed_uniq index, which is what stops
	// ensureMarkets re-crediting 10M when the engine boots against a surviving
	// database with no snapshot. Folding them into a carry row drops that ref
	// and quietly re-arms the double-count. They are bounded anyway - users ×
	// assets - so they never grow enough to be worth reclaiming.
	//
	// Carry rows ARE re-folded. Exempting them made each hourly run append a
	// fresh set instead of collapsing the previous one — about 5,000 rows a
	// week, growing forever, all of which every balance read has to SUM over.
	// Including them keeps exactly one carry row per (user, asset).
	//
	// The new carry row is written with created_at = now(), so it is younger
	// than the retention window and survives its own delete in the same
	// transaction.
	const notCompacted = `reason <> 'seed'`

	if _, err := tx.Exec(`
		INSERT INTO ledger (user_id, asset, delta, reason, ref_id)
		SELECT user_id, asset, SUM(delta), 'carry', NULL
		FROM ledger
		WHERE created_at < now() - $1::interval AND `+notCompacted+`
		GROUP BY user_id, asset`, retention); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM ledger
		WHERE created_at < now() - $1::interval AND `+notCompacted, retention); err != nil {
		return err
	}

	return tx.Commit()
}

func refreshMaterializedViews(db *sql.DB) {
	// klines_1w is deliberately absent: sol_prices keeps two days (see the
	// retention policy in dbase.InitializeKlineDB), so a one-week bucket can
	// never be complete. Refreshing it only bought a lock and a wrong candle.
	views := []string{
		"klines_1m",
		"klines_1h",
	}

	for _, view := range views {
		query := fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", view)
		if _, err := db.Exec(query); err != nil {
			log.Printf("Error refreshing view %s: %v", view, err)
		}
	}

	// log.Println("Materialized views refreshed successfully")
}
