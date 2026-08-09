package main

import (
	"context"
	"expvar"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/auth"
	"github.com/Althaf66/cryptoXchange/internal/dbase"
	"github.com/Althaf66/cryptoXchange/internal/kline"
	"github.com/Althaf66/cryptoXchange/internal/store"
	// "github.com/joho/godotenv"
	"go.uber.org/zap"
)

const version = "1.0.0"

func main() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }

	env := getEnvOrDefault("ENV", "development")
	requireConfig(env, map[string]string{
		"DB_ADDR":      os.Getenv("DB_ADDR"),
		"REDIS_ADDR":   os.Getenv("REDIS_ADDR"),
		"JWT_SECRET":   os.Getenv("JWT_SECRET"),
		"FRONTEND_URL": os.Getenv("FRONTEND_URL"),
	})

	cfg := config{
		addr:        getEnvOrDefault("PORT", ":8080"),
		apiUrl:      getEnvOrDefault("API_URL", "http://localhost:8080"),
		frontendURL: normalizeOrigin(getEnvOrDefault("FRONTEND_URL", "http://localhost:3000")),
		env:         env,
		db: dbConfig{
			addr:     getEnvOrDefault("DB_ADDR", "postgres://admin:adminpassword@localhost/cryptoXchange?sslmode=disable"),
			RedisURL: getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		},

		auth: authConfig{
			token: tokenConfig{
				secret: getEnvOrDefault("JWT_SECRET", "unknown"),
				exp:    time.Hour * 24 * 3,
				iss:    "cryptoXchange",
			},
		},
	}

	// logger
	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	// Logged because a CORS origin mismatch is otherwise invisible from the
	// server side: the request is answered 200 and the browser drops it.
	logger.Infow("cors allowed origin", "frontendURL", cfg.frontendURL)

	//database
	db, err := dbase.New(cfg.db.addr, 30, 30, "15m")
	if err != nil {
		logger.Fatal(err)
	}
	defer db.Close()
	logger.Info("postgres connection pool established")

	store := store.NewPostgresStorage(db)
	redisManager = NewRedisManager()
	// This used to re-check the database's err, which was already known nil, so
	// a bad REDIS_ADDR went unnoticed until the first request timed out.
	if err := redisManager.client.Ping(context.Background()).Err(); err != nil {
		logger.Fatalf("failed to connect to Redis: %v", err)
	}
	defer redisManager.client.Close()
	defer redisManager.publisher.Close()
	logger.Info("redis connection pool established")

	if err := dbase.InitializeKlineDB(db); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	if err := dbase.InitializeExchangeTables(db); err != nil {
		log.Fatal("Failed to initialize exchange tables:", err)
	}

	go kline.StartDataProcessor(db)
	go startCronJob(db)

	jwtAuthenticator := auth.NewJWTAuthenticator(cfg.auth.token.secret, cfg.auth.token.iss, cfg.auth.token.iss)

	app := application{
		config:        cfg,
		store:         store,
		logger:        logger,
		authenticator: jwtAuthenticator,
	}

	expvar.NewString("version").Set(version)
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	mux := app.mount()
	log.Fatal(app.run(mux))
}

// normalizeOrigin makes FRONTEND_URL match the Origin header a browser actually
// sends, which is scheme + host with no trailing slash. It is the single allowed
// CORS origin and compared byte for byte, so "host.example" or a trailing "/"
// silently blocks every browser call while the server still logs 200s.
// Zerops' ${..._zeropsSubdomain} supplies a bare hostname, hence the prefix.
func normalizeOrigin(raw string) string {
	origin := strings.TrimRight(strings.TrimSpace(raw), "/")
	if origin == "" {
		return origin
	}
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		origin = "https://" + origin
	}
	return origin
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// requireConfig fails the boot rather than letting a deployment silently fall
// back to a localhost default. A mistyped env var name otherwise produces an
// app that starts cleanly and then cannot reach its own database, or signs
// tokens with the literal string "unknown".
func requireConfig(env string, values map[string]string) {
	if env == "development" {
		return
	}
	missing := []string{}
	for key, value := range values {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		log.Fatalf("ENV=%s but these are unset: %s", env, strings.Join(missing, ", "))
	}
}
