package ws

import (
	"log"
	"net/http"
	"strings"
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
	user := NewUser(id, conn)

	um.mutex.Lock()
	um.users[id] = user
	um.mutex.Unlock()

	um.registerOnClose(conn, id)
	return user
}

func (um *UserManagerStruct) registerOnClose(conn *websocket.Conn, id string) {
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				um.mutex.Lock()
				delete(um.users, id)
				um.mutex.Unlock()
				SubscriptionManager.GetInstance().UserLeft(id)
				break
			}
		}
	}()
}

func (um *UserManagerStruct) GetUser(id string) *User {
	um.mutex.RLock()
	defer um.mutex.RUnlock()
	return um.users[id]
}

func (um *UserManagerStruct) getRandomID() string {
	fullUUID := uuid.New().String()
	return strings.Split(fullUUID, "-")[0]
}

func validateJWT(tokenString string) (string, error) {
	// In production, use a secure, environment-variable-stored secret
	const secretKey = "unknown"

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
