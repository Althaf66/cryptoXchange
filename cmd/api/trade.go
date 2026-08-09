package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/markets"
	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/gorilla/mux"
)

func (app *application) klinesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	interval := vars["interval"]

	market := r.URL.Query().Get("market")
	if market == "" {
		market = defaultMarket
	}

	klines, err := app.store.Trades.GetKlines(interval, market)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, klines)
}

func (app *application) latestPriceHandler(w http.ResponseWriter, r *http.Request) {
	price, err := app.store.Trades.GetLatestPrice()
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"price": price,
		"time":  time.Now().Format(time.RFC3339),
	}

	WriteJSON(w, http.StatusOK, response)
}

// tickersHandler returns one entry per demo market for the markets page.
// GetTicker returns a zeroed ticker rather than an error when a market has no
// trades yet, so an unseeded market still renders a row.
func (app *application) tickersHandler(w http.ResponseWriter, r *http.Request) {
	tickers := []*store.Ticker{}
	for _, symbol := range markets.Symbols() {
		ticker, err := app.store.Trades.GetTicker(symbol)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}
		tickers = append(tickers, ticker)
	}

	WriteJSON(w, http.StatusOK, tickers)
}

func (app *application) recentTradesHandler(w http.ResponseWriter, r *http.Request) {
	limit := 50 // default limit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	market := r.URL.Query().Get("market")

	trades, err := app.store.Trades.GetRecentTrades(limit, market)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"trades": trades,
		"count":  len(trades),
	})
}

func (app *application) marketTradesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	market := vars["market"]

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	trades, err := app.store.Trades.GetRecentTrades(limit, market)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"market": market,
		"trades": trades,
		"count":  len(trades),
	})
}
