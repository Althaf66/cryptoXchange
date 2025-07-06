package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/go-redis/redis/v8"
)

func main() {
	engine := NewEngine()

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	fmt.Println("Connected to Redis")

	for {
		result, err := rdb.RPop(ctx, "messages").Result()
		if err == redis.Nil {
			// No message, continue
			continue
		} else if err != nil {
			log.Printf("Error popping from Redis: %v", err)
			continue
		}
		log.Println("Received message lpush:", result)

		var message struct {
			ClientID string         `json:"clientId"`
			Message  MessageFromAPI `json:"message"`
		}

		if err := json.Unmarshal([]byte(result), &message); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		engine.Process(message.Message, message.ClientID)
		log.Printf("Processed message for client %s: %v", message.ClientID, message.Message)
	}
}
