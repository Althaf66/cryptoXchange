package ws

import (
	"errors"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type UserManagerStruct struct {
	users map[string]*User
	mutex sync.RWMutex
}

var UserManager = &UserManagerStruct{
	users: make(map[string]*User),
}

func (um *UserManagerStruct) AddUser(conn *websocket.Conn, r *http.Request) *User {
	var id string
	var err error

	tokenString := r.URL.Query().Get("token")
	if tokenString != "" {
		id, err = validateJWT(tokenString)
		if err != nil {
			log.Println("Invalid token:", err)
			conn.Close()
			return nil
		}
	} else {
		id = um.getRandomID()
	}
	// Cleanup is driven by the user's own read loop. A second goroutine reading
	// the same connection races it for every frame, which tore the socket down
	// and swallowed SUBSCRIBE messages.
	user := NewUser(id, conn, func() { um.removeUser(id) })

	um.mutex.Lock()
	um.users[id] = user
	um.mutex.Unlock()

	// Only now is it safe to read: onClose can fire on the very first frame.
	user.Start()

	return user
}

func (um *UserManagerStruct) removeUser(id string) {
	um.mutex.Lock()
	delete(um.users, id)
	um.mutex.Unlock()
	SubscriptionManager.GetInstance().UserLeft(id)
	log.Println("User disconnected:", id)
}

func (um *UserManagerStruct) GetUser(id string) *User {
	um.mutex.RLock()
	defer um.mutex.RUnlock()
	return um.users[id]
}

// getRandomID keys an anonymous connection in the user map. Keeping only the
// first UUID segment left 32 bits, and a collision makes two sessions share a
// map entry — whichever disconnects first tears down the survivor's streams.
func (um *UserManagerStruct) getRandomID() string {
	return uuid.New().String()
}

func validateJWT(tokenString string) (string, error) {
	// Must match the API's JWT_SECRET or every token it issues is rejected here.
	// This was a hardcoded "unknown", which only went unnoticed because the demo
	// UI connects without a token at all.
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		return "", errors.New("JWT_SECRET is not set on the websocket service")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", jwt.ErrTokenInvalidClaims
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", jwt.ErrTokenInvalidClaims
	}

	userID, ok := claims["sub"].(string)
	if !ok || userID == "" {
		return "", jwt.ErrTokenInvalidClaims
	}

	return userID, nil
}
