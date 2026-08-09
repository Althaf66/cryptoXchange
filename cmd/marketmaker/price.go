package main

import "math/rand"

const (
	// sigma is the per-step move, as a fraction of price. 0.15% keeps a tick's
	// four steps inside the ladder's inner rung most of the time.
	sigma = 0.0015
	// theta is how hard the price is pulled back toward its anchor each step.
	theta = 0.02
)

// nextPrice walks price one step: a small random move plus a pull back toward
// anchor. Without the pull a plain random walk wanders off to zero or the moon
// over a long-running demo; with it, BTC still reads as BTC tomorrow morning.
func nextPrice(price, anchor float64, rng *rand.Rand) float64 {
	price *= 1 + rng.NormFloat64()*sigma
	price += theta * (anchor - price)
	return price
}
