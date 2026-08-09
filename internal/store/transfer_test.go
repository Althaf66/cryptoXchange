package store

import (
	"os"
	"testing"

	"github.com/Althaf66/cryptoXchange/internal/dbase"
)

// TestTransferIdempotency needs a real Postgres because the guarantee lives in
// the ON CONFLICT clause, not in Go. Set TEST_DB_ADDR to run it, e.g.
//
//	TEST_DB_ADDR='postgres://admin:adminpassword@localhost/cryptoXchange?sslmode=disable'
func TestTransferIdempotency(t *testing.T) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		t.Skip("TEST_DB_ADDR not set")
	}

	db, err := dbase.New(addr, 5, 5, "1m")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// t.Cleanup runs after the test returns, so closing via defer here would
	// shut the pool before the row cleanup below could use it.
	t.Cleanup(func() { db.Close() })

	if err := dbase.InitializeExchangeTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}

	store := &TransferStore{db}
	transfer := &Transfer{
		ID:        "test-txn-" + t.Name(),
		UserID:    "test-user",
		Asset:     "USD",
		Amount:    100,
		Direction: "deposit",
	}
	// Clear both before and after: a previous run that died mid-test would
	// otherwise make the first Create look like a replay.
	db.Exec(`DELETE FROM transfers WHERE id = $1`, transfer.ID)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM transfers WHERE id = $1`, transfer.ID)
	})

	claimed, err := store.Create(transfer)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if !claimed {
		t.Fatal("first create did not claim the key")
	}

	// Same key again: this is what stops a retried deposit crediting twice.
	claimed, err = store.Create(transfer)
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if claimed {
		t.Error("replay claimed the key; the deposit would be credited twice")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers WHERE id = $1`, transfer.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d transfer rows, want 1", count)
	}
}
