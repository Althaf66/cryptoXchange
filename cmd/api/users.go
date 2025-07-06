package main

import (
	"context"
	"net/http"

	"github.com/Althaf66/cryptoXchange/internal/store"
	"github.com/gorilla/mux"
)

type userKey string

const userCtx userKey = "user"

// @Summary		Fetches a user profile
// @Router			/users/{id} [get]
func (app *application) getUserHandler(w http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["userID"]

	user, err := app.getUser(r.Context(), userID)
	if err != nil {
		switch err {
		case store.ErrUserNotFound:
			app.notFoundResponse(w, r, err)
			return
		default:
			app.internalServerError(w, r, err)
			return
		}
	}

	if err := JsonResponse(w, http.StatusOK, user); err != nil {
		app.internalServerError(w, r, err)
	}
}

func (app *application) userContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["userID"]
		ctx := r.Context()
		user, err := app.store.Users.GetByID(ctx, id)
		if err != nil {
			switch err {
			case store.ErrUserNotFound:
				app.notFoundResponse(w, r, err)
				return
			default:
				app.internalServerError(w, r, err)
				return
			}
		}
		ctx = context.WithValue(ctx, userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserfromCtx(r *http.Request) *store.User {
	user, _ := r.Context().Value(userCtx).(*store.User)
	return user
}
