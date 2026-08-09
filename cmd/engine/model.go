package main

// import "time"

const (
	TRADE_ADDED  = "TRADE_ADDED"
	ORDER_UPDATE = "ORDER_UPDATE"
	LEDGER_ENTRY = "LEDGER_ENTRY"
)

// Ledger reasons. Locks and unlocks are deliberately absent: they move value
// between Available and Locked within one user, leaving Available+Locked
// unchanged, and the ledger only records changes to that total.
const (
	LEDGER_DEPOSIT = "deposit"
	LEDGER_TRADE   = "trade"
	LEDGER_SEED    = "seed"
)

const (
	CREATE_ORDER    = "CREATE_ORDER"
	CANCEL_ORDER    = "CANCEL_ORDER"
	ON_RAMP         = "ON_RAMP"
	GET_DEPTH       = "GET_DEPTH"
	GET_OPEN_ORDERS = "GET_OPEN_ORDERS"
	GET_BALANCE     = "GET_BALANCE"
	CREATE_USER     = "CREATE_USER"
	GET_USERS       = "GET_USERS"
)

const (
	DEPTH_UPDATE  = "DEPTH_UPDATE"
	TICKER_UPDATE = "TICKER_UPDATE"
)

type MessageFromAPI struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type CreateOrderData struct {
	Market   string `json:"market"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Side     string `json:"side"`
	UserID   string `json:"userId"`
	Type     string `json:"type"` // "limit" (default) or "market"
}

type CancelOrderData struct {
	OrderID string `json:"orderId"`
	Market  string `json:"market"`
}

type OnRampData struct {
	Amount string `json:"amount"`
	UserID string `json:"userId"`
	Asset  string `json:"asset"` // empty means the quote currency
	TxnID  string `json:"txnId"`
}

type CreateUserData struct {
	Name   string `json:"name"`
	Amount string `json:"amount"` // optional; empty means create with nothing
	TxnID  string `json:"txnId"`
}

// VirtualUser is a demo account: an engine user id and the name to show for it.
type VirtualUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type GetDepthData struct {
	Market string `json:"market"`
}

type GetOpenOrdersData struct {
	UserID string `json:"userId"`
	Market string `json:"market"`
}

type GetBalanceData struct {
	UserID string `json:"userId"`
}

type MessageToAPI struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type DepthPayload struct {
	Bids [][2]string `json:"bids"`
	Asks [][2]string `json:"asks"`
}

type OrderPlacedPayload struct {
	OrderID     string  `json:"orderId"`
	ExecutedQty float64 `json:"executedQty"`
	Fills       []Fill  `json:"fills"`
}

type OrderCancelledPayload struct {
	OrderID      string  `json:"orderId"`
	ExecutedQty  float64 `json:"executedQty"`
	RemainingQty float64 `json:"remainingQty"`
}

// OrderRejectedPayload carries a human-readable reason back to the API so it can
// map the failure onto a real HTTP status instead of a silent 201.
type OrderRejectedPayload struct {
	Reason string `json:"reason"`
	Code   string `json:"code"` // "INSUFFICIENT_FUNDS" | "NO_ORDERBOOK" | "INVALID_ORDER" | "NO_LIQUIDITY"
}

type WsMessage struct {
	Stream    string          `json:"stream"`
	Data      *DepthData      `json:"data"`
	TradeData *TradeAddedData `json:"tradeData,omitempty"`
}

type TickerUpdateMessage struct {
	Stream string                 `json:"stream"`
	Data   map[string]interface{} `json:"data"`
}

type DepthData struct {
	B  [][2]string `json:"b"`
	A  [][2]string `json:"a"`
	ID int         `json:"id,omitempty"`
	E  string      `json:"e"`
}

type TradeAddedMessage struct {
	Stream string `json:"stream"`
	Data   struct {
		E string `json:"e"`
		T int    `json:"t"`
		M bool   `json:"m"`
		P string `json:"p"`
		Q string `json:"q"`
		S string `json:"s"`
	} `json:"data"`
}

type DbMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type TradeAddedData struct {
	ID            string `json:"id"`
	E             string `json:"e"`
	IsBuyerMaker  bool   `json:"isBuyerMaker"`
	Price         string `json:"price"`
	Quantity      string `json:"quantity"`
	QuoteQuantity string `json:"quoteQuantity"`
	Timestamp     int64  `json:"timestamp"`
	Market        string `json:"market"`
}

// OrderUpdateData describes one change to an order row. ExecutedQty is a
// DELTA, not a running total: the create message carries whatever filled on
// entry and each maker fill carries its own quantity, so the consumer can add
// them without knowing which kind of message it is holding.
//
// The pointer fields are populated only on create. A maker-fill update leaves
// them nil, which is how the consumer tells the two apart.
type OrderUpdateData struct {
	OrderID     string  `json:"orderId"`
	ExecutedQty float64 `json:"executedQty"`
	Market      *string `json:"market,omitempty"`
	Price       *string `json:"price,omitempty"`
	Quantity    *string `json:"quantity,omitempty"`
	Side        *string `json:"side,omitempty"`
	UserID      *string `json:"userId,omitempty"`
	Status      *string `json:"status,omitempty"`
}

type LedgerEntryData struct {
	UserID string  `json:"userId"`
	Asset  string  `json:"asset"`
	Delta  float64 `json:"delta"`
	Reason string  `json:"reason"`
	RefID  string  `json:"refId"`
}

type OnRampPayload struct {
	UserID  string `json:"userId"`
	Asset   string `json:"asset"`
	Balance string `json:"balance"`
}
