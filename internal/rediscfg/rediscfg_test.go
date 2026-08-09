package rediscfg

import "testing"

func TestOptions(t *testing.T) {
	// Bare host:port — local docker compose, no auth.
	t.Setenv("REDIS_ADDR", "redis:6379")
	if got := Options(); got.Addr != "redis:6379" || got.Password != "" {
		t.Errorf("bare addr: got addr=%q password=%q", got.Addr, got.Password)
	}

	// Zerops Valkey connection string — the password must be picked up, or
	// every command fails with "NOAUTH Authentication required".
	t.Setenv("REDIS_ADDR", "redis://default:s3cr3t@redis.zerops:6379")
	got := Options()
	if got.Addr != "redis.zerops:6379" {
		t.Errorf("url addr = %q, want redis.zerops:6379", got.Addr)
	}
	if got.Password != "s3cr3t" {
		t.Errorf("url password = %q, want s3cr3t", got.Password)
	}
	if got.Username != "default" {
		t.Errorf("url username = %q, want default", got.Username)
	}

	// Unset falls back to localhost for local runs.
	t.Setenv("REDIS_ADDR", "")
	if got := Options(); got.Addr != "localhost:6379" {
		t.Errorf("empty addr = %q, want localhost:6379", got.Addr)
	}

	// Two clients must never share one Options value.
	t.Setenv("REDIS_ADDR", "redis:6379")
	first, second := Options(), Options()
	if first == second {
		t.Error("Options returned the same pointer twice")
	}
}
