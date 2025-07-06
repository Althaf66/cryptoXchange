package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"

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
		rdb := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})

		redisInstance = &RedisManager{
			client: rdb,
			ctx:    context.Background(),
		}

		redisInstance.client.Ping(redisInstance.ctx)
	})
	return redisInstance
}

func (r *RedisManager) PushMessage(message DbMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return r.client.LPush(r.ctx, "db_processor", string(data)).Err()
}

func (r *RedisManager) PublishMessage(channel string, message WsMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	log.Printf("Publishing message to channel %s: %s", channel, string(data))

	return r.client.Publish(r.ctx, channel, string(data)).Err()
}

func (r *RedisManager) SendToAPI(clientID string, message MessageToAPI) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	log.Printf("Sending message to API for client %s: %s", clientID, string(data))

	return r.client.Publish(r.ctx, clientID, string(data)).Err()
}
