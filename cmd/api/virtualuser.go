package main

import (
	"net/http"

	"github.com/google/uuid"
)

const (
	CREATE_USER = "CREATE_USER"
	GET_USERS   = "GET_USERS"
)

type CreateUserRequest struct {
	Name   string `json:"name"`
	Amount string `json:"amount"` // optional; credited to every asset
}

// The engine data struct. Amount stays a string end to end, as ON_RAMP does.
type CreateUserData struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	TxnID  string `json:"txnId"`
}

// createVirtualUserHandler mints a demo account and optionally funds it across
// every asset. Unlike /onramp there is no transfers row: that table is the
// idempotency key for real USD deposits, while this is a demo grant whose only
// record is the ledger rows the engine emits.
func (app *application) createVirtualUserHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := ReadJSON(w, r, &req); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: CREATE_USER,
		Data: CreateUserData{Name: req.Name, Amount: req.Amount, TxnID: uuid.NewString()},
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	writeEngineResponse(w, http.StatusCreated, response)
}

func (app *application) getVirtualUsersHandler(w http.ResponseWriter, r *http.Request) {
	response, err := redisManager.SendAndAwait(r.Context(), MessageToEngine{
		Type: GET_USERS,
		Data: struct{}{},
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}
