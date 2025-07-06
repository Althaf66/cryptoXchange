package main

import (
	"log"
	"net/http"

	"github.com/Althaf66/cryptoXchange/internal/ws"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()

	v1 := r.PathPrefix("/v1").Subrouter()
	v1.HandleFunc("/ws", handleWebSocket)

	ws.SubscriptionManager.Init()
	log.Println("Server starting on :3001")
	if err := http.ListenAndServe(":3001", r); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
	log.Println("WebSocket endpoint: ws://localhost:3001/v1/ws")
}
