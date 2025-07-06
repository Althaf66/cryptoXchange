package main

import (
	"net/http"

	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/gorilla/mux"
)

const ON_RAMP = "ON_RAMP"

type OnRampRequest struct {
	UserId string `json:"userId"`
	Amount string `json:"amount"`
}

func (app *application) balanceHandler(w http.ResponseWriter, r *http.Request) {
	userId := mux.Vars(r)["userId"]

	balance, err := app.store.Balances.GetBalanceById(r.Context(), userId)
	if err != nil {
		switch err {
		case store.ErrUserNotFound:
			app.notFoundResponse(w, r, err)
			return
		default:
			app.internalServerError(w, r, err)
			return
		}
	}

	if err := JsonResponse(w, http.StatusOK, balance); err != nil {
		app.internalServerError(w, r, err)
	}
}

func (app *application) onRampHandler(w http.ResponseWriter, r *http.Request) {
	var req OnRampRequest
	if err := ReadJSON(w, r, &req); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if req.UserId == "" || req.Amount <= "" {
		http.Error(w, "userId and positive amount are required", http.StatusBadRequest)
		return
	}

	response, err := NewRedisManager().SendAndAwait(r.Context(), MessageToEngine{
		Type: ON_RAMP,
		Data: req,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	WriteJSON(w, http.StatusOK, response.Payload)
}
