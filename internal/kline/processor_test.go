package kline

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Althaf66/cryptoXchange/internal/dbase"
)

// The order upsert's whole correctness rests on ExecutedQty being a delta that
// accumulates, and on status deriving from the running total. Both are SQL, so
// this needs a real Postgres. Set TEST_DB_ADDR to run it.
func TestOrderUpdateAccumulatesDeltas(t *testing.T) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}

	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}

	orderID := "test-order-" + t.Name()
	t.Cleanup(func() { db.Exec(`DELETE FROM orders WHERE order_id = $1`, orderID) })
	db.Exec(`DELETE FROM orders WHERE order_id = $1`, orderID)

	str := func(s string) *string { return &s }
	apply := func(data OrderUpdateData) {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		handleOrderUpdate(db, raw)
	}

	readOrder := func() (float64, string) {
		t.Helper()
		var executed float64
		var status string
		err := db.QueryRow(
			`SELECT executed_qty, status FROM orders WHERE order_id = $1`, orderID,
		).Scan(&executed, &status)
		if err != nil {
			t.Fatalf("read order: %v", err)
		}
		return executed, status
	}

	// Create: rests on the book having filled nothing.
	apply(OrderUpdateData{
		OrderID:     orderID,
		ExecutedQty: 0,
		Market:      str("SOL_USD"),
		Price:       str("200"),
		Quantity:    str("10"),
		Side:        str("sell"),
		UserID:      str("test-user"),
		Status:      str("open"),
	})
	if executed, status := readOrder(); executed != 0 || status != "open" {
		t.Fatalf("after create: executed=%v status=%q, want 0/open", executed, status)
	}

	// Two maker fills, no identifying fields. These must add, not overwrite.
	apply(OrderUpdateData{OrderID: orderID, ExecutedQty: 4})
	if executed, status := readOrder(); executed != 4 || status != "open" {
		t.Fatalf("after first fill: executed=%v status=%q, want 4/open", executed, status)
	}

	apply(OrderUpdateData{OrderID: orderID, ExecutedQty: 6})
	if executed, status := readOrder(); executed != 10 || status != "filled" {
		t.Fatalf("after second fill: executed=%v status=%q, want 10/filled", executed, status)
	}
}

func TestCancelSetsStatus(t *testing.T) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}

	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}

	orderID := "test-order-" + t.Name()
	t.Cleanup(func() { db.Exec(`DELETE FROM orders WHERE order_id = $1`, orderID) })
	db.Exec(`DELETE FROM orders WHERE order_id = $1`, orderID)

	str := func(s string) *string { return &s }
	apply := func(data OrderUpdateData) {
		t.Helper()
		raw, _ := json.Marshal(data)
		handleOrderUpdate(db, raw)
	}

	apply(OrderUpdateData{
		OrderID: orderID, ExecutedQty: 0,
		Market: str("SOL_USD"), Price: str("200"), Quantity: str("10"),
		Side: str("sell"), UserID: str("test-user"), Status: str("open"),
	})
	apply(OrderUpdateData{OrderID: orderID, ExecutedQty: 0, Status: str("cancelled")})

	var status string
	if err := db.QueryRow(`SELECT status FROM orders WHERE order_id = $1`, orderID).Scan(&status); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("status = %q, want cancelled", status)
	}
}

// Seed credits are re-emitted on every boot without a snapshot, so the ledger
// must reject the duplicates or the demo users' recorded balances double.
func TestSeedLedgerEntriesAreIdempotent(t *testing.T) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}

	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}

	ref := "test-seed-" + t.Name()
	t.Cleanup(func() { db.Exec(`DELETE FROM ledger WHERE ref_id = $1`, ref) })
	db.Exec(`DELETE FROM ledger WHERE ref_id = $1`, ref)

	entry, _ := json.Marshal(LedgerEntryData{
		UserID: "test-user", Asset: "USD", Delta: 10000000,
		Reason: "seed", RefID: ref,
	})
	handleLedgerEntry(db, entry)
	handleLedgerEntry(db, entry)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger WHERE ref_id = $1`, ref).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d seed ledger rows, want 1", count)
	}
}

// Trade legs legitimately share a ref_id across four rows, so the seed
// constraint must not swallow them.
func TestTradeLedgerEntriesShareARef(t *testing.T) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}

	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}

	ref := "test-trade-" + t.Name()
	t.Cleanup(func() { db.Exec(`DELETE FROM ledger WHERE ref_id = $1`, ref) })
	db.Exec(`DELETE FROM ledger WHERE ref_id = $1`, ref)

	legs := []LedgerEntryData{
		{UserID: "a", Asset: "USD", Delta: 100, Reason: "trade", RefID: ref},
		{UserID: "b", Asset: "USD", Delta: -100, Reason: "trade", RefID: ref},
		{UserID: "a", Asset: "SOL", Delta: -0.5, Reason: "trade", RefID: ref},
		{UserID: "b", Asset: "SOL", Delta: 0.5, Reason: "trade", RefID: ref},
	}
	for _, leg := range legs {
		raw, _ := json.Marshal(leg)
		handleLedgerEntry(db, raw)
	}

	var count int
	var sum float64
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(delta), 0) FROM ledger WHERE ref_id = $1`, ref,
	).Scan(&count, &sum); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 4 {
		t.Errorf("got %d trade legs, want 4", count)
	}
	if sum != 0 {
		t.Errorf("trade legs sum to %v, want 0", sum)
	}
}
