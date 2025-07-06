package main

import (
	// "context"
	// "fmt"
	"log"
	"net/http"
)

const (
	CREATE_ORDER    = "CREATE_ORDER"
	CANCEL_ORDER    = "CANCEL_ORDER"
	GET_OPEN_ORDERS = "GET_OPEN_ORDERS"
)

type CreateOrderData struct {
	Market   string `json:"market"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Side     string `json:"side"` // "buy" or "sell"
	UserID   string `json:"userId"`
}

type CancelOrderData struct {
	OrderID string `json:"orderId"`
	Market  string `json:"market"`
}

type GetOpenOrdersData struct {
	UserID string `json:"userId"`
	Market string `json:"market"`
}

// type OrderRequest struct {
// 	Market   string  `json:"market"`
// 	Price    float64 `json:"price"`
// 	Quantity float64 `json:"quantity"`
// 	Side     string  `json:"side"`
// 	UserId   string  `json:"userId"`
// }

// type CancelOrderRequest struct {
// 	OrderId string `json:"orderId"`
// 	Market  string `json:"market"`
// }

// func (app *application) createOrderHandler(w http.ResponseWriter, r *http.Request) {
// 	var req OrderRequest
// 	if err := ReadJSON(w, r, &req); err != nil {
// 		app.badRequestResponse(w, r, err)
// 		return
// 	}
// 	fmt.Println("message:", req)
// 	response, err := GetRedisManager().SendAndAwait(context.Background(), MessageToEngine{
// 		Type: CREATE_ORDER,
// 		Data: req,
// 	})
// 	if err != nil {
// 		app.internalServerError(w, r, err)
// 		return
// 	}
// 	WriteJSON(w, http.StatusCreated, response.Payload)
// }

// func (app *application) cancelOrderHandler(w http.ResponseWriter, r *http.Request) {
// 	var req CancelOrderRequest
// 	if err := ReadJSON(w, r, &req); err != nil {
// 		app.badRequestResponse(w, r, err)
// 		return
// 	}

// 	response, err := GetRedisManager().SendAndAwait(context.Background(), MessageToEngine{
// 		Type: CANCEL_ORDER,
// 		Data: req,
// 	})
// 	if err != nil {
// 		app.internalServerError(w, r, err)
// 		return
// 	}
// 	WriteJSON(w, http.StatusOK, response)
// }

// func (app *application) getOpenOrdersHandler(w http.ResponseWriter, r *http.Request) {
// 	userId := r.URL.Query().Get("userId")
// 	market := r.URL.Query().Get("market")

// 	response, err := GetRedisManager().SendAndAwait(context.Background(), MessageToEngine{
// 		Type: GET_OPEN_ORDERS,
// 		Data: map[string]string{
// 			"userId": userId,
// 			"market": market,
// 		},
// 	})
// 	if err != nil {
// 		app.internalServerError(w, r, err)
// 		return
// 	}

// 	WriteJSON(w, http.StatusOK, response)
// }

func (app *application) createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var orderData CreateOrderData
	if err := ReadJSON(w, r, &orderData); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	log.Printf("Order data: %+v", orderData)

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
	WriteJSON(w, http.StatusCreated, response.Payload)
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

	WriteJSON(w, http.StatusCreated, response.Payload)
}

func (app *application) getOpenOrdersHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	market := r.URL.Query().Get("market")

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

	WriteJSON(w, http.StatusCreated, response.Payload)
}
