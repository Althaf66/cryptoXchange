package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/Althaf66/cryptoXchange/internal/rediscfg"
	"github.com/go-redis/redis/v8"
)

type RedisManager struct {
	client *redis.Client
	ctx    context.Context
}

var (
	redisInstance *RedisManager
	redisOnce     sync.Once
)

func GetRedisInstance() *RedisManager {
	redisOnce.Do(func() {
		rdb := redis.NewClient(rediscfg.Options())

		redisInstance = &RedisManager{
			client: rdb,
			ctx:    context.Background(),
		}

		redisInstance.client.Ping(redisInstance.ctx)
	})
	return redisInstance
}

// pushDbMessage is the single exit point for everything the engine wants
// persisted. It is a variable so tests can capture messages without a Redis or
// a database; a one-implementation interface would not earn its keep here.
var pushDbMessage = func(message DbMessage) error {
	return GetRedisInstance().PushMessage(message)
}

// sendToAPI is the single exit point for every reply to a waiting API request.
// Same reasoning as pushDbMessage: a var lets tests assert that a handler
// answered at all, which is the property that matters — the API blocks on a
// pub/sub reply, so a handler that returns without calling this costs the
// caller its full timeout.
var sendToAPI = func(clientID string, message MessageToAPI) error {
	return GetRedisInstance().SendToAPI(clientID, message)
}

// Push to the database from engine
func (r *RedisManager) PushMessage(message DbMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return r.client.LPush(r.ctx, "db_processor", string(data)).Err()
}

// Publish a message to websocket from the engine
func (r *RedisManager) PublishMessage(channel string, message WsMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	log.Printf("Publishing message to channel %s: %s", channel, string(data))

	return r.client.Publish(r.ctx, channel, string(data)).Err()
}

// Publish message to API from the engine
func (r *RedisManager) SendToAPI(clientID string, message MessageToAPI) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	log.Printf("Sending message to API for client %s: %s", clientID, string(data))

	return r.client.Publish(r.ctx, clientID, string(data)).Err()
}
