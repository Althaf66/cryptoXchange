// Package markets is the single list of demo trading pairs. The engine, the API
// and the seed script are three separate binaries; without one shared list they
// drift apart and the demo boots with markets the seeder never fills.
package markets

// Market is a demo trading pair. Mid and Decimals are only read by the seed
// script, but they live here so a market is described in exactly one place.
type Market struct {
	Base     string
	Quote    string
	Mid      float64 // seed mid price
	Decimals int     // price decimals for seeding
}

// All markets are USD-quoted: the engine hardcodes BASE_CURRENCY = "USD" in its
// onramp and cancel-refund paths, so a non-USD quote would need those fixed too.
var All = []Market{
	{"SOL", "USD", 200, 2},
	{"BTC", "USD", 65000, 1},
	{"ETH", "USD", 3200, 2},
	{"DOGE", "USD", 0.15, 5},
	{"ADA", "USD", 0.45, 4},
}

func (m Market) Ticker() string { return m.Base + "_" + m.Quote }

func Symbols() []string {
	symbols := make([]string, len(All))
	for i, m := range All {
		symbols[i] = m.Ticker()
	}
	return symbols
}
