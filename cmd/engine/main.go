package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/rediscfg"
	"github.com/go-redis/redis/v8"
)

func main() {
	engine := NewEngine()

	rdb := redis.NewClient(rediscfg.Options())

	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	fmt.Println("Connected to Redis")

	// The platform sends SIGTERM on every redeploy and reschedule. Without this
	// the process dies between snapshot ticks, losing up to 5s of state that
	// Postgres already recorded — a divergence nothing can repair afterwards.
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
		<-sigs
		log.Println("shutting down, saving snapshot")
		engine.SaveSnapshot()
		os.Exit(0)
	}()

	for {
		// BRPop blocks instead of spinning: the old RPop loop pegged a core at
		// 100% whenever the queue was empty, which is all the time.
		result, err := rdb.BRPop(ctx, 0, "messages").Result()
		if err == redis.Nil {
			continue
		} else if err != nil {
			log.Printf("Error popping from Redis: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if len(result) < 2 {
			continue
		}
		log.Println("Received message lpush:", result[1])

		var message struct {
			ClientID string         `json:"clientId"`
			Message  MessageFromAPI `json:"message"`
		}

		if err := json.Unmarshal([]byte(result[1]), &message); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		engine.Process(message.Message, message.ClientID)
		log.Printf("Processed message for client %s: %v", message.ClientID, message.Message)
	}
}
