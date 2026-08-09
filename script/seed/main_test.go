package main

import (
	"strconv"
	"testing"

	"github.com/Althaf66/cryptoXchange/internal/markets"
)

// The seeder's whole job is producing a book that looks real at any price
// magnitude. DOGE at 0.15 is the case that breaks under absolute offsets and
// two-decimal formatting: every level rounds to the same price and zero size.
func TestOrdersForKeepsShapeAtEveryMagnitude(t *testing.T) {
	for _, m := range markets.All {
		orders := ordersFor(m)
		if len(orders) != 16 {
			t.Fatalf("%s: got %d orders, want 16", m.Ticker(), len(orders))
		}

		seen := map[string]bool{}
		for _, o := range orders {
			price, err := strconv.ParseFloat(o.Price, 64)
			if err != nil {
				t.Fatalf("%s: unparsable price %q: %v", m.Ticker(), o.Price, err)
			}
			qty, err := strconv.ParseFloat(o.Quantity, 64)
			if err != nil {
				t.Fatalf("%s: unparsable quantity %q: %v", m.Ticker(), o.Quantity, err)
			}
			if qty <= 0 {
				t.Errorf("%s: %s order has zero size (%q)", m.Ticker(), o.Side, o.Quantity)
			}
			// Everything sits within 2% of the mid.
			if price < m.Mid*0.98 || price > m.Mid*1.02 {
				t.Errorf("%s: price %v is not near mid %v", m.Ticker(), price, m.Mid)
			}
			if o.Market != m.Ticker() {
				t.Errorf("got market %q, want %q", o.Market, m.Ticker())
			}
			seen[o.Side+o.Price] = true
		}

		// The ten resting levels must be ten distinct prices, not one price
		// repeated because Decimals was too coarse for this mid.
		if len(seen) < 12 {
			t.Errorf("%s: only %d distinct side/price levels, book is degenerate", m.Ticker(), len(seen))
		}
	}
}
