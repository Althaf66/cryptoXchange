package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/go-redis/redis/v8"
)

type SubscriptionManagerStruct struct {
	subscriptions        map[string][]string
	reverseSubscriptions map[string][]string
	redisClient          *redis.Client
	pubsub               *redis.PubSub
	mutex                sync.RWMutex
	subscribedChannels   map[string]bool
}

var SubscriptionManager = &SubscriptionManagerStruct{
	subscriptions:        make(map[string][]string),
	reverseSubscriptions: make(map[string][]string),
	subscribedChannels:   make(map[string]bool),
}

func (sm *SubscriptionManagerStruct) GetInstance() *SubscriptionManagerStruct {
	return sm
}

func (sm *SubscriptionManagerStruct) Init() {
	sm.redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	sm.pubsub = sm.redisClient.Subscribe(context.Background())
	go sm.listenToRedis()
}

func (sm *SubscriptionManagerStruct) Subscribe(userID, subscription string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Check if user already subscribed
	userSubs := sm.subscriptions[userID]
	log.Printf("User %s current subscriptions: %v", userID, userSubs)
	for _, s := range userSubs {
		if s == subscription {
			return
		}
	}

	// Add to subscriptions
	sm.subscriptions[userID] = append(sm.subscriptions[userID], subscription)
	sm.reverseSubscriptions[subscription] = append(sm.reverseSubscriptions[subscription], userID)

	// Subscribe to Redis channel if this is the first subscription
	if len(sm.reverseSubscriptions[subscription]) == 1 && !sm.subscribedChannels[subscription] {
		sm.pubsub.Subscribe(context.Background(), subscription)
		sm.subscribedChannels[subscription] = true
	}
	log.Println("User", userID, "subscribed to", subscription)
}

func (sm *SubscriptionManagerStruct) Unsubscribe(userID, subscription string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Remove from user subscriptions
	if userSubs, exists := sm.subscriptions[userID]; exists {
		for i, s := range userSubs {
			if s == subscription {
				sm.subscriptions[userID] = append(userSubs[:i], userSubs[i+1:]...)
				break
			}
		}
	}

	// Remove from reverse subscriptions
	if reverseSubs, exists := sm.reverseSubscriptions[subscription]; exists {
		for i, s := range reverseSubs {
			if s == userID {
				sm.reverseSubscriptions[subscription] = append(reverseSubs[:i], reverseSubs[i+1:]...)
				break
			}
		}

		// Unsubscribe from Redis if no more users
		if len(sm.reverseSubscriptions[subscription]) == 0 {
			delete(sm.reverseSubscriptions, subscription)
			sm.pubsub.Unsubscribe(context.Background(), subscription)
			delete(sm.subscribedChannels, subscription)
		}
	}
	log.Println("User", userID, "unsubscribed from", subscription)
}

func (sm *SubscriptionManagerStruct) UserLeft(userID string) {
	log.Printf("user left %s", userID)
	sm.mutex.RLock()
	userSubs := make([]string, len(sm.subscriptions[userID]))
	copy(userSubs, sm.subscriptions[userID])
	sm.mutex.RUnlock()

	for _, s := range userSubs {
		sm.Unsubscribe(userID, s)
	}
}

func (sm *SubscriptionManagerStruct) GetSubscriptions(userID string) []string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	if subs, exists := sm.subscriptions[userID]; exists {
		result := make([]string, len(subs))
		copy(result, subs)
		log.Printf("User %s subscriptions: %v", userID, result)
		return result
	}
	log.Printf("No subscriptions found for user %s", userID)
	return []string{}
}

func (sm *SubscriptionManagerStruct) listenToRedis() {
	ch := sm.pubsub.Channel()
	for msg := range ch {
		log.Printf("Received message on channel %s: %s", msg.Channel, msg.Payload)
		sm.redisCallbackHandler(msg.Payload, msg.Channel)
	}
	log.Println("Redis pubsub channel closed, stopping listener")
}

func (sm *SubscriptionManagerStruct) redisCallbackHandler(message, channel string) {
	var parsedMessage OutgoingMessage
	if err := json.Unmarshal([]byte(message), &parsedMessage); err != nil {
		log.Printf("Error parsing Redis message: %v", err)
		return
	}

	sm.mutex.RLock()
	userIDs := make([]string, len(sm.reverseSubscriptions[channel]))
	copy(userIDs, sm.reverseSubscriptions[channel])
	sm.mutex.RUnlock()
	log.Printf("Emitting message to %d users on channel %s", len(userIDs), channel)
	for _, userID := range userIDs {
		if user := UserManager.GetUser(userID); user != nil {
			log.Printf("Emitting message to user %s on channel %s: %v", userID, channel, parsedMessage)
			user.Emit(parsedMessage)
		}
	}
}
