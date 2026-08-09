package kline

type DbMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type TradeData struct {
	ID           string `json:"id"`
	IsBuyerMaker bool   `json:"isBuyerMaker"`
	Price        string `json:"price"`
	Quantity     string `json:"quantity"`
	Timestamp    int64  `json:"timestamp"`
	Market       string `json:"market"`
}

// OrderUpdateData mirrors the engine's type in cmd/engine/model.go. ExecutedQty
// is a delta to add, not a total to set. Market/Price/Quantity/Side/UserID are
// present only on the create message; a nil UserID means this is an increment
// against a row that already exists.
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
