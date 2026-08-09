// Command marketmaker keeps the demo markets alive. Every tick it re-centres a
// ladder of resting depth and prints a handful of trades at a synthetic price.
//
// There is no external price feed: prices are a mean-reverting random walk
// anchored to the seed mids in internal/markets, so the demo works offline and
// BTC still looks like BTC.
//
// It trades only as its own accounts (botUsers in cmd/engine), never as a demo
// user or one created from the home page, so nobody's open orders or balances
// move underneath them.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/markets"
)

// notionalPerLevel keeps each depth level worth roughly the same in USD, so a
// BTC level isn't 2 BTC while a DOGE level is 2 DOGE. Same idea as script/seed.
const notionalPerLevel = 400.0

// subSteps is how many trades each tick prints. One trade per tick would give a
// candle whose open, high, low and close are identical - a flat dash rather than
// a candlestick.
const subSteps = 4

// The market maker's own accounts, funded by ensureMarkets in cmd/engine.
var bots = [2]string{"mm1", "mm2"}

type order struct {
	Market   string `json:"market"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Side     string `json:"side"`
	UserID   string `json:"userId"`
	Type     string `json:"type"`
}

type client struct {
	api  string
	http *http.Client
}

func main() {
	api := env("API_URL", "http://api:8080/v1")
	tick := tickInterval()

	c := &client{api: api, http: &http.Client{Timeout: 10 * time.Second}}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	prices := c.startingPrices()

	// A previous process's ladder is still on the book and its ids died with it.
	// Clear it before placing a new one.
	c.cancelResting()

	// Resting ladder ids from the previous tick, per market, so they can be
	// pulled before the next one goes up.
	resting := map[string][]string{}

	log.Printf("market maker started: api=%s tick=%s markets=%d", api, tick, len(markets.All))

	for tickNum := 0; ; tickNum++ {
		start := time.Now()
		c.runTick(prices, resting, rng, tick, tickNum)
		// Sleep the remainder, so a slow tick doesn't compound into drift.
		if wait := tick - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
	}
}

// runTick re-centres every market's depth, then walks the price through subSteps
// trades spread across the tick so the candle has a body and wicks.
func (c *client) runTick(prices map[string]float64, resting map[string][]string, rng *rand.Rand, tick time.Duration, tickNum int) {
	for _, m := range markets.All {
		ticker := m.Ticker()
		for _, id := range resting[ticker] {
			// A level consumed by a trade is already gone. That is normal.
			c.cancel(id, ticker)
		}
		resting[ticker] = c.placeLadder(m, prices[ticker], tickNum)
	}

	pause := tick / (subSteps + 1)
	for step := 0; step < subSteps; step++ {
		for _, m := range markets.All {
			ticker := m.Ticker()
			prices[ticker] = nextPrice(prices[ticker], m.Mid, rng)
			c.trade(m, prices[ticker], step)
		}
		time.Sleep(pause)
	}

	for _, m := range markets.All {
		log.Printf("%s at %s", m.Ticker(), formatPrice(m, prices[m.Ticker()]))
	}
}

// ladderSides picks which bot quotes each side of the book this tick.
//
// Whenever a sub-step crosses the ladder, the bot holding the bids pays quote
// currency and the bot holding the asks receives it — in both price directions,
// since a fall hits the bids and a rise lifts the asks. Alternating is what
// makes that flow reverse each tick instead of accumulating.
func ladderSides(tickNum int) (bidder, asker string) {
	return bots[tickNum%2], bots[(tickNum+1)%2]
}

// placeLadder rests five bids below and five asks above the price, widening as
// they go out, and returns the ids so the next tick can pull them.
//
// Which bot takes which side alternates by tick, for the same reason trade()
// alternates its taker roles. Whenever a sub-step crosses the ladder, the bot
// holding the bids pays quote and the bot holding the asks receives it —
// in both price directions, since a fall hits the bids and a rise lifts the
// asks. Pinning mm1 to the bid side therefore drained its USD (and mm2's base
// asset) monotonically, emptying both sides of the book within about a week.
// Alternating makes that flow reverse every tick instead of accumulating.
func (c *client) placeLadder(m markets.Market, price float64, tickNum int) []string {
	bidder, asker := ladderSides(tickNum)

	ids := []string{}
	for i := 1; i <= 5; i++ {
		offset := float64(i) * 0.0025
		bid := order{m.Ticker(), formatPrice(m, price*(1-offset)), quantity(price, i), "buy", bidder, "limit"}
		ask := order{m.Ticker(), formatPrice(m, price*(1+offset)), quantity(price, i), "sell", asker, "limit"}
		for _, o := range []order{bid, ask} {
			if id, err := c.place(o); err != nil {
				log.Printf("%s ladder %s @ %s failed: %v", o.Market, o.Side, o.Price, err)
			} else if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// trade prints one trade at price by resting a limit and immediately crossing it
// from the other bot. Which bot buys alternates: a fixed assignment would drain
// one account's USD and the other's base asset within a day.
func (c *client) trade(m markets.Market, price float64, step int) {
	seller, buyer := bots[step%2], bots[(step+1)%2]
	at := formatPrice(m, price)
	qty := quantity(price, 1)

	sellID, err := c.place(order{m.Ticker(), at, qty, "sell", seller, "limit"})
	if err != nil {
		log.Printf("%s sell @ %s failed: %v", m.Ticker(), at, err)
		return
	}
	if _, err := c.place(order{m.Ticker(), at, qty, "buy", buyer, "limit"}); err != nil {
		// The sell leg is resting and nothing else will ever cross it — the id
		// used to be discarded here, leaving one permanent orphan per failure.
		log.Printf("%s buy @ %s failed: %v; pulling the sell leg", m.Ticker(), at, err)
		if sellID != "" {
			c.cancel(sellID, m.Ticker())
		}
	}
}

// startingPrices resumes each market where it left off, so a restart doesn't
// snap the chart back to the hardcoded seed mid and print a cliff. Doubles as
// the wait-for-API probe: compose has no healthcheck on the api service.
func (c *client) startingPrices() map[string]float64 {
	prices := map[string]float64{}
	for _, m := range markets.All {
		prices[m.Ticker()] = m.Mid
	}

	var tickers []struct {
		Symbol    string `json:"symbol"`
		LastPrice string `json:"lastPrice"`
	}
	for attempt := 0; ; attempt++ {
		err := c.getJSON("/tickers", &tickers)
		if err == nil {
			break
		}
		if attempt%10 == 0 {
			log.Printf("waiting for the API: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	for _, t := range tickers {
		if last, err := strconv.ParseFloat(t.LastPrice, 64); err == nil && last > 0 {
			prices[t.Symbol] = last
		}
	}
	return prices
}

// cancelResting clears every order the bots still have on the book.
//
// `resting` lives only in this process's memory, so a restart used to abandon
// the whole ladder — 5 rungs x 2 sides x len(markets.All) orders that nothing
// would ever cancel, and which the engine's snapshot keeps across its own
// restarts too. They piled up on every restart and distorted the depth chart.
func (c *client) cancelResting() {
	for _, m := range markets.All {
		ticker := m.Ticker()
		for _, bot := range bots {
			var open []struct {
				OrderID string `json:"orderId"`
			}
			path := fmt.Sprintf("/order/open?userId=%s&market=%s", bot, ticker)
			if err := c.getJSON(path, &open); err != nil {
				log.Printf("%s: could not read %s open orders: %v", ticker, bot, err)
				continue
			}
			for _, o := range open {
				c.cancel(o.OrderID, ticker)
			}
			if len(open) > 0 {
				log.Printf("%s: cancelled %d stale %s order(s)", ticker, len(open), bot)
			}
		}
	}
}

func (c *client) place(o order) (string, error) {
	var placed struct {
		OrderID string `json:"orderId"`
	}
	if err := c.postJSON("/order", o, &placed); err != nil {
		return "", err
	}
	return placed.OrderID, nil
}

func (c *client) cancel(orderID, market string) {
	body, _ := json.Marshal(map[string]string{"orderId": orderID, "market": market})
	req, err := http.NewRequest(http.MethodDelete, c.api+"/order", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (c *client) getJSON(path string, out any) error {
	resp, err := c.http.Get(c.api + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *client) postJSON(path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.api+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(msg))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func formatPrice(m markets.Market, v float64) string {
	return strconv.FormatFloat(v, 'f', m.Decimals, 64)
}

func quantity(price float64, level int) string {
	return strconv.FormatFloat(notionalPerLevel*float64(level)/price, 'f', 4, 64)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func tickInterval() time.Duration {
	raw := env("TICK", "1m")
	tick, err := time.ParseDuration(raw)
	if err != nil || tick <= 0 {
		log.Printf("bad TICK %q, using 1m", raw)
		return time.Minute
	}
	return tick
}
