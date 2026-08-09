package main

import (
	"testing"
)

// captureDbMessages swaps the engine's persistence exit point for a recorder,
// so these tests need neither Redis nor Postgres.
func captureDbMessages(t *testing.T) *[]DbMessage {
	t.Helper()
	captured := []DbMessage{}
	original := pushDbMessage
	pushDbMessage = func(m DbMessage) error {
		captured = append(captured, m)
		return nil
	}
	t.Cleanup(func() { pushDbMessage = original })
	return &captured
}

func ledgerEntries(messages []DbMessage) []LedgerEntryData {
	entries := []LedgerEntryData{}
	for _, m := range messages {
		if m.Type == LEDGER_ENTRY {
			entries = append(entries, m.Data.(LedgerEntryData))
		}
	}
	return entries
}

func orderUpdates(messages []DbMessage) []OrderUpdateData {
	updates := []OrderUpdateData{}
	for _, m := range messages {
		if m.Type == ORDER_UPDATE {
			updates = append(updates, m.Data.(OrderUpdateData))
		}
	}
	return updates
}

// totals folds each user's balances down to Available+Locked, which is the
// quantity the ledger claims to track.
func totals(e *Engine) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for user, assets := range e.Balances {
		out[user] = map[string]float64{}
		for asset, b := range assets {
			out[user][asset] = b.Available + b.Locked
		}
	}
	return out
}

// A trade moves value between two users and creates none, so every asset's
// deltas must sum to zero across the whole ledger. This is the invariant the
// reconcile endpoint depends on.
func TestLedgerConservesValueAcrossATrade(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	captured := captureDbMessages(t)
	before := totals(e)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}
	if _, _, _, err := e.CreateOrder(testMarket, "200", "0.5", "buy", "taker", "limit"); err != nil {
		t.Fatalf("buy: %v", err)
	}

	entries := ledgerEntries(*captured)
	if len(entries) == 0 {
		t.Fatal("no ledger entries emitted for a filled trade")
	}

	perAsset := map[string]float64{}
	perUser := map[string]map[string]float64{}
	for _, entry := range entries {
		if entry.Reason != LEDGER_TRADE {
			t.Errorf("unexpected ledger reason %q", entry.Reason)
		}
		if entry.RefID == "" {
			t.Error("ledger entry has no ref_id, cannot be traced to its trade")
		}
		perAsset[entry.Asset] += entry.Delta
		if perUser[entry.UserID] == nil {
			perUser[entry.UserID] = map[string]float64{}
		}
		perUser[entry.UserID][entry.Asset] += entry.Delta
	}

	for asset, sum := range perAsset {
		assertClose(t, "net ledger delta for "+asset, sum, 0)
	}

	// And each user's recorded movement must match what actually happened to
	// their balance. A ledger that balances internally but disagrees with the
	// engine is worse than none.
	after := totals(e)
	for user, assets := range after {
		for asset, total := range assets {
			assertClose(t, user+" "+asset+" ledger vs engine",
				perUser[user][asset], total-before[user][asset])
		}
	}
}

// Locking funds moves value between Available and Locked without changing the
// total, so it must not produce a ledger row. An unfilled resting order is the
// clearest case.
func TestRestingOrderEmitsNoLedgerEntry(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)

	captured := captureDbMessages(t)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	if entries := ledgerEntries(*captured); len(entries) != 0 {
		t.Errorf("locking funds emitted %d ledger entries, want 0: %+v", len(entries), entries)
	}
}

// The processor tells a create from an increment by whether UserID is set, and
// adds ExecutedQty rather than replacing it. Both halves of that contract are
// asserted here because the SQL relying on them lives in another package.
func TestOrderUpdateShape(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	captured := captureDbMessages(t)
	if _, _, _, err := e.CreateOrder(testMarket, "200", "0.5", "buy", "taker", "limit"); err != nil {
		t.Fatalf("buy: %v", err)
	}

	updates := orderUpdates(*captured)
	if len(updates) != 2 {
		t.Fatalf("got %d order updates, want 2 (taker create + one maker fill)", len(updates))
	}

	create := updates[0]
	if create.UserID == nil || *create.UserID != "taker" {
		t.Error("taker's create message must carry UserID so the row can be inserted")
	}
	if create.Quantity == nil || create.Market == nil || create.Side == nil {
		t.Error("create message is missing columns the INSERT needs")
	}
	assertClose(t, "create executed delta", create.ExecutedQty, 0.5)

	fill := updates[1]
	if fill.UserID != nil {
		t.Error("maker fill must leave UserID nil; that is how the processor knows to increment")
	}
	assertClose(t, "maker fill delta", fill.ExecutedQty, 0.5)
}

// A market order's unfilled remainder never rests on the book, so its row has
// to be closed out at once or it lingers in the database as open forever.
func TestMarketOrderIsNeverLeftOpen(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 1)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	captured := captureDbMessages(t)
	// Asks for 3 but only 1 is available.
	if _, _, _, err := e.CreateOrder(testMarket, "0", "3", "buy", "taker", "market"); err != nil {
		t.Fatalf("market buy: %v", err)
	}

	updates := orderUpdates(*captured)
	if len(updates) == 0 {
		t.Fatal("no order updates emitted")
	}
	create := updates[0]
	if create.Status == nil {
		t.Fatal("market order create carried no status")
	}
	// It must be closed out — but not reported as fully filled, which is what
	// the old `!restRemainder` test did for every market order regardless of
	// how little actually executed.
	if *create.Status != "partially_filled" {
		t.Errorf("market order that filled 1 of 3 has status %q, want partially_filled", *create.Status)
	}
}

func TestCancelMarksOrderCancelled(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)

	_, _, orderID, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit")
	if err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	captured := captureDbMessages(t)
	e.handleCancelOrder(MessageFromAPI{
		Type: CANCEL_ORDER,
		Data: CancelOrderData{OrderID: orderID, Market: testMarket},
	}, "test-client")

	updates := orderUpdates(*captured)
	if len(updates) != 1 {
		t.Fatalf("got %d order updates on cancel, want 1", len(updates))
	}
	if updates[0].Status == nil || *updates[0].Status != "cancelled" {
		t.Errorf("cancel status = %v, want cancelled", updates[0].Status)
	}
	if updates[0].OrderID != orderID {
		t.Errorf("cancel targeted %q, want %q", updates[0].OrderID, orderID)
	}
}

// Deposits are the one balance change that creates value, so they must leave a
// ledger row pointing back at the transfer that authorised it.
func TestOnRampRecordsDeposit(t *testing.T) {
	e := newTestEngine(t)
	captured := captureDbMessages(t)

	e.onRamp("newuser", "", 500, "txn-abc")

	entries := ledgerEntries(*captured)
	if len(entries) != 1 {
		t.Fatalf("got %d ledger entries for a deposit, want 1", len(entries))
	}
	if entries[0].Reason != LEDGER_DEPOSIT {
		t.Errorf("reason = %q, want %q", entries[0].Reason, LEDGER_DEPOSIT)
	}
	if entries[0].RefID != "txn-abc" {
		t.Errorf("ref_id = %q, want the transfer id", entries[0].RefID)
	}
	assertClose(t, "deposit delta", entries[0].Delta, 500)
	assertClose(t, "credited balance", bal(t, e, "newuser", "USD").Available, 500)
}

// The header's Add fund panel can pick any asset, so a deposit must land on the
// one it names and leave the quote currency alone.
func TestOnRampCreditsTheNamedAsset(t *testing.T) {
	e := newTestEngine(t)
	captured := captureDbMessages(t)

	e.onRamp("newuser", "SOL", 5, "txn-sol")

	entries := ledgerEntries(*captured)
	if len(entries) != 1 {
		t.Fatalf("got %d ledger entries for a deposit, want 1", len(entries))
	}
	if entries[0].Asset != "SOL" {
		t.Errorf("ledger asset = %q, want %q", entries[0].Asset, "SOL")
	}
	assertClose(t, "credited SOL", bal(t, e, "newuser", "SOL").Available, 5)
	if _, credited := e.Balances["newuser"]["USD"]; credited {
		t.Error("a SOL deposit created a USD balance")
	}
}
