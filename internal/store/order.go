package store

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type OrderStore struct {
	db *sql.DB
}

type Order struct {
	OrderID     string    `json:"orderId"`
	UserID      string    `json:"userId"`
	Market      string    `json:"market"`
	Side        string    `json:"side"`
	Price       float64   `json:"price"`
	Quantity    float64   `json:"quantity"`
	ExecutedQty float64   `json:"executedQty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ListByUser returns a user's order history, newest first. market is optional;
// the branching mirrors GetRecentTrades in trade.go.
func (o *OrderStore) ListByUser(userID, market string, limit int) ([]Order, error) {
	var query string
	var args []interface{}

	if market != "" {
		query = `
			SELECT order_id, user_id, market, side, price, quantity, executed_qty, status, created_at, updated_at
			FROM orders
			WHERE user_id = $1 AND market = $2
			ORDER BY created_at DESC
			LIMIT $3`
		args = []interface{}{userID, market, limit}
	} else {
		query = `
			SELECT order_id, user_id, market, side, price, quantity, executed_qty, status, created_at, updated_at
			FROM orders
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2`
		args = []interface{}{userID, limit}
	}

	rows, err := o.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []Order{}
	for rows.Next() {
		var ord Order
		if err := rows.Scan(&ord.OrderID, &ord.UserID, &ord.Market, &ord.Side,
			&ord.Price, &ord.Quantity, &ord.ExecutedQty, &ord.Status,
			&ord.CreatedAt, &ord.UpdatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, ord)
	}
	return orders, rows.Err()
}
