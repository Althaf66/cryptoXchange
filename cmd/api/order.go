package main

import (
	"log"
	"net/http"
)

const (
	CREATE_ORDER    = "CREATE_ORDER"
	CANCEL_ORDER    = "CANCEL_ORDER"
	GET_OPEN_ORDERS = "GET_OPEN_ORDERS"
	GET_BALANCE     = "GET_BALANCE"
)

type CreateOrderData struct {
	Market   string `json:"market"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Side     string `json:"side"` // "buy" or "sell"
	UserID   string `json:"userId"`
	Type     string `json:"type"` // "limit" (default) or "market"
}

// rejectionStatus maps an engine rejection code onto an HTTP status. Without it
// a rejected order came back as 201 Created with an empty payload.
func rejectionStatus(code string) int {
	switch code {
	case "INSUFFICIENT_FUNDS", "INVALID_ORDER", "NO_LIQUIDITY", "INVALID_USER":
		return http.StatusBadRequest
	case "NO_ORDERBOOK":
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// writeEngineResponse forwards an engine reply, turning ORDER_REJECTED into a
// real error status the frontend can show the user.
func writeEngineResponse(w http.ResponseWriter, okStatus int, response *MessageFromOrderbook) {
	if response.Type == "ORDER_REJECTED" {
		payload, _ := response.Payload.(map[string]interface{})
		reason, _ := payload["reason"].(string)
		code, _ := payload["code"].(string)
		if reason == "" {
			reason = "order rejected"
		}
		WriteJSON(w, rejectionStatus(code), map[string]string{"error": reason, "code": code})
		return
	}
	WriteJSON(w, okStatus, response.Payload)
}

type CancelOrderData struct {
	OrderID string `json:"orderId"`
	Market  string `json:"market"`
}

type GetOpenOrdersData struct {
	UserID string `json:"userId"`
	Market string `json:"market"`
}

func (app *application) createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var orderData CreateOrderData
	if err := ReadJSON(w, r, &orderData); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	log.Printf("Order data: %+v", orderData)

	if orderData.Type == "" {
		orderData.Type = "limit"
	}

	message := MessageToEngine{
		Type: CREATE_ORDER,
		Data: orderData,
	}

	response, err := redisManager.SendAndAwait(r.Context(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Payload data: %+v", response.Payload)
	writeEngineResponse(w, http.StatusCreated, response)
}

func (app *application) cancelOrderHandler(w http.ResponseWriter, r *http.Request) {
	var cancelData CancelOrderData
	if err := ReadJSON(w, r, &cancelData); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	message := MessageToEngine{
		Type: CANCEL_ORDER,
		Data: cancelData,
	}

	response, err := redisManager.SendAndAwait(r.Context(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}

// orderHistoryHandler reads from Postgres, not the engine: the engine only
// knows orders still resting on the book, so filled and cancelled ones are
// invisible to /order/open.
func (app *application) orderHistoryHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}

	orders, err := app.store.Orders.ListByUser(userID, r.URL.Query().Get("market"), parseLimit(r, 50))
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, orders)
}

func (app *application) getOpenOrdersHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	market := r.URL.Query().Get("market")
	if market == "" {
		market = defaultMarket
	}

	data := GetOpenOrdersData{
		UserID: userID,
		Market: market,
	}

	message := MessageToEngine{
		Type: GET_OPEN_ORDERS,
		Data: data,
	}

	response, err := redisManager.SendAndAwait(r.Context(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}
