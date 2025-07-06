package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
)

type Order struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	OrderID  string  `json:"orderId"`
	Filled   float64 `json:"filled"`
	Side     string  `json:"side"`
	UserID   string  `json:"userId"`
}

type Fill struct {
	Price         string `json:"price"`
	Qty           int    `json:"qty"`
	TradeID       int    `json:"tradeId"`
	OtherUserID   string `json:"otherUserId"`
	MarkerOrderID string `json:"markerOrderId"`
}

type Orderbook struct {
	Bids         []Order `json:"bids"`
	Asks         []Order `json:"asks"`
	BaseAsset    string  `json:"baseAsset"`
	QuoteAsset   string  `json:"quoteAsset"`
	LastTradeID  int     `json:"lastTradeId"`
	CurrentPrice float64 `json:"currentPrice"`
}

func NewOrderbook(baseAsset string, bids []Order, asks []Order, lastTradeID int, currentPrice float64) *Orderbook {
	return &Orderbook{
		Bids:         bids,
		Asks:         asks,
		BaseAsset:    baseAsset,
		QuoteAsset:   "USD",
		LastTradeID:  lastTradeID,
		CurrentPrice: currentPrice,
	}
}

func (o *Orderbook) Ticker() string {
	return fmt.Sprintf("%s_%s", o.BaseAsset, o.QuoteAsset)
}

func (o *Orderbook) GetSnapshot() map[string]interface{} {
	return map[string]interface{}{
		"baseAsset":    o.BaseAsset,
		"bids":         o.Bids,
		"asks":         o.Asks,
		"lastTradeId":  o.LastTradeID,
		"currentPrice": o.CurrentPrice,
	}
}

func (o *Orderbook) AddOrder(order Order) (int, []Fill) {
	if order.Side == "buy" {
		executedQty, fills := o.MatchBid(order)
		order.Filled = float64(executedQty)
		if float64(executedQty) == order.Quantity {
			return executedQty, fills
		}
		o.Bids = append(o.Bids, order)
		return executedQty, fills
	} else {
		executedQty, fills := o.MatchAsk(order)
		order.Filled = float64(executedQty)
		if float64(executedQty) == order.Quantity {
			return executedQty, fills
		}
		o.Asks = append(o.Asks, order)
		return executedQty, fills
	}
}

func (o *Orderbook) MatchBid(order Order) (int, []Fill) {
	var fills []Fill
	executedQty := 0

	for i := 0; i < len(o.Asks); i++ {
		if o.Asks[i].Price <= order.Price && float64(executedQty) < order.Quantity {
			filledQty := min(int(order.Quantity-float64(executedQty)), int(o.Asks[i].Quantity))
			executedQty += filledQty
			o.Asks[i].Filled += float64(filledQty)

			o.LastTradeID++
			fills = append(fills, Fill{
				Price:         fmt.Sprintf("%.2f", o.Asks[i].Price),
				Qty:           filledQty,
				TradeID:       o.LastTradeID,
				OtherUserID:   o.Asks[i].UserID,
				MarkerOrderID: o.Asks[i].OrderID,
			})
		}
	}

	// Remove fully filled orders
	for i := len(o.Asks) - 1; i >= 0; i-- {
		if o.Asks[i].Filled == o.Asks[i].Quantity {
			o.Asks = append(o.Asks[:i], o.Asks[i+1:]...)
		}
	}

	return executedQty, fills
}

func (o *Orderbook) MatchAsk(order Order) (int, []Fill) {
	var fills []Fill
	executedQty := 0

	for i := 0; i < len(o.Bids); i++ {
		if o.Bids[i].Price >= order.Price && float64(executedQty) < order.Quantity {
			amountRemaining := min(int(order.Quantity-float64(executedQty)), int(o.Bids[i].Quantity))
			executedQty += amountRemaining
			o.Bids[i].Filled += float64(amountRemaining)

			o.LastTradeID++
			fills = append(fills, Fill{
				Price:         fmt.Sprintf("%.2f", o.Bids[i].Price),
				Qty:           amountRemaining,
				TradeID:       o.LastTradeID,
				OtherUserID:   o.Bids[i].UserID,
				MarkerOrderID: o.Bids[i].OrderID,
			})
		}
	}

	// Remove fully filled orders
	for i := len(o.Bids) - 1; i >= 0; i-- {
		if o.Bids[i].Filled == o.Bids[i].Quantity {
			o.Bids = append(o.Bids[:i], o.Bids[i+1:]...)
		}
	}

	return executedQty, fills
}

func (o *Orderbook) GetDepth() DepthPayload {
	var bids [][2]string
	var asks [][2]string

	bidsObj := make(map[string]float64)
	asksObj := make(map[string]float64)

	for _, order := range o.Bids {
		priceStr := fmt.Sprintf("%.2f", order.Price)
		bidsObj[priceStr] += order.Quantity
	}

	for _, order := range o.Asks {
		priceStr := fmt.Sprintf("%.2f", order.Price)
		asksObj[priceStr] += order.Quantity
	}

	for price, quantity := range bidsObj {
		bids = append(bids, [2]string{price, fmt.Sprintf("%.2f", quantity)})
	}

	for price, quantity := range asksObj {
		asks = append(asks, [2]string{price, fmt.Sprintf("%.2f", quantity)})
	}
	if len(bids) == 0 {
		// If no matching bid found, send zero quantity
		bids = [][2]string{}
	}
	if len(asks) == 0 {
		// If no matching asks found, send zero quantity
		asks = [][2]string{}
	}
	log.Println("Orderbook Depth - Bids:", bids, "Asks:", asks)

	return DepthPayload{
		Bids: bids,
		Asks: asks,
	}
}

func (o *Orderbook) GetOpenOrders(userID string) []Order {
	var orders []Order

	for _, ask := range o.Asks {
		if ask.UserID == userID {
			orders = append(orders, ask)
		}
	}

	for _, bid := range o.Bids {
		if bid.UserID == userID {
			orders = append(orders, bid)
		}
	}

	return orders
}

func (o *Orderbook) CancelBid(order Order) *float64 {
	for i, bid := range o.Bids {
		if bid.OrderID == order.OrderID {
			price := o.Bids[i].Price
			o.Bids = append(o.Bids[:i], o.Bids[i+1:]...)
			return &price
		}
	}
	return nil
}

func (o *Orderbook) CancelAsk(order Order) *float64 {
	for i, ask := range o.Asks {
		if ask.OrderID == order.OrderID {
			price := o.Asks[i].Price
			o.Asks = append(o.Asks[:i], o.Asks[i+1:]...)
			return &price
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func generateOrderID() string {
	fullUUID := uuid.New().String()
	return strings.Split(fullUUID, "-")[0]
}
