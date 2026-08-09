package main

import (
	"database/sql"
	"os"
	"testing"

	"github.com/Althaf66/cryptoXchange/internal/dbase"
)

// compactLedger's guarantee lives entirely in SQL, so this needs a real
// Postgres. Same opt-in as internal/store/transfer_test.go:
//
//	TEST_DB_ADDR='postgres://admin:adminpassword@localhost/cryptoXchange?sslmode=disable'
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}
	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}
	// Registered here rather than deferred by the caller: t.Cleanup is LIFO and
	// runs after the test returns, so a caller's `defer db.Close()` would shut
	// the pool before any row cleanup could use it.
	t.Cleanup(func() { db.Close() })
	return db
}

// A balance is SUM(delta) over the whole ledger, so compaction has exactly two
// obligations: collapse rows, and never change that sum.
//
// The carry rows used to be exempt from their own delete, so every run appended
// a fresh set instead of folding the previous one — unbounded growth that every
// balance read then had to sum over.
func TestCompactLedgerCollapsesItsOwnCarryRows(t *testing.T) {
	db := testDB(t)

	const user = "compact-test-user"
	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM ledger WHERE user_id = $1`, user); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	// Older than the 2-day retention window, so every run sees them as expired.
	for _, row := range []struct {
		asset string
		delta float64
	}{
		{"USD", 1000}, {"USD", -250}, {"USD", 30.5},
		{"SOL", 12}, {"SOL", -4},
	} {
		if _, err := db.Exec(`
			INSERT INTO ledger (user_id, asset, delta, reason, ref_id, created_at)
			VALUES ($1, $2, $3, 'trade', NULL, now() - INTERVAL '3 days')`,
			user, row.asset, row.delta); err != nil {
			t.Fatalf("seed ledger: %v", err)
		}
	}

	sum := func() map[string]float64 {
		t.Helper()
		rows, err := db.Query(
			`SELECT asset, SUM(delta) FROM ledger WHERE user_id = $1 GROUP BY asset`, user)
		if err != nil {
			t.Fatalf("sum: %v", err)
		}
		defer rows.Close()
		out := map[string]float64{}
		for rows.Next() {
			var asset string
			var total float64
			if err := rows.Scan(&asset, &total); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out[asset] = total
		}
		return out
	}

	carryRows := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(
			`SELECT count(*) FROM ledger WHERE user_id = $1 AND reason = 'carry'`, user,
		).Scan(&n); err != nil {
			t.Fatalf("count carry: %v", err)
		}
		return n
	}

	want := sum()
	if len(want) != 2 {
		t.Fatalf("seed produced %d assets, want 2", len(want))
	}

	// Reproducing the growth needs an *aged* carry row meeting newly expired
	// rows — which is what happens hourly in production. A carry row is written
	// with created_at = now(), so simply calling compactLedger repeatedly never
	// re-examines it and both the old and fixed versions look identical.
	for round := 1; round <= 3; round++ {
		if err := compactLedger(db); err != nil {
			t.Fatalf("round %d compact: %v", round, err)
		}
		if got := sum(); !sameBalances(got, want) {
			t.Fatalf("round %d changed the balance: got %v, want %v", round, got, want)
		}
		if n := carryRows(); n > len(want) {
			t.Fatalf("round %d left %d carry rows, want at most one per asset (%d)",
				round, n, len(want))
		}

		// Push the carry rows past the retention window and add another expired
		// batch, standing in for the next hour's worth of aged trades.
		if _, err := db.Exec(`
			UPDATE ledger SET created_at = now() - INTERVAL '3 days'
			WHERE user_id = $1 AND reason = 'carry'`, user); err != nil {
			t.Fatalf("round %d age carry rows: %v", round, err)
		}
		if _, err := db.Exec(`
			INSERT INTO ledger (user_id, asset, delta, reason, ref_id, created_at)
			VALUES ($1, 'USD', 5, 'trade', NULL, now() - INTERVAL '3 days')`,
			user); err != nil {
			t.Fatalf("round %d seed next batch: %v", round, err)
		}
		want["USD"] += 5
	}
}

// Seed rows must survive compaction however the carry rule changes. They carry
// the deterministic ref_id that ledger_seed_uniq protects, which is what stops
// ensureMarkets re-crediting 10M when the engine boots against a surviving
// database with no snapshot. Folding them into a carry row drops that ref and
// silently re-arms the double-count.
func TestCompactLedgerNeverFoldsSeedRows(t *testing.T) {
	db := testDB(t)

	const user = "compact-seed-user"
	const ref = "compact-seed-user:USD"
	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM ledger WHERE user_id = $1`, user); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.Exec(`
		INSERT INTO ledger (user_id, asset, delta, reason, ref_id, created_at)
		VALUES ($1, 'USD', 10000000, 'seed', $2, now() - INTERVAL '30 days')`,
		user, ref); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := compactLedger(db); err != nil {
		t.Fatalf("compact: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM ledger WHERE user_id = $1 AND reason = 'seed' AND ref_id = $2`,
		user, ref).Scan(&n); err != nil {
		t.Fatalf("count seed: %v", err)
	}
	if n != 1 {
		t.Errorf("seed row count = %d after compaction, want 1 (its ref_id must survive)", n)
	}
}

func sameBalances(got, want map[string]float64) bool {
	if len(got) != len(want) {
		return false
	}
	for asset, w := range want {
		g, ok := got[asset]
		// NUMERIC(38,18) round-trips through float64 here, so compare loosely.
		if !ok || g-w > 1e-9 || w-g > 1e-9 {
			return false
		}
	}
	return true
}
