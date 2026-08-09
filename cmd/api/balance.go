package main

import (
	"math"
	"net/http"
	"strconv"

	"github.com/Althaf66/cryptoXchange/internal/markets"
	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const ON_RAMP = "ON_RAMP"

// baseCurrency mirrors BASE_CURRENCY in the engine. Deposits are quote-asset
// only, so the transfers row records that asset.
const baseCurrency = "USD"

type OnRampRequest struct {
	UserId string `json:"userId"`
	Amount string `json:"amount"`
	// Asset to credit. Empty means the quote currency, which is what every
	// deposit was before the header could pick one.
	Asset string `json:"asset"`
	// TxnID is the idempotency key. Clients that send one are safe to retry;
	// when it is absent the server generates one so the deposit is still
	// recorded, but a double-submit will credit twice.
	TxnID string `json:"txnId"`
}

type GetBalanceData struct {
	UserID string `json:"userId"`
}

// isKnownAsset reports whether an asset is one a demo market actually trades:
// the quote currency, or the base of any pair.
func isKnownAsset(asset string) bool {
	if asset == baseCurrency {
		return true
	}
	for _, m := range markets.All {
		if m.Base == asset {
			return true
		}
	}
	return false
}

// balanceHandler asks the engine, which is the source of truth for balances.
// The Postgres balances table was never written to by anything.
func (app *application) balanceHandler(w http.ResponseWriter, r *http.Request) {
	userId := mux.Vars(r)["userId"]
	if userId == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: GET_BALANCE,
		Data: GetBalanceData{UserID: userId},
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}

// onRampHandler records the deposit before crediting it. The transfers row is
// written first, synchronously: if the engine is told to credit first and this
// process dies, the money exists in memory with nothing on disk explaining it.
func (app *application) onRampHandler(w http.ResponseWriter, r *http.Request) {
	var req OnRampRequest
	if err := ReadJSON(w, r, &req); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if req.UserId == "" || req.Amount == "" {
		http.Error(w, "userId and positive amount are required", http.StatusBadRequest)
		return
	}

	amount, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil || amount <= 0 {
		http.Error(w, "amount must be a positive number", http.StatusBadRequest)
		return
	}

	if req.Asset == "" {
		req.Asset = baseCurrency
	}
	// Without this an asset nobody trades gets credited and shows up forever as
	// a junk row in the balances panel.
	if !isKnownAsset(req.Asset) {
		http.Error(w, "unknown asset "+req.Asset, http.StatusBadRequest)
		return
	}

	if req.TxnID == "" {
		req.TxnID = uuid.NewString()
	}

	claimed, err := app.store.Transfers.Create(&store.Transfer{
		ID:        req.TxnID,
		UserID:    req.UserId,
		Asset:     req.Asset,
		Amount:    amount,
		Direction: "deposit",
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Key already used: a retry of a deposit we have seen. Return the current
	// balance without crediting again.
	if !claimed {
		app.writeCurrentBalance(w, r, req.UserId)
		return
	}

	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: ON_RAMP,
		Data: req,
	})
	if err != nil {
		// ponytail: a row left pending means the engine never confirmed. A
		// sweeper that retries or reverses them is the upgrade path; at demo
		// scale the reconcile endpoint is enough to notice.
		if markErr := app.store.Transfers.MarkStatus(req.TxnID, "failed"); markErr != nil {
			app.logger.Errorw("could not mark transfer failed", "txnId", req.TxnID, "error", markErr)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := app.store.Transfers.MarkStatus(req.TxnID, "confirmed"); err != nil {
		app.logger.Errorw("could not mark transfer confirmed", "txnId", req.TxnID, "error", err)
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}

// writeCurrentBalance answers a replayed deposit with the balance as it stands,
// so a retrying client sees the same shape of response as the original call.
func (app *application) writeCurrentBalance(w http.ResponseWriter, r *http.Request, userID string) {
	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: GET_BALANCE,
		Data: GetBalanceData{UserID: userID},
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, response.Payload)
}

func (app *application) transferHistoryHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	transfers, err := app.store.Transfers.ListByUser(userID, parseLimit(r, 50))
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, transfers)
}

// reconcileDrift is one (user, asset) where the ledger and the engine disagree.
type reconcileDrift struct {
	UserID      string  `json:"userId"`
	Asset       string  `json:"asset"`
	LedgerTotal float64 `json:"ledgerTotal"`
	EngineTotal float64 `json:"engineTotal"`
	Difference  float64 `json:"difference"`
}

// reconcileHandler compares SUM(ledger.delta) against the engine's
// Available+Locked for every user the ledger knows about. A ledger nothing ever
// checks is decoration; this is the check.
//
// The epsilon is not optional: the engine settles in float64, so exact equality
// fails on noise alone after a few dozen trades.
func (app *application) reconcileHandler(w http.ResponseWriter, r *http.Request) {
	const epsilon = 1e-6

	ledgerBalances, err := app.store.Ledger.Balances()
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// One engine round trip per user, not per (user, asset).
	engineTotals := map[string]map[string]float64{}
	drifts := []reconcileDrift{}

	for _, lb := range ledgerBalances {
		if _, fetched := engineTotals[lb.UserID]; !fetched {
			totals, err := app.engineBalanceTotals(r, lb.UserID)
			if err != nil {
				app.internalServerError(w, r, err)
				return
			}
			engineTotals[lb.UserID] = totals
		}

		engineTotal := engineTotals[lb.UserID][lb.Asset]
		diff := lb.Total - engineTotal
		if math.Abs(diff) > epsilon {
			drifts = append(drifts, reconcileDrift{
				UserID:      lb.UserID,
				Asset:       lb.Asset,
				LedgerTotal: lb.Total,
				EngineTotal: engineTotal,
				Difference:  diff,
			})
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"checked": len(ledgerBalances),
		"drifts":  drifts,
	})
}

// engineBalanceTotals asks the engine for a user's balances and folds each
// asset down to Available+Locked, which is what a ledger sum represents.
func (app *application) engineBalanceTotals(r *http.Request, userID string) (map[string]float64, error) {
	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: GET_BALANCE,
		Data: GetBalanceData{UserID: userID},
	})
	if err != nil {
		return nil, err
	}

	totals := map[string]float64{}
	payload, ok := response.Payload.(map[string]interface{})
	if !ok {
		return totals, nil
	}

	for asset, raw := range payload {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		available, _ := entry["available"].(float64)
		locked, _ := entry["locked"].(float64)
		totals[asset] = available + locked
	}
	return totals, nil
}

// parseLimit reads an optional ?limit=, falling back to fallback and capping at
// 500 so a stray query cannot ask for the whole table.
func parseLimit(r *http.Request, fallback int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 500 {
		return 500
	}
	return limit
}
