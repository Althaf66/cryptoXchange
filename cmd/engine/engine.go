package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const BASE_CURRENCY = "USD"

type UserBalance struct {
	Available float64 `json:"available"`
	Locked    float64 `json:"locked"`
}

type Engine struct {
	Orderbooks map[string]*Orderbook              `json:"orderbooks"`
	Balances   map[string]map[string]*UserBalance `json:"balances"`
}

func NewEngine() *Engine {
	engine := &Engine{
		Orderbooks: make(map[string]*Orderbook),
		Balances:   make(map[string]map[string]*UserBalance),
	}

	// Initialize with SOL orderbook
	solOrderbook := NewOrderbook("SOL", []Order{}, []Order{}, 0, 0)
	engine.Orderbooks[solOrderbook.Ticker()] = solOrderbook
	engine.setBaseBalances()

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

func (e *Engine) SaveSnapshot() {
	snapshot := Engine{
		Orderbooks: e.Orderbooks,
		Balances:   e.Balances,
	}
	// for _, o := range e.Orderbooks {
	// 	snapshot.Orderbooks = append(snapshot.Orderbooks, o.GetSnapshot())
	// }
	data, _ := json.Marshal(snapshot)
	os.WriteFile("../../snapshot.json", data, 0644)
}

func (e *Engine) Process(message MessageFromAPI, clientID string) {
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
	}
}

func (e *Engine) handleCreateOrder(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error creating order: %v", r)
			GetRedisInstance().SendToAPI(clientID, MessageToAPI{
				Type: "ORDER_CANCELLED",
				Payload: OrderCancelledPayload{
					OrderID:      "",
					ExecutedQty:  0,
					RemainingQty: 0,
				},
			})
			log.Printf("order cancelled: %v", r)
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data CreateOrderData
	json.Unmarshal(dataBytes, &data)

	executedQty, fills, orderID := e.CreateOrder(data.Market, data.Price, data.Quantity, data.Side, data.UserID)

	GetRedisInstance().SendToAPI(clientID, MessageToAPI{
		Type: "ORDER_PLACED",
		Payload: OrderPlacedPayload{
			OrderID:     orderID,
			ExecutedQty: executedQty,
			Fills:       fills,
		},
	})
}

func (e *Engine) handleCancelOrder(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error cancelling order: %v", r)
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data CancelOrderData
	json.Unmarshal(dataBytes, &data)

	orderbook, exists := e.Orderbooks[data.Market]
	if !exists {
		log.Println("No orderbook found")
		return
	}

	quoteAsset := strings.Split(data.Market, "_")[1]

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
		log.Println("No order found")
		return
	}

	if order.Side == "buy" {
		price := orderbook.CancelBid(*order)
		leftQuantity := (order.Quantity - order.Filled) * order.Price

		if userBalance, exists := e.Balances[order.UserID]; exists {
			if baseCurrency, exists := userBalance[BASE_CURRENCY]; exists {
				baseCurrency.Available += leftQuantity
				baseCurrency.Locked -= leftQuantity
			}
		}

		if price != nil {
			e.sendUpdatedDepthAt(fmt.Sprintf("%.2f", *price), data.Market)
		}
	} else {
		price := orderbook.CancelAsk(*order)
		leftQuantity := order.Quantity - order.Filled

		if userBalance, exists := e.Balances[order.UserID]; exists {
			if quoteBalance, exists := userBalance[quoteAsset]; exists {
				quoteBalance.Available += leftQuantity
				quoteBalance.Locked -= leftQuantity
			}
		}

		if price != nil {
			e.sendUpdatedDepthAt(fmt.Sprintf("%.2f", *price), data.Market)
		}
	}

	GetRedisInstance().SendToAPI(clientID, MessageToAPI{
		Type: "ORDER_CANCELLED",
		Payload: OrderCancelledPayload{
			OrderID:      data.OrderID,
			ExecutedQty:  0,
			RemainingQty: 0,
		},
	})
}

func (e *Engine) handleGetOpenOrders(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error getting open orders: %v", r)
		}
	}()

	dataBytes, _ := json.Marshal(message.Data)
	var data GetOpenOrdersData
	json.Unmarshal(dataBytes, &data)

	orderbook, exists := e.Orderbooks[data.Market]
	if !exists {
		log.Println("No orderbook found")
		return
	}

	openOrders := orderbook.GetOpenOrders(data.UserID)

	GetRedisInstance().SendToAPI(clientID, MessageToAPI{
		Type:    "OPEN_ORDERS",
		Payload: openOrders,
	})
}

func (e *Engine) handleOnRamp(message MessageFromAPI, clientID string) {
	dataBytes, _ := json.Marshal(message.Data)
	var data OnRampData
	json.Unmarshal(dataBytes, &data)

	amount, err := strconv.ParseFloat(data.Amount, 64)
	if err != nil {
		log.Printf("Error parsing amount: %v", err)
		return
	}
	userid, balance := e.onRamp(data.UserID, amount)
	GetRedisInstance().SendToAPI(clientID, MessageToAPI{
		Type: "ON_RAMP",
		Payload: OnRampPayload{
			UserID:  userid,
			Balance: fmt.Sprintf("%.2f", balance),
		},
	})
}

func (e *Engine) handleGetDepth(message MessageFromAPI, clientID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Error getting depth: %v", r)
			GetRedisInstance().SendToAPI(clientID, MessageToAPI{
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

	GetRedisInstance().SendToAPI(clientID, MessageToAPI{
		Type:    GET_DEPTH,
		Payload: orderbook.GetDepth(),
	})
	log.Printf("Depth for market %s sent to client %s", data.Market, clientID)
}

func (e *Engine) CreateOrder(market, priceStr, quantityStr, side, userID string) (int, []Fill, string) {
	orderbook, exists := e.Orderbooks[market]
	if !exists {
		panic("No orderbook found")
	}

	baseAsset := strings.Split(market, "_")[0]
	quoteAsset := strings.Split(market, "_")[1]

	price, _ := strconv.ParseFloat(priceStr, 64)
	quantity, _ := strconv.ParseFloat(quantityStr, 64)

	e.CheckAndLockFunds(baseAsset, quoteAsset, side, userID, price, quantity)

	order := Order{
		Price:    price,
		Quantity: quantity,
		OrderID:  generateOrderID(),
		Filled:   0,
		Side:     side,
		UserID:   userID,
	}

	executedQty, fills, err := orderbook.AddOrder(order)
	if err != nil {
		log.Printf("Error adding order: %v", err)
		return 0, nil, ""
	}
	e.UpdateBalance(userID, baseAsset, quoteAsset, side, fills, executedQty)

	e.CreateDbTrades(fills, market, userID)
	e.UpdateDbOrders(order, executedQty, fills, market)
	e.publishWSDepthUpdates(fills, priceStr, side, market)
	e.publishWSTrades(fills, userID, market)

	return executedQty, fills, order.OrderID
}

func (e *Engine) CheckAndLockFunds(baseAsset, quoteAsset, side, userID string, price, quantity float64) {
	if _, exists := e.Balances[userID]; !exists {
		e.Balances[userID] = make(map[string]*UserBalance)
	}

	if side == "buy" {
		if _, exists := e.Balances[userID][quoteAsset]; !exists {
			e.Balances[userID][quoteAsset] = &UserBalance{Available: 0, Locked: 0}
		}

		totalCost := quantity * price
		if e.Balances[userID][quoteAsset].Available < totalCost {
			panic("Insufficient funds")
		}

		e.Balances[userID][quoteAsset].Available -= totalCost
		e.Balances[userID][quoteAsset].Locked += totalCost
	} else {
		if _, exists := e.Balances[userID][baseAsset]; !exists {
			e.Balances[userID][baseAsset] = &UserBalance{Available: 0, Locked: 0}
		}

		if e.Balances[userID][baseAsset].Available < quantity {
			panic("Insufficient funds")
		}

		e.Balances[userID][baseAsset].Available -= quantity
		e.Balances[userID][baseAsset].Locked += quantity
	}
}

func (e *Engine) UpdateBalance(userID, baseAsset, quoteAsset, side string, fills []Fill, executedQty int) {
	if side == "buy" {
		for _, fill := range fills {
			fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
			fillQty := float64(fill.Qty)

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
		}
	} else {
		for _, fill := range fills {
			fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
			fillQty := float64(fill.Qty)

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
		}
	}
}

func (e *Engine) CreateDbTrades(fills []Fill, market, userID string) {
	for _, fill := range fills {
		fillPrice, _ := strconv.ParseFloat(fill.Price, 64)
		fillQty := float64(fill.Qty)

		GetRedisInstance().PushMessage(DbMessage{
			Type: TRADE_ADDED,
			Data: TradeAddedData{
				Market:        market,
				ID:            strconv.Itoa(fill.TradeID),
				IsBuyerMaker:  fill.OtherUserID == userID,
				Price:         fill.Price,
				Quantity:      strconv.Itoa(fill.Qty),
				QuoteQuantity: fmt.Sprintf("%.2f", fillQty*fillPrice),
				Timestamp:     time.Now().Unix(),
			},
		})
	}
}
func (e *Engine) UpdateDbOrders(order Order, executedQty int, fills []Fill, market string) {
	side := order.Side
	GetRedisInstance().PushMessage(DbMessage{
		Type: ORDER_UPDATE,
		Data: OrderUpdateData{
			OrderID:     order.OrderID,
			ExecutedQty: executedQty,
			Market:      &market,
			Price:       func() *string { s := fmt.Sprintf("%.2f", order.Price); return &s }(),
			Side:        &side,
		},
	})

	for _, fill := range fills {
		GetRedisInstance().PushMessage(DbMessage{
			Type: ORDER_UPDATE,
			Data: OrderUpdateData{
				OrderID:     fill.MarkerOrderID,
				ExecutedQty: fill.Qty,
			},
		})
	}
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
				Quantity:     strconv.Itoa(fill.Qty),
				Timestamp:    time.Now().Unix(),
			},
		})
	}
}

func (e *Engine) sendUpdatedDepthAt(price, market string) {
	orderbook, exists := e.Orderbooks[market]
	if !exists {
		return
	}

	depth := orderbook.GetDepth()

	var updatedBids [][2]string
	var updatedAsks [][2]string

	// Filter bids that match the price
	for _, bid := range depth.Bids {
		if bid[0] == price {
			updatedBids = append(updatedBids, bid)
		}
	}

	// Filter asks that match the price
	for _, ask := range depth.Asks {
		if ask[0] == price {
			updatedAsks = append(updatedAsks, ask)
		}
	}

	// If no matching bids/asks found, send zero quantity
	if len(updatedBids) == 0 {
		updatedBids = [][2]string{}
	}
	if len(updatedAsks) == 0 {
		updatedAsks = [][2]string{}
	}
	log.Printf("sendUpdated,Updated bids: %v, Updated asks: %v", updatedBids, updatedAsks)
	log.Printf("Publishing depth update for market %s at price %s", market, price)
	GetRedisInstance().PublishMessage(fmt.Sprintf("depth@%s", market), WsMessage{
		Stream: fmt.Sprintf("depth@%s", market),
		Data: &DepthData{
			B: updatedBids, // Send as array (empty if no match found)
			A: updatedAsks,
			E: "depth",
		},
	})
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

func (e *Engine) onRamp(userID string, amount float64) (string, float64) {
	log.Printf("OnRamp: User %s depositing %.2f %s", userID, amount, BASE_CURRENCY)
	if _, exists := e.Balances[userID]; !exists {
		e.Balances[userID] = make(map[string]*UserBalance)
		e.Balances[userID][BASE_CURRENCY] = &UserBalance{
			Available: amount,
			Locked:    0,
		}
		log.Printf("Created new balance for user %s with %.2f %s", userID, e.Balances[userID][BASE_CURRENCY].Available, BASE_CURRENCY)
	} else {
		if _, exists := e.Balances[userID][BASE_CURRENCY]; !exists {
			e.Balances[userID][BASE_CURRENCY] = &UserBalance{
				Available: 0,
				Locked:    0,
			}
		}
		log.Printf("User %s already exists, adding %.2f %s to their balance", userID, amount, BASE_CURRENCY)
		log.Printf("To adding %.2f ", e.Balances[userID][BASE_CURRENCY].Available)
		e.Balances[userID][BASE_CURRENCY].Available += amount
		log.Printf("added %.2f ", e.Balances[userID][BASE_CURRENCY].Available)
	}
	return userID, e.Balances[userID][BASE_CURRENCY].Available
}

func (e *Engine) setBaseBalances() {
	// Initialize balances for user "1"
	e.Balances["1"] = make(map[string]*UserBalance)
	e.Balances["1"][BASE_CURRENCY] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}
	e.Balances["1"]["SOL"] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}

	// Initialize balances for user "2"
	e.Balances["2"] = make(map[string]*UserBalance)
	e.Balances["2"][BASE_CURRENCY] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}
	e.Balances["2"]["SOL"] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}

	// Initialize balances for user "5"
	e.Balances["5"] = make(map[string]*UserBalance)
	e.Balances["5"][BASE_CURRENCY] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}
	e.Balances["5"]["SOL"] = &UserBalance{
		Available: 10000000,
		Locked:    0,
	}
}
