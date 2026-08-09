package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserStore struct {
	db *sql.DB
}

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrDuplicateEmail    = errors.New("a user with that email already exists")
	ErrDuplicateUsername = errors.New("a user with that username already exists")
	QueryTimeOutDuration = 5 * time.Second
)

type User struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Password  password `json:"-"`
	CreatedAt string   `json:"created_at"`
}

type password struct {
	text *string
	hash []byte
}

func (s *UserStore) Create(ctx context.Context, tx *sql.Tx, user *User) error {
	query := `INSERT INTO users (username, password, email) VALUES ($1,$2,$3) RETURNING id, created_at`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeOutDuration)
	defer cancel()

	err := tx.QueryRowContext(ctx, query, user.Username, user.Password.hash, user.Email).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return err
	}

	return nil
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT id, username, email, password, created_at
	FROM users WHERE id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeOutDuration)
	defer cancel()

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Username, &user.Email,
		&user.Password.hash, &user.CreatedAt)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return nil, ErrUserNotFound
		default:
			return nil, err
		}
	}
	return user, nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id,username,email,password,created_at FROM users 
	WHERE email = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeOutDuration)
	defer cancel()

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, email).Scan(&user.ID, &user.Username, &user.Email,
		&user.Password.hash, &user.CreatedAt)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return nil, ErrUserNotFound
		default:
			return nil, err
		}
	}

	return user, nil
}

func (p *password) Set(text string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(text), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	p.text = &text
	p.hash = hash

	return nil
}

func (p *password) Compare(text string) error {
	return bcrypt.CompareHashAndPassword(p.hash, []byte(text))
}

func (s *UserStore) CreateAndInvite(ctx context.Context, user *User, token string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		if err := s.Create(ctx, tx, user); err != nil {
			return err
		}
		return nil
	})
}

func (s *UserStore) Delete(ctx context.Context, userid string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		err := s.delete(ctx, tx, userid)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *UserStore) delete(ctx context.Context, tx *sql.Tx, userid string) error {
	query := `DELETE FROM users WHERE id=$1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeOutDuration)
	defer cancel()

	_, err := tx.ExecContext(ctx, query, userid)
	if err != nil {
		return err
	}
	return nil
}
