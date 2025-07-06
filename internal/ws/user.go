package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type User struct {
	ID            string
	Conn          *websocket.Conn
	subscriptions []string
	mutex         sync.RWMutex
}

func NewUser(id string, conn *websocket.Conn) *User {
	user := &User{
		ID:            id,
		Conn:          conn,
		subscriptions: make([]string, 0),
	}
	user.addListeners()
	// Automatically subscribe user to their own trades channel
	SubscriptionManager.GetInstance().Subscribe(user.ID, "trades:"+user.ID)
	log.Println("New user connected:", user.ID)
	return user
}

func (u *User) Subscribe(subscription string) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	u.subscriptions = append(u.subscriptions, subscription)
}

func (u *User) Unsubscribe(subscription string) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	for i, s := range u.subscriptions {
		if s == subscription {
			u.subscriptions = append(u.subscriptions[:i], u.subscriptions[i+1:]...)
			break
		}
	}
}

func (u *User) Emit(message OutgoingMessage) error {
	data, err := json.Marshal(message)
	log.Printf("Emitting message to user %s: %s", u.ID, string(data))
	if err != nil {
		return err
	}
	return u.Conn.WriteMessage(websocket.TextMessage, data)
}

func (u *User) addListeners() {
	go func() {
		for {
			_, messageBytes, err := u.Conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading message: %v", err)
				break
			}
			log.Printf("Received message from user %s: %s", u.ID, string(messageBytes))

			var parsedMessage IncomingMessage
			if err := json.Unmarshal(messageBytes, &parsedMessage); err != nil {
				log.Printf("Error parsing message: %v", err)
				continue
			}

			switch parsedMessage.Method {
			case SUBSCRIBE:
				for _, s := range parsedMessage.Params {
					SubscriptionManager.GetInstance().Subscribe(u.ID, s)
				}
			case UNSUBSCRIBE:
				for _, s := range parsedMessage.Params {
					SubscriptionManager.GetInstance().Unsubscribe(u.ID, s)
				}
			}
			log.Printf("User %s sent message: %s", u.ID, parsedMessage.Method)
		}
	}()
}
