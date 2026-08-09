// Command seed fills the order book with resting depth and a handful of
// executed trades so a fresh demo opens on a real-looking market instead of an
// empty book.
//
// Every market in internal/markets is seeded unless -market narrows it.
//
// Usage: go run ./script/seed [-api http://localhost:8080/v1] [-market BTC_USD]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/markets"
)

type order struct {
	Market   string `json:"market"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Side     string `json:"side"`
	UserID   string `json:"userId"`
	Type     string `json:"type"`
}

// notionalPerLevel keeps each depth level worth roughly the same in USD, so a
// BTC level isn't 2 BTC while a DOGE level is 2 DOGE.
const notionalPerLevel = 400.0

func main() {
	api := flag.String("api", "http://localhost:8080/v1", "API base URL")
	only := flag.String("market", "", "seed only this market (default: all)")
	flag.Parse()

	var orders []order
	for _, m := range markets.All {
		if *only != "" && *only != m.Ticker() {
			continue
		}
		orders = append(orders, ordersFor(m)...)
	}

	if len(orders) == 0 {
		log.Fatalf("no markets matched %q", *only)
	}

	placed := 0
	for _, o := range orders {
		if err := post(*api+"/order", o); err != nil {
			log.Printf("%s order %s %s @ %s failed: %v", o.Market, o.Side, o.Quantity, o.Price, err)
			continue
		}
		placed++
		// The engine processes one message at a time; give it room to breathe.
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("seeded %d/%d orders", placed, len(orders))
}

// ordersFor builds resting depth plus a few crossing orders around a market's
// mid. Offsets are proportional, not absolute, so a 0.15 mid gets the same shape
// of book as a 65000 one.
func ordersFor(m markets.Market) []order {
	ticker := m.Ticker()
	price := func(v float64) string { return strconv.FormatFloat(v, 'f', m.Decimals, 64) }
	qty := func(level int) string {
		return strconv.FormatFloat(notionalPerLevel*float64(level)/m.Mid, 'f', 4, 64)
	}

	var orders []order

	// Five bids below and five asks above the mid, widening as they go out.
	for i := 1; i <= 5; i++ {
		offset := float64(i) * 0.0025
		orders = append(orders,
			order{ticker, price(m.Mid * (1 - offset)), qty(i), "buy", "1", "limit"},
			order{ticker, price(m.Mid * (1 + offset)), qty(i), "sell", "2", "limit"},
		)
	}

	// Crossing orders from a third account so there is trade history and the
	// candlestick chart has something to draw.
	crossQty := strconv.FormatFloat(notionalPerLevel/4/m.Mid, 'f', 4, 64)
	for i := 0; i < 6; i++ {
		side, p := "buy", m.Mid*1.005
		if i%2 == 1 {
			side, p = "sell", m.Mid*0.995
		}
		orders = append(orders, order{ticker, price(p), crossQty, side, "5", "limit"})
	}

	return orders
}

func post(url string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(msg))
	}
	return nil
}
