package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Althaf66/cryptoXchange/internal/auth"
	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type application struct {
	config        config
	store         store.Storage
	logger        *zap.SugaredLogger
	authenticator auth.Authenticator
}

type config struct {
	addr        string
	env         string
	frontendURL string
	apiUrl      string
	db          dbConfig
	auth        authConfig
}

type dbConfig struct {
	addr     string
	RedisURL string
}

type authConfig struct {
	token tokenConfig
}

type tokenConfig struct {
	secret string
	exp    time.Duration
	iss    string
}

func (app *application) mount() http.Handler {
	r := mux.NewRouter()
	corsOptions := handlers.CORS(
		handlers.AllowedOrigins([]string{app.config.frontendURL}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)
	// t := c.Handler(r)
	c := corsOptions(r)

	v1 := r.PathPrefix("/v1").Subrouter()
	// Liveness probe for the platform. Pointed at a 404 the container is
	// restarted forever, which takes the engine's state with it.
	v1.HandleFunc("/health", app.healthcheckHandler).Methods("GET")
	v1.HandleFunc("/authentication/user", app.registerUserHandler).Methods("POST")
	v1.HandleFunc("/authentication/token", app.createTokenHandler).Methods("POST")

	v1.HandleFunc("/order", app.createOrderHandler).Methods("POST")
	v1.HandleFunc("/order", app.cancelOrderHandler).Methods("DELETE")
	v1.HandleFunc("/order/open", app.getOpenOrdersHandler).Methods("GET")
	v1.HandleFunc("/order/history", app.orderHistoryHandler).Methods("GET")
	v1.HandleFunc("/depth", app.getDepthHandler).Methods("GET")
	v1.HandleFunc("/onramp", app.onRampHandler).Methods("POST")
	v1.HandleFunc("/klines/{interval}", app.klinesHandler).Methods("GET")
	v1.HandleFunc("/latestprice", app.latestPriceHandler).Methods("GET")
	v1.HandleFunc("/tickers", app.tickersHandler).Methods("GET")
	v1.HandleFunc("/trades", app.recentTradesHandler).Methods("GET")
	v1.HandleFunc("/trades/{market}", app.marketTradesHandler).Methods("GET")

	// Demo mode: balances are readable without a token so the UI can show them
	// for the selected demo user. Signup/login still work and /users stays authed.
	// Order and deposit history follow the same rule for the same reason.
	v1.HandleFunc("/balance/{userId}", app.balanceHandler).Methods("GET")
	v1.HandleFunc("/transfers", app.transferHistoryHandler).Methods("GET")

	// Operational check, not a user-facing route: compares the ledger against
	// what the engine holds in memory.
	v1.HandleFunc("/admin/reconcile", app.reconcileHandler).Methods("GET")

	// Demo accounts, created and funded from the home page. Registered before
	// the /users/{userID} subrouter below so "virtual" is never taken for a id.
	v1.HandleFunc("/users/virtual", app.getVirtualUsersHandler).Methods("GET")
	v1.HandleFunc("/users/virtual", app.createVirtualUserHandler).Methods("POST")

	userSubrouter := v1.PathPrefix("/users/{userID}").Subrouter()
	userSubrouter.Use(app.AuthTokenMiddleware)
	userSubrouter.HandleFunc("/", app.getUserHandler).Methods("GET")

	return c
}

func (app *application) run(mux http.Handler) error {
	srv := &http.Server{
		Addr:         app.config.addr,
		Handler:      mux,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}

	shutdown := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		app.logger.Infow("signal caught", "signal", s.String())
		shutdown <- srv.Shutdown(ctx)
	}()

	app.logger.Info("server has started ", "addr", app.config.addr, " env:", app.config.env)

	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdown
	if err != nil {
		return err
	}

	app.logger.Infow("server has stopped", "addr", app.config.addr, "env", app.config.env)

	return nil
}
