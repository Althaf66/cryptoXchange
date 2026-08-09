// Package rediscfg builds Redis client options from REDIS_ADDR so every
// service connects the same way.
//
// REDIS_ADDR accepts two shapes:
//   - "host:port" — local docker compose, no auth.
//   - "redis://user:password@host:port" — Zerops Valkey, which mandates
//     authentication and hands out a ready-made connection string. A bare
//     host:port against it fails with "NOAUTH Authentication required".
package rediscfg

import (
	"log"
	"os"
	"strings"

	"github.com/go-redis/redis/v8"
)

// Options returns a fresh *redis.Options per call — go-redis takes ownership of
// the struct it is given, so two clients must never share one.
func Options() *redis.Options {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	if !strings.Contains(addr, "://") {
		return &redis.Options{Addr: addr}
	}

	opt, err := redis.ParseURL(addr)
	if err != nil {
		// Fatal rather than falling back: treating a malformed URL as a
		// hostname produces a connection timeout far from the real cause.
		log.Fatalf("REDIS_ADDR is not a valid redis URL: %v", err)
	}
	return opt
}
