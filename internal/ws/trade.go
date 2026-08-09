package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Helper function to simulate a trade fill (for testing purposes)
func SimulateTradeFill(userID, symbol, side string, quantity, price float64) {
	tradeUpdate := TradeUpdateMessage{
		Type: "trade",
		Data: TradeData{
			TradeID:         generateTradeID(),
			OrderID:         generateOrderID(),
			UserID:          userID,
			Symbol:          symbol,
			Side:            side,
			Quantity:        fmt.Sprintf("%.8f", quantity),
			Price:           fmt.Sprintf("%.8f", price),
			FilledQty:       fmt.Sprintf("%.8f", quantity),
			RemainingQty:    "0.00000000",
			Status:          "filled",
			Timestamp:       time.Now().UnixMilli(),
			Commission:      fmt.Sprintf("%.8f", quantity*price*0.001), // 0.1% commission
			CommissionAsset: "USDT",
			E:               "trade",
		},
	}

	// Publish to Redis on the user's trade channel
	channel := "trades:" + userID
	messageBytes, _ := json.Marshal(tradeUpdate)
	SubscriptionManager.redisClient.Publish(context.Background(), channel, string(messageBytes))
}

func generateTradeID() string {
	return fmt.Sprintf("T%d", time.Now().UnixNano())
}

func generateOrderID() string {
	return fmt.Sprintf("O%d", time.Now().UnixNano())
}
