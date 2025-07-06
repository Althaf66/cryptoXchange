package ws

// Message types
const (
	SUBSCRIBE   = "SUBSCRIBE"
	UNSUBSCRIBE = "UNSUBSCRIBE"
)

// IncomingMessage types
type SubscribeMessage struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type UnsubscribeMessage struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type IncomingMessage struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

// OutgoingMessage types
type TickerData struct {
	C  *string `json:"c,omitempty"`
	H  *string `json:"h,omitempty"`
	L  *string `json:"l,omitempty"`
	V  *string `json:"v,omitempty"`
	VV *string `json:"V,omitempty"`
	S  *string `json:"s,omitempty"`
	ID int     `json:"id"`
	E  string  `json:"e"`
}

type TickerUpdateMessage struct {
	Type string     `json:"type"`
	Data TickerData `json:"data"`
}

type DepthData struct {
	B  [][2]string `json:"b"`
	A  [][2]string `json:"a"`
	ID int         `json:"id"`
	E  string      `json:"e"`
}

type DepthUpdateMessage struct {
	Type string    `json:"type"`
	Data DepthData `json:"data"`
}

type TradeData struct {
	TradeID         string `json:"tradeId"`
	OrderID         string `json:"orderId"`
	UserID          string `json:"userId"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"` // "buy" or "sell"
	Quantity        string `json:"quantity"`
	Price           string `json:"price"`
	FilledQty       string `json:"filledQty"`
	RemainingQty    string `json:"remainingQty"`
	Status          string `json:"status"` // "filled", "partial", "cancelled"
	Timestamp       int64  `json:"timestamp"`
	Commission      string `json:"commission,omitempty"`
	CommissionAsset string `json:"commissionAsset,omitempty"`
	E               string `json:"e"`
}

type TradeUpdateMessage struct {
	Type string    `json:"type"`
	Data TradeData `json:"data"`
}

type OutgoingMessage struct {
	Stream     string      `json:"stream"`
	DepthData  *DepthData  `json:"data,omitempty"`
	TickerData *TickerData `json:"tickerdata,omitempty"`
	TradeData  *TradeData  `json:"tradedata,omitempty"`
}
