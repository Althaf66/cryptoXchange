package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type MessageToEngine struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type MessageFromOrderbook struct {
	Payload interface{} `json:"payload"`
}

type RedisMessage struct {
	ClientID string          `json:"clientId"`
	Message  MessageToEngine `json:"message"`
}

type RedisManager struct {
	client    *redis.Client
	publisher *redis.Client
}

var redisManager *RedisManager

// Initialize Redis manager
func NewRedisManager() *RedisManager {
	if redisManager == nil {
		redisManager = &RedisManager{
			client: redis.NewClient(&redis.Options{
				Addr: "localhost:6379",
			}),
			publisher: redis.NewClient(&redis.Options{
				Addr: "localhost:6379",
			}),
		}
	}
	return redisManager
}

// SendAndAwait sends a message and waits for response with timeout and retry logic
func (rm *RedisManager) SendAndAwait(ctx context.Context, message MessageToEngine) (*MessageFromOrderbook, error) {
	return rm.SendAndAwaitWithTimeout(ctx, message, 30*time.Second, 3)
}

// SendAndAwaitWithTimeout provides configurable timeout and retry options
func (rm *RedisManager) SendAndAwaitWithTimeout(ctx context.Context, message MessageToEngine, timeout time.Duration, maxRetries int) (*MessageFromOrderbook, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retrying SendAndAwait, attempt %d/%d", attempt+1, maxRetries)
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		response, err := rm.sendAndAwaitSingle(ctx, message, timeout)
		if err == nil {
			return response, nil
		}

		lastErr = err
		log.Printf("SendAndAwait attempt %d failed: %v", attempt+1, err)

		// Don't retry on context cancellation
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}

	return nil, lastErr
}

// sendAndAwaitSingle performs a single attempt to send and receive
func (rm *RedisManager) sendAndAwaitSingle(ctx context.Context, message MessageToEngine, timeout time.Duration) (*MessageFromOrderbook, error) {
	clientID := rm.getRandomClientID()

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Subscribe to response channel BEFORE sending the message
	pubsub := rm.client.Subscribe(timeoutCtx, clientID)
	defer pubsub.Close()

	// Wait for subscription to be confirmed
	_, err := pubsub.ReceiveTimeout(timeoutCtx, 5*time.Second)
	if err != nil {
		return nil, errors.New("failed to confirm subscription: " + err.Error())
	}

	// Create message
	redisMsg := RedisMessage{
		ClientID: clientID,
		Message:  message,
	}

	log.Printf("Sending message to engine with clientID %s: %+v", clientID, message)

	msgBytes, err := json.Marshal(redisMsg)
	if err != nil {
		return nil, errors.New("failed to marshal message: " + err.Error())
	}

	// Send message to engine
	err = rm.publisher.LPush(timeoutCtx, "messages", string(msgBytes)).Err()
	if err != nil {
		return nil, errors.New("failed to send message to Redis: " + err.Error())
	}

	log.Printf("Message sent to engine successfully, waiting for response on channel: %s", clientID)

	// Wait for response with timeout
	select {
	case <-timeoutCtx.Done():
		return nil, errors.New("timeout waiting for response")
	default:
		// Use a channel to handle the response asynchronously
		responseChan := make(chan *MessageFromOrderbook, 1)
		errorChan := make(chan error, 1)

		go func() {
			defer close(responseChan)
			defer close(errorChan)

			// Keep trying to receive messages until we get one or timeout
			for {
				select {
				case <-timeoutCtx.Done():
					errorChan <- timeoutCtx.Err()
					return
				default:
					msg, err := pubsub.ReceiveMessage(timeoutCtx)
					if err != nil {
						if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
							errorChan <- err
							return
						}
						// Log non-fatal errors and continue
						log.Printf("Error receiving message (will retry): %v", err)
						time.Sleep(100 * time.Millisecond)
						continue
					}

					log.Printf("Received response from engine: %s", msg.Payload)

					var response MessageFromOrderbook
					err = json.Unmarshal([]byte(msg.Payload), &response)
					if err != nil {
						log.Printf("Error unmarshaling response: %v", err)
						continue // Try to receive another message
					}

					responseChan <- &response
					return
				}
			}
		}()

		// Wait for either response or error
		select {
		case response := <-responseChan:
			if response != nil {
				log.Printf("Successfully received and unmarshaled response: %+v", response)
				return response, nil
			}
		case err := <-errorChan:
			return nil, err
		case <-timeoutCtx.Done():
			return nil, errors.New("timeout waiting for response")
		}
	}

	return nil, errors.New("unexpected end of function")
}

func (rm *RedisManager) getRandomClientID() string {
	fullUUID := uuid.New().String()
	return strings.Split(fullUUID, "-")[0]
}
