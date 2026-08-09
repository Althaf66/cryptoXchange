package main

import "testing"

// captureReplies redirects sendToAPI into a slice for the duration of a test.
// The property under test is not the payload shape but simply that a handler
// answered: the API blocks on a pub/sub reply, so a handler that returns
// without replying costs the caller its full 30s timeout and two retries.
func captureReplies(t *testing.T) *[]MessageToAPI {
	t.Helper()
	original := sendToAPI
	got := []MessageToAPI{}
	sendToAPI = func(clientID string, m MessageToAPI) error {
		got = append(got, m)
		return nil
	}
	t.Cleanup(func() { sendToAPI = original })
	return &got
}

func onlyReply(t *testing.T, got *[]MessageToAPI) MessageToAPI {
	t.Helper()
	if len(*got) != 1 {
		t.Fatalf("expected exactly one reply, got %d: %+v", len(*got), *got)
	}
	return (*got)[0]
}

// The market maker cancels already-filled levels every tick by design, so this
// path is hot, not exceptional.
func TestCancelMissingOrderReplies(t *testing.T) {
	e := newTestEngine(t)
	got := captureReplies(t)

	e.Process(MessageFromAPI{
		Type: CANCEL_ORDER,
		Data: CancelOrderData{OrderID: "nosuchid", Market: testMarket},
	}, "client-1")

	reply := onlyReply(t, got)
	if reply.Type != "ORDER_REJECTED" {
		t.Fatalf("got type %q, want ORDER_REJECTED", reply.Type)
	}
	if code := reply.Payload.(OrderRejectedPayload).Code; code != "ORDER_NOT_FOUND" {
		t.Errorf("got code %q, want ORDER_NOT_FOUND", code)
	}
}

func TestCancelUnknownMarketReplies(t *testing.T) {
	e := newTestEngine(t)
	got := captureReplies(t)

	e.Process(MessageFromAPI{
		Type: CANCEL_ORDER,
		Data: CancelOrderData{OrderID: "x", Market: "NOPE_USD"},
	}, "client-1")

	if onlyReply(t, got).Type != "ORDER_REJECTED" {
		t.Fatal("cancel on an unknown market must reply, not drop")
	}
}

func TestUnknownCommandReplies(t *testing.T) {
	e := newTestEngine(t)
	got := captureReplies(t)

	e.Process(MessageFromAPI{Type: "NOT_A_COMMAND"}, "client-1")

	if onlyReply(t, got).Type != "ORDER_REJECTED" {
		t.Fatal("an unrecognised command must reply, not drop")
	}
}

// Cancelling a partially filled order used to report zeros for both fields.
func TestCancelReportsRealQuantities(t *testing.T) {
	e := newTestEngine(t)
	ob := e.Orderbooks[testMarket]

	maker := Order{Price: 200, Quantity: 10, OrderID: "maker-1", Side: "sell", UserID: "1"}
	if _, _, err := ob.AddOrder(maker, true); err != nil {
		t.Fatalf("resting the maker failed: %v", err)
	}
	// Cross 4 of the 10, leaving 6 outstanding.
	taker := Order{Price: 200, Quantity: 4, OrderID: "taker-1", Side: "buy", UserID: "2"}
	if _, _, err := ob.AddOrder(taker, false); err != nil {
		t.Fatalf("crossing the maker failed: %v", err)
	}
	fund(e, "1", 0, 10)

	got := captureReplies(t)
	e.Process(MessageFromAPI{
		Type: CANCEL_ORDER,
		Data: CancelOrderData{OrderID: "maker-1", Market: testMarket},
	}, "client-1")

	reply := onlyReply(t, got)
	if reply.Type != "ORDER_CANCELLED" {
		t.Fatalf("got type %q, want ORDER_CANCELLED: %+v", reply.Type, reply.Payload)
	}
	p := reply.Payload.(OrderCancelledPayload)
	if p.ExecutedQty != 4 {
		t.Errorf("ExecutedQty = %v, want 4", p.ExecutedQty)
	}
	if p.RemainingQty != 6 {
		t.Errorf("RemainingQty = %v, want 6", p.RemainingQty)
	}
}

// The market maker places ~900k orders a week. Truncating the UUID to its first
// segment left 32 bits, and the orders table (retaining 2 days, ~259k rows) then
// took roughly 55 primary-key collisions a week — each one silently merged into
// an unrelated order by ON CONFLICT DO UPDATE rather than raising an error.
//
// 200k is well past the point where a 32-bit id fails this (expected collisions
// there are in the thousands) while staying fast.
func TestOrderIDsDoNotCollide(t *testing.T) {
	const n = 200_000
	seen := make(map[string]struct{}, n)
	for i := range n {
		id := generateOrderID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate order id %q after %d generations", id, i)
		}
		seen[id] = struct{}{}
	}
}

// 0.1+0.2 == 0.30000000000000004, so a maker filled in those two pieces used to
// fail `Filled < Quantity` in the mirror direction and stay on the book with a
// ~1e-17 remainder, holding its owner's lock on funds they could never spend.
func TestFullyFilledOrderLeavesNoDustLock(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 1)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "0.3", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}
	for _, qty := range []string{"0.1", "0.2"} {
		if _, _, _, err := e.CreateOrder(testMarket, "200", qty, "buy", "taker", "limit"); err != nil {
			t.Fatalf("buy %s: %v", qty, err)
		}
	}

	ob := e.Orderbooks[testMarket]
	if len(ob.Asks) != 0 {
		t.Errorf("a fully consumed order is still resting: %+v", ob.Asks)
	}
	if locked := bal(t, e, "maker", "SOL").Locked; locked != 0 {
		t.Errorf("maker still has %v SOL locked after the order was fully filled", locked)
	}
}

// releaseFunds must sweep a residual lock rather than skip it, which is what the
// old `if refund > 0` guard did whenever float noise made the surplus negative.
func TestReleaseFundsSweepsDustLock(t *testing.T) {
	b := &UserBalance{Available: 10, Locked: -1e-17}
	releaseFunds(b, 0)
	if b.Locked != 0 {
		t.Errorf("Locked = %v, want 0", b.Locked)
	}
	assertClose(t, "total is conserved", b.Available+b.Locked, 10-1e-17)
}

// NaN passes `<= 0` and `Available < needed` because every comparison against
// NaN is false. Reaching a balance would also break json.Marshal, which kills
// SaveSnapshot for the rest of the process's life.
func TestNonFiniteOrderRejectedBeforeLockingFunds(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "1", 1000, 10)

	for _, bad := range []string{"NaN", "Inf", "-Inf"} {
		t.Run(bad, func(t *testing.T) {
			_, _, _, err := e.CreateOrder(testMarket, bad, "1", "buy", "1", "limit")
			if err == nil {
				t.Fatalf("price %q was accepted", bad)
			}
			_, _, _, err = e.CreateOrder(testMarket, "200", bad, "buy", "1", "limit")
			if err == nil {
				t.Fatalf("quantity %q was accepted", bad)
			}
			b := bal(t, e, "1", "USD")
			if b.Available != 1000 || b.Locked != 0 {
				t.Fatalf("balance was touched: available=%v locked=%v", b.Available, b.Locked)
			}
		})
	}
}
