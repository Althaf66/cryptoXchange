package main

import (
	"fmt"
	"sort"
	"strconv"

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
	Price         string  `json:"price"`
	Qty           float64 `json:"qty"`
	TradeID       int     `json:"tradeId"`
	OtherUserID   string  `json:"otherUserId"`
	MarkerOrderID string  `json:"markerOrderId"`
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

// AddOrder matches an incoming order against the book. restRemainder controls
// whether any unfilled quantity is left resting: limit orders rest, market
// orders do not.
func (o *Orderbook) AddOrder(order Order, restRemainder bool) (float64, []Fill, error) {
	// Validate the order first
	if err := o.validateOrder(order); err != nil {
		return 0, nil, err
	}

	var executedQty float64
	var fills []Fill
	if order.Side == "buy" {
		executedQty, fills = o.MatchBid(order)
	} else {
		executedQty, fills = o.MatchAsk(order)
	}

	if !restRemainder || executedQty >= order.Quantity {
		return executedQty, fills, nil
	}

	// Only add remaining quantity to orderbook
	order.Filled = 0
	order.Quantity -= executedQty
	if order.Side == "buy" {
		o.insertBidInOrder(order)
	} else {
		o.insertAskInOrder(order)
	}
	return executedQty, fills, nil
}

func (o *Orderbook) MatchBid(order Order) (float64, []Fill) {
	var fills []Fill
	executedQty := 0.0

	// Sort asks by price (lowest first) to ensure best price matching
	// In a real system, this should be maintained as a sorted structure
	for i := 0; i < len(o.Asks) && executedQty < order.Quantity; i++ {
		ask := &o.Asks[i]

		// Check if this ask can be matched with the bid
		if ask.Price <= order.Price {
			// Calculate remaining quantity available in the ask order
			askRemainingQty := ask.Quantity - ask.Filled
			if askRemainingQty <= 0 {
				continue // Skip fully filled orders
			}

			// Calculate how much can be filled
			bidRemainingQty := order.Quantity - executedQty
			filledQty := min(bidRemainingQty, askRemainingQty)

			if filledQty > 0 {
				executedQty += filledQty
				ask.Filled += filledQty

				// Update current price to the trade price
				o.CurrentPrice = ask.Price

				o.LastTradeID++
				fills = append(fills, Fill{
					Price:         formatNum(ask.Price),
					Qty:           filledQty,
					TradeID:       o.LastTradeID,
					OtherUserID:   ask.UserID,
					MarkerOrderID: ask.OrderID,
				})
			}
		}
	}

	// Remove fully filled orders
	o.Asks = o.removeFullyFilledOrders(o.Asks)

	return executedQty, fills
}

func (o *Orderbook) MatchAsk(order Order) (float64, []Fill) {
	var fills []Fill
	executedQty := 0.0

	// Sort bids by price (highest first) to ensure best price matching
	// In a real system, this should be maintained as a sorted structure
	for i := 0; i < len(o.Bids) && executedQty < order.Quantity; i++ {
		bid := &o.Bids[i]

		// Check if this bid can be matched with the ask
		if bid.Price >= order.Price {
			// Calculate remaining quantity available in the bid order
			bidRemainingQty := bid.Quantity - bid.Filled
			if bidRemainingQty <= 0 {
				continue // Skip fully filled orders
			}

			// Calculate how much can be filled
			askRemainingQty := order.Quantity - executedQty
			filledQty := min(askRemainingQty, bidRemainingQty)

			if filledQty > 0 {
				executedQty += filledQty
				bid.Filled += filledQty

				// Update current price to the trade price
				o.CurrentPrice = bid.Price

				o.LastTradeID++
				fills = append(fills, Fill{
					Price:         formatNum(bid.Price),
					Qty:           filledQty,
					TradeID:       o.LastTradeID,
					OtherUserID:   bid.UserID,
					MarkerOrderID: bid.OrderID,
				})
			}
		}
	}

	// Remove fully filled orders
	o.Bids = o.removeFullyFilledOrders(o.Bids)

	return executedQty, fills
}

func (o *Orderbook) GetDepth() DepthPayload {
	return o.GetDepthWithLimit(20) // Default to 20 levels
}

// GetDepthWithLimit returns orderbook depth with a specified limit of price levels
func (o *Orderbook) GetDepthWithLimit(limit int) DepthPayload {
	bidsMap := make(map[string]float64)
	asksMap := make(map[string]float64)

	// Aggregate bids by price level (only unfilled quantities)
	for _, order := range o.Bids {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 {
			priceStr := formatNum(order.Price)
			bidsMap[priceStr] += remainingQty
		}
	}

	// Aggregate asks by price level (only unfilled quantities)
	for _, order := range o.Asks {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 {
			priceStr := formatNum(order.Price)
			asksMap[priceStr] += remainingQty
		}
	}

	// Convert to sorted arrays
	bids := o.sortBidsDescending(bidsMap, limit)
	asks := o.sortAsksAscending(asksMap, limit)

	return DepthPayload{
		Bids: bids,
		Asks: asks,
	}
}

func (o *Orderbook) GetOpenOrders(userID string) []Order {
	orders := []Order{}

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

func (o *Orderbook) insertBidInOrder(order Order) {
	inserted := false
	for i, bid := range o.Bids {
		if order.Price > bid.Price {
			// Insert before this bid (higher price)
			o.Bids = append(o.Bids[:i], append([]Order{order}, o.Bids[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		// Append at the end (lowest price or empty slice)
		o.Bids = append(o.Bids, order)
	}
}

// insertAskInOrder inserts an ask order maintaining price-time priority (lowest price first)
func (o *Orderbook) insertAskInOrder(order Order) {
	inserted := false
	for i, ask := range o.Asks {
		if order.Price < ask.Price {
			// Insert before this ask (lower price)
			o.Asks = append(o.Asks[:i], append([]Order{order}, o.Asks[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		// Append at the end (highest price or empty slice)
		o.Asks = append(o.Asks, order)
	}
}

// generateOrderID returns a full UUID. It used to keep only the first segment —
// 32 bits — which the market maker exhausts fast: at ~900k orders a week against
// an orders table retaining 2 days, roughly 55 inserts a week collided with an
// unrelated existing row. That does not error, because the row is written with
// ON CONFLICT (order_id) DO UPDATE, so the new order's executed quantity was
// silently added to a stranger's order.
func generateOrderID() string {
	return uuid.New().String()
}

// dustEpsilon is the quantity below which a remainder is treated as zero.
// Filled accumulates as a sum of float64 fills, so 0.1+0.2 lands on
// 0.30000000000000004 and the mirror case lands just under — leaving an order
// resting with ~1e-17 outstanding that still holds its owner's lock.
// Every market here quotes to at most 5 decimals (internal/markets), so 1e-9 is
// far below anything real and far above the noise.
const dustEpsilon = 1e-9

// removeFullyFilledOrders removes orders that have been completely filled,
// treating a dust remainder as complete.
func (o *Orderbook) removeFullyFilledOrders(orders []Order) []Order {
	result := make([]Order, 0, len(orders))
	for _, order := range orders {
		if order.Quantity-order.Filled > dustEpsilon {
			result = append(result, order)
		}
	}
	return result
}

// validateOrder checks if an order is valid before processing
func (o *Orderbook) validateOrder(order Order) error {
	if order.Price <= 0 {
		return fmt.Errorf("invalid price: %f", order.Price)
	}
	if order.Quantity <= 0 {
		return fmt.Errorf("invalid quantity: %f", order.Quantity)
	}
	if order.UserID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if order.OrderID == "" {
		return fmt.Errorf("order ID cannot be empty")
	}
	if order.Side != "buy" && order.Side != "sell" {
		return fmt.Errorf("invalid side: %s", order.Side)
	}
	return nil
}

// sortBidsDescending sorts bids by price in descending order (highest price first)
// and returns limited number of price levels
func (o *Orderbook) sortBidsDescending(bidsMap map[string]float64, limit int) [][2]string {
	if len(bidsMap) == 0 {
		return [][2]string{}
	}

	// Convert map to slice of price-quantity pairs for sorting
	type priceLevel struct {
		price    float64
		priceStr string
		quantity float64
	}

	levels := make([]priceLevel, 0, len(bidsMap))
	for priceStr, quantity := range bidsMap {
		if quantity > 0 { // Only include levels with positive quantity
			if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
				levels = append(levels, priceLevel{
					price:    price,
					priceStr: priceStr,
					quantity: quantity,
				})
			}
		}
	}

	// Sort by price descending (highest first)
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].price > levels[j].price
	})

	// Apply limit and convert to result format
	maxLevels := limit
	if len(levels) < maxLevels {
		maxLevels = len(levels)
	}

	result := make([][2]string, maxLevels)
	for i := 0; i < maxLevels; i++ {
		result[i] = [2]string{
			levels[i].priceStr,
			formatNum(levels[i].quantity),
		}
	}

	return result
}

// sortAsksAscending sorts asks by price in ascending order (lowest price first)
// and returns limited number of price levels
func (o *Orderbook) sortAsksAscending(asksMap map[string]float64, limit int) [][2]string {
	if len(asksMap) == 0 {
		return [][2]string{}
	}

	// Convert map to slice of price-quantity pairs for sorting
	type priceLevel struct {
		price    float64
		priceStr string
		quantity float64
	}

	levels := make([]priceLevel, 0, len(asksMap))
	for priceStr, quantity := range asksMap {
		if quantity > 0 { // Only include levels with positive quantity
			if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
				levels = append(levels, priceLevel{
					price:    price,
					priceStr: priceStr,
					quantity: quantity,
				})
			}
		}
	}

	// Sort by price ascending (lowest first)
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].price < levels[j].price
	})

	// Apply limit and convert to result format
	maxLevels := limit
	if len(levels) < maxLevels {
		maxLevels = len(levels)
	}

	result := make([][2]string, maxLevels)
	for i := 0; i < maxLevels; i++ {
		result[i] = [2]string{
			levels[i].priceStr,
			formatNum(levels[i].quantity),
		}
	}

	return result
}

// GetBestBidAsk returns the best bid and ask prices with their quantities
func (o *Orderbook) GetBestBidAsk() (bestBid, bestAsk *[2]string) {
	// Find best bid (highest price)
	var highestBidPrice float64 = -1
	var bestBidQty float64

	for _, order := range o.Bids {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 && order.Price > highestBidPrice {
			highestBidPrice = order.Price
			bestBidQty = remainingQty
		} else if remainingQty > 0 && order.Price == highestBidPrice {
			bestBidQty += remainingQty
		}
	}

	// Find best ask (lowest price)
	var lowestAskPrice float64 = -1
	var bestAskQty float64

	for _, order := range o.Asks {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 {
			if lowestAskPrice == -1 || order.Price < lowestAskPrice {
				lowestAskPrice = order.Price
				bestAskQty = remainingQty
			} else if order.Price == lowestAskPrice {
				bestAskQty += remainingQty
			}
		}
	}

	// Format results
	if highestBidPrice > 0 {
		bid := [2]string{
			formatNum(highestBidPrice),
			formatNum(bestBidQty),
		}
		bestBid = &bid
	}

	if lowestAskPrice > 0 {
		ask := [2]string{
			formatNum(lowestAskPrice),
			formatNum(bestAskQty),
		}
		bestAsk = &ask
	}

	return bestBid, bestAsk
}

// GetSpread returns the bid-ask spread
func (o *Orderbook) GetSpread() (spread float64, spreadPercent float64) {
	bestBid, bestAsk := o.GetBestBidAsk()

	if bestBid == nil || bestAsk == nil {
		return 0, 0
	}

	bidPrice, _ := strconv.ParseFloat((*bestBid)[0], 64)
	askPrice, _ := strconv.ParseFloat((*bestAsk)[0], 64)

	spread = askPrice - bidPrice
	if bidPrice > 0 {
		spreadPercent = (spread / bidPrice) * 100
	}

	return spread, spreadPercent
}

// GetDepthStats returns additional statistics about the orderbook depth
func (o *Orderbook) GetDepthStats() map[string]interface{} {
	totalBidVolume := 0.0
	totalAskVolume := 0.0
	bidOrderCount := 0
	askOrderCount := 0

	for _, order := range o.Bids {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 {
			totalBidVolume += remainingQty
			bidOrderCount++
		}
	}

	for _, order := range o.Asks {
		remainingQty := order.Quantity - order.Filled
		if remainingQty > 0 {
			totalAskVolume += remainingQty
			askOrderCount++
		}
	}

	bestBid, bestAsk := o.GetBestBidAsk()
	spread, spreadPercent := o.GetSpread()

	stats := map[string]interface{}{
		"totalBidVolume": formatNum(totalBidVolume),
		"totalAskVolume": formatNum(totalAskVolume),
		"bidOrderCount":  bidOrderCount,
		"askOrderCount":  askOrderCount,
		"spread":         formatNum(spread),
		"spreadPercent":  fmt.Sprintf("%.4f", spreadPercent),
		"currentPrice":   formatNum(o.CurrentPrice),
	}

	if bestBid != nil {
		stats["bestBid"] = *bestBid
	}
	if bestAsk != nil {
		stats["bestAsk"] = *bestAsk
	}

	return stats
}
