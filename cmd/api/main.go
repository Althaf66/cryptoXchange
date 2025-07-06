package main

import (
	"expvar"
	"log"
	"runtime"
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

	cfg := config{
		addr:        ":8080",
		apiUrl:      "http://localhost:8080",
		frontendURL: "http://localhost:3000",
		env:         "development",
		db: dbConfig{
			addr:     "postgres://admin:adminpassword@localhost/cryptoXchange?sslmode=disable",
			RedisURL: "redis://localhost:6379",
		},

		auth: authConfig{
			token: tokenConfig{
				secret: "unknown",
				exp:    time.Hour * 24 * 3,
				iss:    "cryptoXchange",
			},
		},
	}

	// logger
	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	//database
	db, err := dbase.New(cfg.db.addr, 30, 30, "15m")
	if err != nil {
		logger.Fatal(err)
	}
	defer db.Close()
	logger.Info("postgres connection pool established")

	store := store.NewPostgresStorage(db)
	redisManager = NewRedisManager()
	if err != nil {
		logger.Info("Failed to connect to Redis:", err)
	}
	defer redisManager.client.Close()
	logger.Info("redis connection pool established")

	if err := dbase.InitializeKlineDB(db); err != nil {
		log.Fatal("Failed to initialize database:", err)
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

// func getEnvOrDefault(key, defaultValue string) string {
// 	if value := os.Getenv(key); value != "" {
// 		return value
// 	}
// 	return defaultValue
// }
