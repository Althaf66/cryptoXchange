package main

// import "time"

const (
	TRADE_ADDED  = "TRADE_ADDED"
	ORDER_UPDATE = "ORDER_UPDATE"
)

const (
	CREATE_ORDER    = "CREATE_ORDER"
	CANCEL_ORDER    = "CANCEL_ORDER"
	ON_RAMP         = "ON_RAMP"
	GET_DEPTH       = "GET_DEPTH"
	GET_OPEN_ORDERS = "GET_OPEN_ORDERS"
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
}

type CancelOrderData struct {
	OrderID string `json:"orderId"`
	Market  string `json:"market"`
}

type OnRampData struct {
	Amount string `json:"amount"`
	UserID string `json:"userId"`
	TxnID  string `json:"txnId"`
}

type GetDepthData struct {
	Market string `json:"market"`
}

type GetOpenOrdersData struct {
	UserID string `json:"userId"`
	Market string `json:"market"`
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
	OrderID     string `json:"orderId"`
	ExecutedQty int    `json:"executedQty"`
	Fills       []Fill `json:"fills"`
}

type OrderCancelledPayload struct {
	OrderID      string `json:"orderId"`
	ExecutedQty  int    `json:"executedQty"`
	RemainingQty int    `json:"remainingQty"`
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

type OrderUpdateData struct {
	OrderID     string  `json:"orderId"`
	ExecutedQty int     `json:"executedQty"`
	Market      *string `json:"market,omitempty"`
	Price       *string `json:"price,omitempty"`
	Quantity    *string `json:"quantity,omitempty"`
	Side        *string `json:"side,omitempty"`
}

type OnRampPayload struct {
	UserID  string `json:"userId"`
	Balance string `json:"balance"`
}
