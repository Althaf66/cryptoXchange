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
		handlers.AllowedOrigins([]string{"http://localhost:3000"}), // Allow your frontend origin
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type"}),
	)
	// t := c.Handler(r)
	c := corsOptions(r)

	v1 := r.PathPrefix("/v1").Subrouter()
	v1.HandleFunc("/authentication/user", app.registerUserHandler).Methods("POST")
	v1.HandleFunc("/authentication/token", app.createTokenHandler).Methods("POST")

	v1.HandleFunc("/order", app.createOrderHandler).Methods("POST")
	v1.HandleFunc("/order", app.cancelOrderHandler).Methods("DELETE")
	v1.HandleFunc("/order/open", app.getOpenOrdersHandler).Methods("GET")
	v1.HandleFunc("/depth", app.getDepthHandler).Methods("GET")
	v1.HandleFunc("/onramp", app.onRampHandler).Methods("POST")
	v1.HandleFunc("/klines/{interval}", app.klinesHandler).Methods("GET")
	v1.HandleFunc("/latestprice", app.latestPriceHandler).Methods("GET")
	v1.HandleFunc("/trades", app.recentTradesHandler).Methods("GET")
	v1.HandleFunc("/trades/{market}", app.marketTradesHandler).Methods("GET")

	balanceSubrouter := v1.PathPrefix("/balance/{userId}").Subrouter()
	balanceSubrouter.Use(app.AuthTokenMiddleware)
	balanceSubrouter.HandleFunc("/", app.balanceHandler).Methods("GET")

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
