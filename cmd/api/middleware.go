package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

func (app *application) AuthTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is missing"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is malformed"))
			return
		}

		token := parts[1]
		jwtToken, err := app.authenticator.ValidateToken(token)
		if err != nil {
			app.unauthorizedErrorResponse(w, r, err)
			return
		}

		claims, _ := jwtToken.Claims.(jwt.MapClaims)
		userid := fmt.Sprintf("%.f", claims["sub"])
		// if err != nil {
		// 	app.unauthorizedErrorResponse(w, r, err)
		// 	return
		// }

		ctx := r.Context()
		user, err := app.getUser(ctx, userid)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		ctx = context.WithValue(ctx, userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (app *application) getUser(ctx context.Context, userID string) (*store.User, error) {
	user, err := app.store.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return user, nil
}
