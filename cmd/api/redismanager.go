package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/rediscfg"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type MessageToEngine struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type MessageFromOrderbook struct {
	Type    string      `json:"type"`
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
			client:    redis.NewClient(rediscfg.Options()),
			publisher: redis.NewClient(rediscfg.Options()),
		}
	}
	return redisManager
}

// isMutating reports whether re-sending a command could change state twice.
// The engine consumes "messages" with a blocking BRPop, so a command that
// timed out has not necessarily been skipped — it may simply be queued.
// Retrying those places the same order, or credits the same deposit, again.
func isMutating(msgType string) bool {
	switch msgType {
	case CREATE_ORDER, CANCEL_ORDER, ON_RAMP, CREATE_USER:
		return true
	}
	return false
}

// SendAndAwait sends a message and waits for a response.
//
// Reads are retried; writes are not. The engine now replies on every path
// (including "order not found"), so a missing reply means the engine is gone or
// wedged — in which case a retry only queues a second copy of the command.
func (rm *RedisManager) SendAndAwait(ctx context.Context, message MessageToEngine) (*MessageFromOrderbook, error) {
	retries := 3
	if isMutating(message.Type) {
		retries = 1
	}
	return rm.SendAndAwaitWithTimeout(ctx, message, 30*time.Second, retries)
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

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Subscribe before publishing, so a fast engine cannot answer into the void.
	pubsub := rm.client.Subscribe(timeoutCtx, clientID)
	defer pubsub.Close()

	if _, err := pubsub.ReceiveTimeout(timeoutCtx, 5*time.Second); err != nil {
		return nil, fmt.Errorf("failed to confirm subscription: %w", err)
	}

	msgBytes, err := json.Marshal(RedisMessage{ClientID: clientID, Message: message})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := rm.publisher.LPush(timeoutCtx, "messages", string(msgBytes)).Err(); err != nil {
		return nil, fmt.Errorf("failed to send message to Redis: %w", err)
	}

	// ReceiveMessage already honours the context deadline, so this needs no
	// goroutine and no channels. The previous version closed a response channel
	// and an error channel in the same defer, leaving the select with two ready
	// cases: half the time it read nil from the closed response channel, fell
	// through, and returned a generic error that defeated the caller's
	// don't-retry-on-cancel check.
	for {
		msg, err := pubsub.ReceiveMessage(timeoutCtx)
		if err != nil {
			return nil, fmt.Errorf("awaiting engine reply on %s: %w", clientID, err)
		}

		var response MessageFromOrderbook
		if err := json.Unmarshal([]byte(msg.Payload), &response); err != nil {
			log.Printf("Discarding unparseable engine reply on %s: %v", clientID, err)
			continue
		}
		return &response, nil
	}
}

// getRandomClientID names the pub/sub channel a single request listens on.
// It used to keep only the first UUID segment — 32 bits, which collides often
// enough at load that two concurrent requests would share a channel and each
// receive the other's reply. Nothing downstream checks that a response matches
// the command that was sent, so the full UUID is the whole defence.
func (rm *RedisManager) getRandomClientID() string {
	return uuid.New().String()
}
