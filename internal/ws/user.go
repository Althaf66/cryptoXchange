package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type User struct {
	ID            string
	Conn          *websocket.Conn
	subscriptions []string
	mutex         sync.RWMutex
	// onClose runs when the read loop ends. gorilla/websocket allows only one
	// concurrent reader, so cleanup has to hang off this loop rather than a
	// second goroutine of its own.
	onClose func()
}

func NewUser(id string, conn *websocket.Conn, onClose func()) *User {
	user := &User{
		ID:            id,
		Conn:          conn,
		subscriptions: make([]string, 0),
		onClose:       onClose,
	}
	// Automatically subscribe user to their own trades channel
	SubscriptionManager.GetInstance().Subscribe(user.ID, "trades:"+user.ID)
	log.Println("New user connected:", user.ID)
	return user
}

// Start begins reading. It is deliberately separate from NewUser: the read loop
// runs onClose on disconnect, and if that fires before UserManager has recorded
// the user, the cleanup deletes nothing and the entry is then inserted dead —
// leaking the connection and its subscriptions permanently.
func (u *User) Start() {
	u.addListeners()
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

// Emit forwards a payload to the browser byte for byte. The engine owns the
// stream schema; re-serializing it here through a local struct silently dropped
// every field the two definitions did not share.
func (u *User) Emit(data []byte) error {
	// Every subscriber's write happens inline on the single Redis dispatch
	// goroutine, so an unbounded write blocks market data for everyone. The
	// deadline caps that at 10s and drops the stalled client instead.
	// ponytail: a per-user send channel and writer goroutine is the real fix;
	// add it when a slow client is actually observed stalling the fan-out.
	if err := u.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return u.Conn.WriteMessage(websocket.TextMessage, data)
}

// Keepalive timings. A proxy in front of this service will drop an idle
// connection, and a half-open TCP connection never produces a read error — so
// without a deadline the read loop parks forever, onClose never fires, and the
// user and its subscriptions leak while the broadcast loop keeps writing to a
// socket nobody is reading.
const (
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
)

func (u *User) addListeners() {
	// WriteControl is safe to call concurrently with WriteMessage, so the ping
	// ticker needs no lock against the broadcast goroutine's Emit.
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for range ticker.C {
			deadline := time.Now().Add(10 * time.Second)
			if err := u.Conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				return
			}
		}
	}()

	go func() {
		defer func() {
			u.Conn.Close()
			if u.onClose != nil {
				u.onClose()
			}
		}()

		u.Conn.SetReadDeadline(time.Now().Add(pongWait))
		u.Conn.SetPongHandler(func(string) error {
			return u.Conn.SetReadDeadline(time.Now().Add(pongWait))
		})

		for {
			_, messageBytes, err := u.Conn.ReadMessage()
			if err != nil {
				log.Printf("User %s disconnected: %v", u.ID, err)
				break
			}
			// Any traffic proves the peer is alive, not just pongs.
			u.Conn.SetReadDeadline(time.Now().Add(pongWait))
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
