package main

import (
	"encoding/json"
	"log"
	"sort"
	"strconv"
	"strings"
)

// Virtual users are the demo accounts the UI trades from. The engine has always
// materialized a balance map for any id it is handed, so an id is all trading
// needs; this file adds the name and a way to mint and fund one from the UI.

func (e *Engine) handleCreateUser(message MessageFromAPI, clientID string) {
	dataBytes, _ := json.Marshal(message.Data)
	var data CreateUserData
	json.Unmarshal(dataBytes, &data)

	name := strings.TrimSpace(data.Name)
	if name == "" {
		sendRejection(clientID, &OrderError{Code: "INVALID_USER", Reason: "name is required"})
		return
	}

	// An empty amount is the documented way to create an unfunded account.
	amount := 0.0
	if data.Amount != "" {
		parsed, err := strconv.ParseFloat(data.Amount, 64)
		if err != nil || parsed < 0 {
			sendRejection(clientID, &OrderError{Code: "INVALID_USER", Reason: "amount must be a non-negative number"})
			return
		}
		amount = parsed
	}

	userID := e.nextUserID()
	e.Users[userID] = name
	if e.Balances[userID] == nil {
		e.Balances[userID] = make(map[string]*UserBalance)
	}
	e.creditAllAssets(userID, amount, data.TxnID)
	log.Printf("created virtual user %s (%s) with %.2f across %d assets", userID, name, amount, len(allAssets()))

	sendToAPI(clientID, MessageToAPI{
		Type:    CREATE_USER,
		Payload: VirtualUser{ID: userID, Name: name},
	})
}

func (e *Engine) handleGetUsers(clientID string) {
	users := make([]VirtualUser, 0, len(e.Users))
	for id, name := range e.Users {
		users = append(users, VirtualUser{ID: id, Name: name})
	}
	// Numeric where possible, so "10" sorts after "5" rather than before it.
	sort.Slice(users, func(i, j int) bool {
		a, errA := strconv.Atoi(users[i].ID)
		b, errB := strconv.Atoi(users[j].ID)
		if errA == nil && errB == nil {
			return a < b
		}
		return users[i].ID < users[j].ID
	})

	sendToAPI(clientID, MessageToAPI{
		Type:    GET_USERS,
		Payload: users,
	})
}

// creditAllAssets adds amount to every asset in allAssets().
func (e *Engine) creditAllAssets(userID string, amount float64, refID string) {
	if amount <= 0 {
		return
	}
	if e.Balances[userID] == nil {
		e.Balances[userID] = make(map[string]*UserBalance)
	}
	for _, asset := range allAssets() {
		if e.Balances[userID][asset] == nil {
			e.Balances[userID][asset] = &UserBalance{}
		}
		e.Balances[userID][asset].Available += amount
		// LEDGER_DEPOSIT, not LEDGER_SEED: seed rows are deduped on ref_id by a
		// unique index, and this is a real grant rather than a boot-time top-up.
		e.pushLedger(userID, asset, amount, LEDGER_DEPOSIT, refID+":"+asset)
	}
}

// nextUserID mints the next free numeric id, keeping new accounts in the same
// "1"/"2"/"5" family the seed script trades from.
// Balances is checked too: an id can have a balance without ever having had a
// name (the seed script's accounts predate this file).
func (e *Engine) nextUserID() string {
	highest := 0
	bump := func(id string) {
		if n, err := strconv.Atoi(id); err == nil && n > highest {
			highest = n
		}
	}
	for id := range e.Users {
		bump(id)
	}
	for id := range e.Balances {
		bump(id)
	}
	return strconv.Itoa(highest + 1)
}
