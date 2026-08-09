package main

import (
	"math/rand"
	"testing"
)

// The mean reversion is the whole point: a plain random walk escapes any band
// you pick given enough steps, and a demo left running overnight would show BTC
// at 5 or at 500,000. 10k steps is roughly a week of ticks.
func TestPriceStaysNearItsAnchor(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const anchor = 65000.0
	price := anchor

	for i := 0; i < 10000; i++ {
		price = nextPrice(price, anchor, rng)
		if price <= 0 {
			t.Fatalf("price went non-positive (%v) after %d steps", price, i)
		}
		if price < anchor*0.5 || price > anchor*2 {
			t.Fatalf("price %v left the 0.5x-2x band after %d steps", price, i)
		}
	}
}

// A step must actually move the price, or the chart is a flat line.
func TestPriceMoves(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	const anchor = 200.0
	if got := nextPrice(anchor, anchor, rng); got == anchor {
		t.Error("nextPrice returned the price unchanged")
	}
}
