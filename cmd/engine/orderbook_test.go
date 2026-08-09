package main

import (
	"math"
	"testing"
)

const testMarket = "SOL_USD"

// newTestEngine builds an engine without the snapshot file or the background
// save goroutine that NewEngine wires up.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := &Engine{
		Orderbooks: make(map[string]*Orderbook),
		Balances:   make(map[string]map[string]*UserBalance),
		Users:      make(map[string]string),
	}
	ob := NewOrderbook("SOL", []Order{}, []Order{}, 0, 0)
	e.Orderbooks[ob.Ticker()] = ob
	return e
}

func fund(e *Engine, userID string, usd, sol float64) {
	e.Balances[userID] = map[string]*UserBalance{
		"USD": {Available: usd},
		"SOL": {Available: sol},
	}
}

func bal(t *testing.T, e *Engine, userID, asset string) *UserBalance {
	t.Helper()
	b, ok := e.Balances[userID][asset]
	if !ok {
		t.Fatalf("no %s balance for user %s", asset, userID)
	}
	return b
}

// assertClose compares floats with a tolerance, since balances accumulate
// through repeated addition and subtraction.
func assertClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// Fractional quantities used to truncate to zero: min() took ints, so buying
// 0.5 SOL executed nothing at all.
func TestFractionalPartialFill(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	executed, fills, _, err := e.CreateOrder(testMarket, "200", "0.5", "buy", "taker", "limit")
	if err != nil {
		t.Fatalf("buy: %v", err)
	}

	assertClose(t, "executedQty", executed, 0.5)
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want 1", len(fills))
	}
	assertClose(t, "fill qty", fills[0].Qty, 0.5)

	assertClose(t, "taker SOL available", bal(t, e, "taker", "SOL").Available, 0.5)
	assertClose(t, "taker USD available", bal(t, e, "taker", "USD").Available, 10000-100)
	assertClose(t, "taker USD locked", bal(t, e, "taker", "USD").Locked, 0)
	assertClose(t, "maker USD available", bal(t, e, "maker", "USD").Available, 100)
	// 1.5 sold - 0.5 filled = 1.0 still locked in the resting ask.
	assertClose(t, "maker SOL locked", bal(t, e, "maker", "SOL").Locked, 1.0)
}

// A taker crossing two resting price levels should fill at each maker's price,
// not at its own limit price.
func TestFillAcrossTwoLevels(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1", "sell", "maker", "limit"); err != nil {
		t.Fatalf("ask @200: %v", err)
	}
	if _, _, _, err := e.CreateOrder(testMarket, "201", "1", "sell", "maker", "limit"); err != nil {
		t.Fatalf("ask @201: %v", err)
	}

	executed, fills, _, err := e.CreateOrder(testMarket, "205", "2", "buy", "taker", "limit")
	if err != nil {
		t.Fatalf("buy: %v", err)
	}

	assertClose(t, "executedQty", executed, 2)
	if len(fills) != 2 {
		t.Fatalf("got %d fills, want 2", len(fills))
	}
	if fills[0].Price != "200" || fills[1].Price != "201" {
		t.Errorf("filled at %s and %s, want 200 then 201", fills[0].Price, fills[1].Price)
	}

	// Locked at the 205 limit, filled at 200 and 201: the 9.00 surplus must be
	// returned rather than staying locked forever.
	assertClose(t, "taker USD available", bal(t, e, "taker", "USD").Available, 10000-401)
	assertClose(t, "taker USD locked", bal(t, e, "taker", "USD").Locked, 0)
	assertClose(t, "taker SOL available", bal(t, e, "taker", "SOL").Available, 2)
}

func TestCancelRefundsExactLock(t *testing.T) {
	t.Run("buy refunds quote", func(t *testing.T) {
		e := newTestEngine(t)
		fund(e, "u", 10000, 0)

		_, _, orderID, err := e.CreateOrder(testMarket, "200", "2", "buy", "u", "limit")
		if err != nil {
			t.Fatalf("buy: %v", err)
		}
		assertClose(t, "locked after order", bal(t, e, "u", "USD").Locked, 400)

		e.handleCancelOrder(MessageFromAPI{
			Type: CANCEL_ORDER,
			Data: CancelOrderData{OrderID: orderID, Market: testMarket},
		}, "test-client")

		assertClose(t, "USD available", bal(t, e, "u", "USD").Available, 10000)
		assertClose(t, "USD locked", bal(t, e, "u", "USD").Locked, 0)
	})

	// Cancelling a sell used to credit USD instead of SOL: the handler looked up
	// the quote asset for an order that locked the base asset.
	t.Run("sell refunds base", func(t *testing.T) {
		e := newTestEngine(t)
		fund(e, "u", 0, 5)

		_, _, orderID, err := e.CreateOrder(testMarket, "200", "2", "sell", "u", "limit")
		if err != nil {
			t.Fatalf("sell: %v", err)
		}
		assertClose(t, "locked after order", bal(t, e, "u", "SOL").Locked, 2)

		e.handleCancelOrder(MessageFromAPI{
			Type: CANCEL_ORDER,
			Data: CancelOrderData{OrderID: orderID, Market: testMarket},
		}, "test-client")

		assertClose(t, "SOL available", bal(t, e, "u", "SOL").Available, 5)
		assertClose(t, "SOL locked", bal(t, e, "u", "SOL").Locked, 0)
		assertClose(t, "USD available", bal(t, e, "u", "USD").Available, 0)
	})
}

func TestInsufficientFundsRejectsCleanly(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "u", 100, 0)

	_, _, _, err := e.CreateOrder(testMarket, "200", "1", "buy", "u", "limit")
	if err == nil {
		t.Fatal("expected rejection, got nil error")
	}
	oe, ok := err.(*OrderError)
	if !ok || oe.Code != "INSUFFICIENT_FUNDS" {
		t.Fatalf("got %#v, want an INSUFFICIENT_FUNDS OrderError", err)
	}

	// Balances must be untouched, and nothing may rest on the book.
	assertClose(t, "USD available", bal(t, e, "u", "USD").Available, 100)
	assertClose(t, "USD locked", bal(t, e, "u", "USD").Locked, 0)
	if got := len(e.Orderbooks[testMarket].Bids); got != 0 {
		t.Errorf("%d bids rested after a rejected order, want 0", got)
	}
}

func TestMarketOrderDoesNotRest(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}

	// Ask for more than the book holds: the fillable part executes, the rest is
	// dropped rather than left resting, and its lock is returned.
	executed, _, _, err := e.CreateOrder(testMarket, "0", "3", "buy", "taker", "market")
	if err != nil {
		t.Fatalf("market buy: %v", err)
	}

	assertClose(t, "executedQty", executed, 1)
	if got := len(e.Orderbooks[testMarket].Bids); got != 0 {
		t.Errorf("market order left %d bids resting, want 0", got)
	}
	assertClose(t, "taker USD available", bal(t, e, "taker", "USD").Available, 10000-200)
	assertClose(t, "taker USD locked", bal(t, e, "taker", "USD").Locked, 0)
	assertClose(t, "taker SOL available", bal(t, e, "taker", "SOL").Available, 1)
}

func TestMarketOrderWithEmptyBookIsRejected(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "u", 10000, 0)

	_, _, _, err := e.CreateOrder(testMarket, "0", "1", "buy", "u", "market")
	oe, ok := err.(*OrderError)
	if !ok || oe.Code != "NO_LIQUIDITY" {
		t.Fatalf("got %#v, want a NO_LIQUIDITY OrderError", err)
	}
	assertClose(t, "USD available", bal(t, e, "u", "USD").Available, 10000)
}

func TestDepthAggregatesRemainingQuantity(t *testing.T) {
	e := newTestEngine(t)
	fund(e, "maker", 0, 10)
	fund(e, "taker", 10000, 0)

	if _, _, _, err := e.CreateOrder(testMarket, "200", "1.5", "sell", "maker", "limit"); err != nil {
		t.Fatalf("resting sell: %v", err)
	}
	if _, _, _, err := e.CreateOrder(testMarket, "200", "0.5", "buy", "taker", "limit"); err != nil {
		t.Fatalf("buy: %v", err)
	}

	depth := e.Orderbooks[testMarket].GetDepth()
	if len(depth.Asks) != 1 {
		t.Fatalf("got %d ask levels, want 1", len(depth.Asks))
	}
	// Full precision, not fixed 2dp: 2dp rounds a 0.0062 BTC size to "0.00".
	if depth.Asks[0][0] != "200" || depth.Asks[0][1] != "1" {
		t.Errorf("ask level = %v, want [200 1]", depth.Asks[0])
	}
}

// A sub-dollar market is the case fixed 2-decimal price formatting destroys:
// every level rounds onto the same string key and the ladder collapses.
func TestDepthKeepsDistinctLevelsOnSubDollarMarket(t *testing.T) {
	e := newTestEngine(t)
	ob := NewOrderbook("DOGE", []Order{}, []Order{}, 0, 0)
	e.Orderbooks[ob.Ticker()] = ob
	e.Balances["maker"] = map[string]*UserBalance{"USD": {Available: 1e6}, "DOGE": {Available: 1e6}}

	for _, price := range []string{"0.14963", "0.14925", "0.14888"} {
		if _, _, _, err := e.CreateOrder("DOGE_USD", price, "1000", "sell", "maker", "limit"); err != nil {
			t.Fatalf("resting sell at %s: %v", price, err)
		}
	}

	depth := e.Orderbooks["DOGE_USD"].GetDepth()
	if len(depth.Asks) != 3 {
		t.Fatalf("got %d ask levels, want 3 distinct: %v", len(depth.Asks), depth.Asks)
	}
}
