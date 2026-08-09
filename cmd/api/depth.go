package main

import (
	"net/http"
)

const GET_DEPTH = "GET_DEPTH"

// defaultMarket is the only market the engine boots with today.
const defaultMarket = "SOL_USD"

type GetDepthData struct {
	Market string `json:"market"`
}

func (app *application) getDepthHandler(w http.ResponseWriter, r *http.Request) {
	market := r.URL.Query().Get("market")
	if market == "" {
		market = defaultMarket
	}

	data := GetDepthData{
		Market: market,
	}

	message := MessageToEngine{
		Type: GET_DEPTH,
		Data: data,
	}

	response, err := redisManager.SendAndAwait(r.Context(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}
