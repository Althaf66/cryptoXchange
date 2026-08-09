package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Althaf66/cryptoXchange/internal/ws"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()

	v1 := r.PathPrefix("/v1").Subrouter()
	v1.HandleFunc("/ws", handleWebSocket)

	addr := os.Getenv("WS_PORT")
	if addr == "" {
		addr = ":3001"
	}

	ws.SubscriptionManager.Init()
	log.Printf("WebSocket endpoint listening on %s/v1/ws", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
