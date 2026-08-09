-- Users Table
-- Stores user information with a unique email and username
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email varchar(255) UNIQUE NOT NULL,
    username VARCHAR(255) UNIQUE NOT NULL,
    password BYTEA NOT NULL,
    created_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Balances Table
-- Tracks user asset balances, linked to users table via userId
CREATE TABLE IF NOT EXISTS balances (
    userId BIGSERIAL,
    asset VARCHAR(50) NOT NULL,
    amount DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (userId, asset),
    FOREIGN KEY (userId) REFERENCES users(id) ON DELETE CASCADE
);





