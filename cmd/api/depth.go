package main

import (
	"net/http"
)

const GET_DEPTH = "GET_DEPTH"

type GetDepthData struct {
	Market string `json:"market"`
}

func (app *application) getDepthHandler(w http.ResponseWriter, r *http.Request) {
	// symbol := r.URL.Query().Get("symbol")

	data := GetDepthData{
		Market: "SOL_USD",
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
