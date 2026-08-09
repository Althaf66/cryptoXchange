package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/markets"
)

const BASE_CURRENCY = "USD"

// OrderError is a rejection the API can turn into a real HTTP status instead of
// a 500 or a silent 201.
type OrderError struct {
	Code   string
	Reason string
}

func (e *OrderError) Error() string { return e.Reason }

type UserBalance struct {
	Available float64 `json:"available"`
	Locked    float64 `json:"locked"`
}

type Engine struct {
	Orderbooks map[string]*Orderbook              `json:"orderbooks"`
	Balances   map[string]map[string]*UserBalance `json:"balances"`
	// Users maps an engine user id to its display name. Names are presentation
	// only - the engine has always keyed everything off the bare id.
	Users map[string]string `json:"users"`

	// ponytail: one lock for the whole engine. The message loop is single
	// threaded; this only guards it against the snapshot goroutine. Split per
	// orderbook if a second market ever needs real concurrency.
	mu sync.Mutex
}

func snapshotPath() string {
	if p := os.Getenv("SNAPSHOT_PATH"); p != "" {
		return p
	}
	return "./snapshot.json"
}

func NewEngine() *Engine {
	engine := &Engine{
		Orderbooks: make(map[string]*Orderbook),
		Balances:   make(map[string]map[string]*UserBalance),
		Users:      make(map[string]string),
	}

	if !engine.loadSnapshot() {
		log.Println("no snapshot found, starting from seeded state")
	}
	engine.ensureMarkets()

	// Start snapshot saving goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			engine.SaveSnapshot()
		}
	}()

	return engine
}

// loadSnapshot restores the book and balances written by SaveSnapshot. Without
// it every engine restart wipes the demo back to three hardcoded users, and the
// per-orderbook LastTradeID resets to 0 and collides with the trade rows already
// in Postgres.
func (e *Engine) loadSnapshot() bool {
	data, err := os.ReadFile(snapshotPath())
	if err != nil {
		return false
	}

	var snapshot struct {
		Orderbooks map[string]*Orderbook              `json:"orderbooks"`
		Balances   map[string]map[string]*UserBalance `json:"balances"`
		Users      map[string]string                  `json:"users"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		log.Printf("snapshot is corrupt, ignoring it: %v", err)
		return false
	}
	if len(snapshot.Orderbooks) == 0 {
		return false
	}

	e.Orderbooks = snapshot.Orderbooks
	if snapshot.Balances != nil {
		e.Balances = snapshot.Balances
	}
	if snapshot.Users != nil {
		e.Users = snapshot.Users
	}
	log.Printf("restored snapshot: %d orderbook(s), %d user balance(s)",
		len(e.Orderbooks), len(e.Balances))
	return true
}

func (e *Engine) SaveSnapshot() {
	e.mu.Lock()
	data, err := json.Marshal(struct {
		Orderbooks map[string]*Orderbook              `json:"orderbooks"`
		Balances   map[string]map[string]*UserBalance `json:"balances"`
		Users      map[string]string                  `json:"users"`
	}{e.Orderbooks, e.Balances, e.Users})
	e.mu.Unlock()

	if err != nil {
		log.Printf("Error marshaling snapshot: %v", err)
		return
	}
	// Write-then-rename, not WriteFile: WriteFile truncates first, so a crash
	// mid-write leaves valid-length garbage that loadSnapshot discards — which
	// silently re-seeds every balance while Postgres still holds the trades.
	// Rename is atomic, so the file is either the old snapshot or the new one.
	// This matters most on shutdown: main.go saves once more on SIGTERM, racing
	// the platform's grace period before SIGKILL.
	path := snapshotPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("Error writing snapshot: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("Error replacing snapshot: %v", err)
	}
}

func (e *Engine) Process(message MessageFromAPI, clientID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch message.Type {
	case CREATE_ORDER:
		e.handleCreateOrder(message, clientID)
	case CANCEL_ORDER:
		e.handleCancelOrder(message, clientID)
	case GET_OPEN_ORDERS:
		e.handleGetOpenOrders(message, clientID)
	case ON_RAMP:
		e.handleOnRamp(message, clientID)
	case GET_DEPTH:
		e.handleGetDepth(message, clientID)
	case GET_BALANCE:
		e.handleGetBalance(message, clientID)
	case CREATE_USER:
		e.handleCreateUser(message, clientID)
	case GET_USERS:
		e.handleGetUsers(clientID)
	default:
		// Every path through Process must answer. The API blocks on a pub/sub
		// reply, so a message we silently drop costs the caller its full
		// timeout rather than returning an error.
		sendRejection(clientID, &OrderError{
			Code:   "UNKNOWN_COMMAND",
			Reason: "unsupported message type: " + message.Type,
		})
	}
}

func (e *Engine) handleCreateOrder(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic creating order: %v", r)
			sendRejection(clientID, &OrderError{Code: "INTERNAL", Reason: fmt.Sprintf("%v", r)})
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data CreateOrderData
	json.Unmarshal(dataBytes, &data)

	executedQty, fills, orderID, err := e.CreateOrder(
		data.Market, data.Price, data.Quantity, data.Side, data.UserID, data.Type)
	if err != nil {
		log.Printf("Order rejected: %v", err)
		sendRejection(clientID, err)
		return
	}

	sendToAPI(clientID, MessageToAPI{
		Type: "ORDER_PLACED",
		Payload: OrderPlacedPayload{
			OrderID:     orderID,
			ExecutedQty: executedQty,
			Fills:       fills,
		},
	})
}

func sendRejection(clientID string, err error) {
	code := "INTERNAL"
	if oe, ok := err.(*OrderError); ok {
		code = oe.Code
	}
	sendToAPI(clientID, MessageToAPI{
		Type:    "ORDER_REJECTED",
		Payload: OrderRejectedPayload{Reason: err.Error(), Code: code},
	})
}

func (e *Engine) handleGetBalance(message MessageFromAPI, clientID string) {
	dataBytes, _ := json.Marshal(message.Data)
	var data GetBalanceData
	json.Unmarshal(dataBytes, &data)

	balances, exists := e.Balances[data.UserID]
	if !exists {
		balances = map[string]*UserBalance{}
	}

	sendToAPI(clientID, MessageToAPI{
		Type:    GET_BALANCE,
		Payload: balances,
	})
}

func (e *Engine) handleCancelOrder(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error cancelling order: %v", r)
			sendRejection(clientID, &OrderError{Code: "INTERNAL", Reason: fmt.Sprintf("%v", r)})
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data CancelOrderData
	json.Unmarshal(dataBytes, &data)

	orderbook, exists := e.Orderbooks[data.Market]
	if !exists {
		sendRejection(clientID, &OrderError{
			Code:   "NO_ORDERBOOK",
			Reason: "no orderbook for market " + data.Market,
		})
		return
	}

	baseAsset := strings.Split(data.Market, "_")[0]

	// Find order in asks or bids
	var order *Order
	for _, ask := range orderbook.Asks {
		if ask.OrderID == data.OrderID {
			order = &ask
			break
		}
	}
	if order == nil {
		for _, bid := range orderbook.Bids {
			if bid.OrderID == data.OrderID {
				order = &bid
				break
			}
		}
	}

	if order == nil {
		// Routine, not exceptional: an order that filled between the client
		// reading the book and sending the cancel is already gone. The market
		// maker does this every tick by design.
		sendRejection(clientID, &OrderError{
			Code:   "ORDER_NOT_FOUND",
			Reason: "no resting order " + data.OrderID + " on " + data.Market,
		})
		return
	}

	if order.Side == "buy" {
		price := orderbook.CancelBid(*order)
		leftQuantity := (order.Quantity - order.Filled) * order.Price

		// ponytail: refunds BASE_CURRENCY rather than the market's quote asset.
		// Correct while every market is USD-quoted; derive the quote from
		// data.Market if a non-USD pair is ever added.
		if userBalance, exists := e.Balances[order.UserID]; exists {
			if baseCurrency, exists := userBalance[BASE_CURRENCY]; exists {
				releaseFunds(baseCurrency, leftQuantity)
			}
		}

		if price != nil {
			e.publishWSDepthUpdates(nil, "", "", data.Market)
		}
	} else {
		price := orderbook.CancelAsk(*order)
		leftQuantity := order.Quantity - order.Filled

		// A sell locks the base asset, not the quote asset.
		if userBalance, exists := e.Balances[order.UserID]; exists {
			if baseBalance, exists := userBalance[baseAsset]; exists {
				releaseFunds(baseBalance, leftQuantity)
			}
		}

		if price != nil {
			e.publishWSDepthUpdates(nil, "", "", data.Market)
		}
	}

	e.markOrderCancelled(data.OrderID)

	sendToAPI(clientID, MessageToAPI{
		Type: "ORDER_CANCELLED",
		Payload: OrderCancelledPayload{
			OrderID:      data.OrderID,
			ExecutedQty:  order.Filled,
			RemainingQty: order.Quantity - order.Filled,
		},
	})
}

func (e *Engine) handleGetOpenOrders(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error getting open orders: %v", r)
			sendRejection(clientID, &OrderError{Code: "INTERNAL", Reason: fmt.Sprintf("%v", r)})
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data GetOpenOrdersData
	json.Unmarshal(dataBytes, &data)

	orderbook, exists := e.Orderbooks[data.Market]
	if !exists {
		sendRejection(clientID, &OrderError{
			Code:   "NO_ORDERBOOK",
			Reason: "no orderbook for market " + data.Market,
		})
		return
	}

	openOrders := orderbook.GetOpenOrders(data.UserID)

	sendToAPI(clientID, MessageToAPI{
		Type:    "OPEN_ORDERS",
		Payload: openOrders,
	})
}

func (e *Engine) handleOnRamp(message MessageFromAPI, clientID string) {
	dataBytes, _ := json.Marshal(message.Data)
	var data OnRampData
	json.Unmarshal(dataBytes, &data)

	amount, err := strconv.ParseFloat(data.Amount, 64)
	// ParseFloat accepts "NaN" and "Inf" with a nil error, and every guard
	// downstream is a comparison — all of which are false for NaN. A NaN that
	// reaches a balance also makes json.Marshal fail, which kills SaveSnapshot
	// permanently, so it has to be rejected at the parse.
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		sendRejection(clientID, &OrderError{
			Code:   "INVALID_AMOUNT",
			Reason: "amount must be a positive, finite number: " + data.Amount,
		})
		return
	}
	userid, asset, balance := e.onRamp(data.UserID, data.Asset, amount, data.TxnID)
	sendToAPI(clientID, MessageToAPI{
		Type: "ON_RAMP",
		Payload: OnRampPayload{
			UserID:  userid,
			Asset:   asset,
			Balance: fmt.Sprintf("%.2f", balance),
		},
	})
}

func (e *Engine) handleGetDepth(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error getting depth: %v", r)
			sendToAPI(clientID, MessageToAPI{
				Type: GET_DEPTH,
				Payload: DepthPayload{
					Bids: [][2]string{},
					Asks: [][2]string{},
				},
			})
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data GetDepthData
	json.Unmarshal(dataBytes, &data)
	log.Printf("GetDepthData: %+v", data)

	orderbook, exists := e.Orderbooks[data.Market]
	if !exists {
		panic("No orderbook found")
	}

	sendToAPI(clientID, MessageToAPI{
		Type:    GET_DEPTH,
		Payload: orderbook.GetDepth(),
	})
	log.Printf("Depth for market %s sent to client %s", data.Market, clientID)
}

// isFinitePositive rejects NaN and ±Inf as well as zero and negatives.
// strconv.ParseFloat parses "NaN"/"Inf" without an error, so a plain `<= 0`
// check lets them straight through.
func isFinitePositive(f float64) bool {
	return f > 0 && !math.IsInf(f, 0)
}

// marketOrderSlippage pads a market order's limit price so it sweeps the book
// instead of resting. 5% is plenty for a demo book quoted around 200.
const marketOrderSlippage = 0.05

func (e *Engine) CreateOrder(market, priceStr, quantityStr, side, userID, orderType string) (float64, []Fill, string, error) {
	orderbook, exists := e.Orderbooks[market]
	if !exists {
		return 0, nil, "", &OrderError{Code: "NO_ORDERBOOK", Reason: "no orderbook for market " + market}
	}

	baseAsset := strings.Split(market, "_")[0]
	quoteAsset := strings.Split(market, "_")[1]

	price, err := strconv.ParseFloat(priceStr, 64)
	if orderType == "market" {
		// A market order is a limit order priced through the far side of the
		// book. Whatever price the client sent is ignored.
		bestBid, bestAsk := orderbook.GetBestBidAsk()
		var ref *[2]string
		if side == "buy" {
			ref = bestAsk
		} else {
			ref = bestBid
		}
		if ref == nil {
			return 0, nil, "", &OrderError{Code: "NO_LIQUIDITY", Reason: "no resting orders to fill a market order against"}
		}
		refPrice, _ := strconv.ParseFloat((*ref)[0], 64)
		if side == "buy" {
			price = refPrice * (1 + marketOrderSlippage)
		} else {
			price = refPrice * (1 - marketOrderSlippage)
		}
	} else if err != nil {
		return 0, nil, "", &OrderError{Code: "INVALID_ORDER", Reason: "price is not a number: " + priceStr}
	}

	quantity, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil {
		return 0, nil, "", &OrderError{Code: "INVALID_ORDER", Reason: "quantity is not a number: " + quantityStr}
	}

	// Has to happen before CheckAndLockFunds, not in validateOrder: funds are
	// locked first, and a NaN slips past `Available < needed` (every comparison
	// against NaN is false) to poison the balance beyond repair — releaseLock
	// cannot undo it, and the resulting snapshot never marshals again.
	if !isFinitePositive(price) || !isFinitePositive(quantity) {
		return 0, nil, "", &OrderError{
			Code:   "INVALID_ORDER",
			Reason: "price and quantity must be positive, finite numbers",
		}
	}

	if err := e.CheckAndLockFunds(baseAsset, quoteAsset, side, userID, price, quantity); err != nil {
		return 0, nil, "", err
	}

	order := Order{
		Price:    price,
		Quantity: quantity,
		OrderID:  generateOrderID(),
		Filled:   0,
		Side:     side,
		UserID:   userID,
	}

	restRemainder := orderType != "market"
	executedQty, fills, err := orderbook.AddOrder(order, restRemainder)
	if err != nil {
		// Validation failed after we locked funds - give them straight back.
		e.releaseLock(userID, baseAsset, quoteAsset, side, quantity, price)
		return 0, nil, "", &OrderError{Code: "INVALID_ORDER", Reason: err.Error()}
	}

	e.UpdateBalance(userID, baseAsset, quoteAsset, side, market, fills, executedQty)
	e.releaseOverLock(userID, baseAsset, quoteAsset, side, fills, executedQty, quantity, price, restRemainder)

	e.CreateDbTrades(fills, market, userID)
	e.UpdateDbOrders(order, executedQty, fills, market, restRemainder)
	e.publishWSDepthUpdates(fills, priceStr, side, market)
	e.publishWSTrades(fills, userID, market)

	return executedQty, fills, order.OrderID, nil
}

func (e *Engine) CheckAndLockFunds(baseAsset, quoteAsset, side, userID string, price, quantity float64) error {
	if _, exists := e.Balances[userID]; !exists {
		e.Balances[userID] = make(map[string]*UserBalance)
	}

	asset, needed := baseAsset, quantity
	if side == "buy" {
		asset, needed = quoteAsset, quantity*price
	}

	if _, exists := e.Balances[userID][asset]; !exists {
		e.Balances[userID][asset] = &UserBalance{Available: 0, Locked: 0}
	}

	bal := e.Balances[userID][asset]
	if bal.Available < needed {
		return &OrderError{
			Code: "INSUFFICIENT_FUNDS",
			Reason: fmt.Sprintf("insufficient %s: need %.4f, have %.4f",
				asset, needed, bal.Available),
		}
	}

	bal.Available -= needed
	bal.Locked += needed
	return nil
}

// releaseLock returns a full lock to available, used when an order is rejected
// after funds were already locked.
func (e *Engine) releaseLock(userID, baseAsset, quoteAsset, side string, quantity, price float64) {
	asset, amount := baseAsset, quantity
	if side == "buy" {
		asset, amount = quoteAsset, quantity*price
	}
	if bal, ok := e.Balances[userID][asset]; ok {
		releaseFunds(bal, amount)
	}
}

// releaseFunds moves amount from Locked back to Available.
//
// It also sweeps dust: a lock that has been fully released can still read as
// ±1e-17 after a chain of float64 adds and subtracts, and the guard used to be
// a plain `refund > 0`, so a noise-negative surplus was skipped entirely and
// stayed Locked for good — funds the user could see but never spend. Only ever
// moves value between the two fields, never changes their sum, which is why it
// correctly emits no ledger row.
func releaseFunds(bal *UserBalance, amount float64) {
	if amount > 0 {
		bal.Available += amount
		bal.Locked -= amount
	}
	if math.Abs(bal.Locked) < dustEpsilon {
		bal.Available += bal.Locked
		bal.Locked = 0
	}
}

// releaseOverLock refunds the difference between what was locked at the order
// price and what the fills actually cost. Buys lock at the limit price but can
// fill cheaper, and market orders lock at a padded price; without this the
// surplus stays Locked forever. It also frees the remainder of a market order,
// which never rests on the book.
func (e *Engine) releaseOverLock(userID, baseAsset, quoteAsset, side string, fills []Fill, executedQty, quantity, price float64, restRemainder bool) {
	unrested := 0.0
	if !restRemainder {
		unrested = quantity - executedQty
	}

	if side == "buy" {
		spent := 0.0
		for _, fill := range fills {
			fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
			spent += fillPrice * fill.Qty
		}
		refund := executedQty*price - spent + unrested*price
		if bal, ok := e.Balances[userID][quoteAsset]; ok {
			releaseFunds(bal, refund)
		}
		return
	}

	// Sells lock base quantity 1:1, so only an unrested remainder needs freeing.
	if bal, ok := e.Balances[userID][baseAsset]; ok {
		releaseFunds(bal, unrested)
	}
}

// pushLedger records a change to a user's total holding of an asset
// (Available+Locked). Anything that only shuffles value between Available and
// Locked - locking, unlocking, refunds - must NOT call this: the total did not
// move, and a row here would break the reconcile invariant.
func (e *Engine) pushLedger(userID, asset string, delta float64, reason, refID string) {
	if delta == 0 {
		return
	}
	err := pushDbMessage(DbMessage{
		Type: LEDGER_ENTRY,
		Data: LedgerEntryData{
			UserID: userID,
			Asset:  asset,
			Delta:  delta,
			Reason: reason,
			RefID:  refID,
		},
	})
	if err != nil {
		log.Printf("Error pushing ledger entry for %s/%s: %v", userID, asset, err)
	}
}

// tradeRefID matches the id CreateDbTrades writes into sol_prices, so a ledger
// row can be traced back to the trade that caused it.
func tradeRefID(market string, tradeID int) string {
	return market + "-" + strconv.Itoa(tradeID)
}

func (e *Engine) UpdateBalance(userID, baseAsset, quoteAsset, side, market string, fills []Fill, executedQty float64) {
	if side == "buy" {
		for _, fill := range fills {
			fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
			fillQty := fill.Qty

			// Update other user's quote asset
			if _, exists := e.Balances[fill.OtherUserID]; !exists {
				e.Balances[fill.OtherUserID] = make(map[string]*UserBalance)
			}
			if _, exists := e.Balances[fill.OtherUserID][quoteAsset]; !exists {
				e.Balances[fill.OtherUserID][quoteAsset] = &UserBalance{Available: 0, Locked: 0}
			}

			e.Balances[fill.OtherUserID][quoteAsset].Available += fillQty * fillPrice
			e.Balances[userID][quoteAsset].Locked -= fillQty * fillPrice

			// Update base asset
			if _, exists := e.Balances[fill.OtherUserID][baseAsset]; !exists {
				e.Balances[fill.OtherUserID][baseAsset] = &UserBalance{Available: 0, Locked: 0}
			}
			if _, exists := e.Balances[userID][baseAsset]; !exists {
				e.Balances[userID][baseAsset] = &UserBalance{Available: 0, Locked: 0}
			}

			e.Balances[fill.OtherUserID][baseAsset].Locked -= fillQty
			e.Balances[userID][baseAsset].Available += fillQty

			// Four legs, netting to zero per asset: a trade moves value between
			// two users, it never creates any.
			ref := tradeRefID(market, fill.TradeID)
			e.pushLedger(fill.OtherUserID, quoteAsset, fillQty*fillPrice, LEDGER_TRADE, ref)
			e.pushLedger(userID, quoteAsset, -fillQty*fillPrice, LEDGER_TRADE, ref)
			e.pushLedger(fill.OtherUserID, baseAsset, -fillQty, LEDGER_TRADE, ref)
			e.pushLedger(userID, baseAsset, fillQty, LEDGER_TRADE, ref)
		}
	} else {
		for _, fill := range fills {
			fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
			fillQty := fill.Qty

			// Update quote asset
			if _, exists := e.Balances[fill.OtherUserID]; !exists {
				e.Balances[fill.OtherUserID] = make(map[string]*UserBalance)
			}
			if _, exists := e.Balances[fill.OtherUserID][quoteAsset]; !exists {
				e.Balances[fill.OtherUserID][quoteAsset] = &UserBalance{Available: 0, Locked: 0}
			}
			if _, exists := e.Balances[userID][quoteAsset]; !exists {
				e.Balances[userID][quoteAsset] = &UserBalance{Available: 0, Locked: 0}
			}

			e.Balances[fill.OtherUserID][quoteAsset].Locked -= fillQty * fillPrice
			e.Balances[userID][quoteAsset].Available += fillQty * fillPrice

			// Update base asset
			if _, exists := e.Balances[fill.OtherUserID][baseAsset]; !exists {
				e.Balances[fill.OtherUserID][baseAsset] = &UserBalance{Available: 0, Locked: 0}
			}

			e.Balances[fill.OtherUserID][baseAsset].Available += fillQty
			e.Balances[userID][baseAsset].Locked -= fillQty

			// Mirror image of the buy branch.
			ref := tradeRefID(market, fill.TradeID)
			e.pushLedger(fill.OtherUserID, quoteAsset, -fillQty*fillPrice, LEDGER_TRADE, ref)
			e.pushLedger(userID, quoteAsset, fillQty*fillPrice, LEDGER_TRADE, ref)
			e.pushLedger(fill.OtherUserID, baseAsset, fillQty, LEDGER_TRADE, ref)
			e.pushLedger(userID, baseAsset, -fillQty, LEDGER_TRADE, ref)
		}
	}
}

// formatNum renders a price or quantity for the UI and the trade log. Fixed 2
// decimals cannot serve every market at once: it rounds a 0.0062 BTC size to
// "0.00" and collapses five distinct DOGE levels around 0.149 onto one string
// key. Eight decimals covers every market here; trimming the zeros keeps whole
// numbers short and hides the float noise that summing quantities accumulates
// (0.0016999999999999993 -> 0.0017).
func formatNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', 8, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func (e *Engine) CreateDbTrades(fills []Fill, market, userID string) {
	for _, fill := range fills {
		fillPrice, _ := strconv.ParseFloat(fill.Price, 64)

		pushDbMessage(DbMessage{
			Type: TRADE_ADDED,
			Data: TradeAddedData{
				Market: market,
				// LastTradeID is per-orderbook and restarts at 0 for each new
				// market, so the raw id collides on the (id, time) primary key.
				ID:            market + "-" + strconv.Itoa(fill.TradeID),
				IsBuyerMaker:  fill.OtherUserID == userID,
				Price:         fill.Price,
				Quantity:      formatNum(fill.Qty),
				QuoteQuantity: fmt.Sprintf("%.2f", fill.Qty*fillPrice),
				// Milliseconds: the kline processor divides this by 1000.
				Timestamp: time.Now().UnixMilli(),
			},
		})
	}
}

// UpdateDbOrders records the taker's new order plus the fills it consumed.
// restRemainder is false for market orders, whose unfilled remainder never
// rests on the book - without it a partially filled market order would sit in
// the database as open forever.
func (e *Engine) UpdateDbOrders(order Order, executedQty float64, fills []Fill, market string, restRemainder bool) {
	side := order.Side
	userID := order.UserID
	price := formatNum(order.Price)
	quantity := formatNum(order.Quantity)

	// restRemainder is false for every market order, so testing it alone marked
	// a market order "filled" no matter how little executed — a sweep of 1 out
	// of 3 was indistinguishable from a complete fill in the order history.
	//
	// A market order cannot execute nothing: the slippage pad is applied to the
	// best price, so the top level is always crossable, and an empty book is
	// rejected as NO_LIQUIDITY before any row is written.
	status := "open"
	switch {
	case executedQty >= order.Quantity:
		status = "filled"
	case !restRemainder:
		// The remainder is gone rather than open — a market order never rests.
		status = "partially_filled"
	}

	// The taker's own row: everything needed to INSERT it, with the quantity it
	// filled on entry as the first delta.
	pushDbMessage(DbMessage{
		Type: ORDER_UPDATE,
		Data: OrderUpdateData{
			OrderID:     order.OrderID,
			ExecutedQty: executedQty,
			Market:      &market,
			Price:       &price,
			Quantity:    &quantity,
			Side:        &side,
			UserID:      &userID,
			Status:      &status,
		},
	})

	// Maker rows already exist from their own create, so these carry the delta
	// alone. The nil identifying fields are what mark them as increments.
	for _, fill := range fills {
		pushDbMessage(DbMessage{
			Type: ORDER_UPDATE,
			Data: OrderUpdateData{
				OrderID:     fill.MarkerOrderID,
				ExecutedQty: fill.Qty,
			},
		})
	}
}

// markOrderCancelled records a cancellation. Without it a cancelled order sits
// in the database as permanently open.
func (e *Engine) markOrderCancelled(orderID string) {
	status := "cancelled"
	pushDbMessage(DbMessage{
		Type: ORDER_UPDATE,
		Data: OrderUpdateData{
			OrderID:     orderID,
			ExecutedQty: 0,
			Status:      &status,
		},
	})
}

func (e *Engine) publishWSTrades(fills []Fill, userID, market string) {
	for _, fill := range fills {
		GetRedisInstance().PublishMessage(fmt.Sprintf("trade@%s", market), WsMessage{
			Stream: fmt.Sprintf("trade@%s", market),
			TradeData: &TradeAddedData{
				E:            "trade",
				Market:       market,
				ID:           strconv.Itoa(fill.TradeID),
				IsBuyerMaker: fill.OtherUserID == userID,
				Price:        fill.Price,
				Quantity:     formatNum(fill.Qty),
				Timestamp:    time.Now().UnixMilli(),
			},
		})
	}
}

func (e *Engine) publishWSDepthUpdates(fills []Fill, price, side, market string) {
	orderbook, exists := e.Orderbooks[market]
	if !exists {
		log.Printf("Orderbook for market %s not found", market)
		return
	}

	depth := orderbook.GetDepth()
	log.Printf("Depth for market %s: %+v", market, depth)

	// Initialize updated bids and asks with the full order book
	updatedBids := make([][2]string, len(depth.Bids))
	copy(updatedBids, depth.Bids)
	updatedAsks := make([][2]string, len(depth.Asks))
	copy(updatedAsks, depth.Asks)

	message := WsMessage{
		Stream: fmt.Sprintf("depth@%s", market),
		Data: &DepthData{
			B: updatedBids,
			A: updatedAsks,
			E: "depth",
		},
	}
	log.Printf("Publishing WsMessage to %s: %+v", message.Stream, message)

	GetRedisInstance().PublishMessage(message.Stream, message)
}

// onRamp credits a deposit of one asset. txnID is the transfers row the API
// already wrote before sending this message; it becomes the ledger ref so a
// credit can be traced back to the deposit that caused it.
//
// An empty asset means the quote currency, which is what every deposit was
// before the header's Add fund panel could pick one.
func (e *Engine) onRamp(userID, asset string, amount float64, txnID string) (string, string, float64) {
	if asset == "" {
		asset = BASE_CURRENCY
	}
	if e.Balances[userID] == nil {
		e.Balances[userID] = make(map[string]*UserBalance)
	}
	if e.Balances[userID][asset] == nil {
		e.Balances[userID][asset] = &UserBalance{}
	}

	e.Balances[userID][asset].Available += amount
	e.pushLedger(userID, asset, amount, LEDGER_DEPOSIT, txnID)
	log.Printf("OnRamp: user %s deposited %.2f %s, now holds %.2f",
		userID, amount, asset, e.Balances[userID][asset].Available)

	return userID, asset, e.Balances[userID][asset].Available
}

// demoUsers are the accounts the seed script trades from.
var demoUsers = []string{"1", "2", "5"}

// botUsers are the market maker's own accounts. They are funded like the demo
// users but deliberately left out of e.Users: the frontend's account picker is
// built from that map, and nobody should be able to trade *as* the bot - its
// ladder churns every minute and would bury a human's own open orders.
//
// The ids are non-numeric on purpose. nextUserID only considers ids that parse
// as integers, so these can never collide with an account made on the home page.
var botUsers = []string{"mm1", "mm2"}

// ensureMarkets adds any orderbook in markets.All that the engine doesn't have
// yet, plus demo balances for its base asset. It runs on every boot, snapshot or
// not: a snapshot written before a market was added would otherwise pin the
// engine to the old market list forever.
func (e *Engine) ensureMarkets() {
	for _, m := range markets.All {
		if _, exists := e.Orderbooks[m.Ticker()]; !exists {
			book := NewOrderbook(m.Base, []Order{}, []Order{}, 0, 0)
			e.Orderbooks[book.Ticker()] = book
			log.Printf("created orderbook %s", book.Ticker())
		}
	}

	for _, user := range demoUsers {
		if e.Users[user] == "" {
			e.Users[user] = "Demo user " + user
		}
		e.seedBalances(user)
	}

	// Funded the same way, but never named - see botUsers.
	for _, user := range botUsers {
		e.seedBalances(user)
	}
}

// seedBalances gives a demo account 10M of every asset, once. Existing balances
// are left alone so a restart doesn't hand traders a fresh 10M.
func (e *Engine) seedBalances(user string) {
	if e.Balances[user] == nil {
		e.Balances[user] = make(map[string]*UserBalance)
	}
	// Credit quote first, then every base asset.
	for _, asset := range allAssets() {
		if _, exists := e.Balances[user][asset]; !exists {
			e.Balances[user][asset] = &UserBalance{Available: 10000000, Locked: 0}
			// Seed credits get ledger rows too, otherwise every reconcile run
			// reports the demo users as drifting by 10M. The ref is
			// deterministic so deleting snapshot.json against a surviving
			// database re-seeds memory without double-counting the ledger.
			e.pushLedger(user, asset, 10000000, LEDGER_SEED, user+":"+asset)
		}
	}
}

// allAssets is every asset a demo account can hold: the quote currency plus the
// base of each market. "Fund all markets" means this list.
func allAssets() []string {
	return append([]string{BASE_CURRENCY}, baseAssets()...)
}

func baseAssets() []string {
	assets := make([]string, len(markets.All))
	for i, m := range markets.All {
		assets[i] = m.Base
	}
	return assets
}
