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

type OrderUpdateData struct {
	OrderID     string  `json:"orderId"`
	ExecutedQty float64 `json:"executedQty"`
	Market      *string `json:"market,omitempty"`
	Price       *string `json:"price,omitempty"`
	Quantity    *string `json:"quantity,omitempty"`
	Side        *string `json:"side,omitempty"`
}
