# CryptoXchange 📊

A spot cryptocurrency exchange built with Go and Next.js: a real price-time
matching engine, live order book and trade streams over WebSocket, and a trading
UI you can actually place orders from.

## 📋 Table of Contents

- [Overview](#-overview)
- [Features](#-features)
- [Architecture](#%EF%B8%8F-architecture)
- [Getting Started](#-getting-started)
- [Demo Walkthrough](#-demo-walkthrough)
- [API Documentation](#-api-documentation)

## 🔍 Overview

Four services talk to each other over Redis and Postgres:

| Service | Role |
|---|---|
| `cmd/api` | REST API. Forwards order commands to the engine over a Redis list and waits for the reply on a pub/sub channel. |
| `cmd/engine` | Single-threaded matching engine. Owns the order book **and** all balances; snapshots to disk every 5s. |
| `cmd/websocket` | Fans out `depth@{market}` and `trade@{market}` streams to browsers. |
| `internal/kline` | Consumes executed trades off Redis into TimescaleDB, which rolls them into 1m/1h/1w candles. |

### 🎥 Demo Video
   https://www.loom.com/share/bbcc2ea1986a43c394a52d03af7973ef
   
## ✨ Features

- **Price-time priority matching** with fractional quantities, partial fills, and
  fills across multiple price levels
- **Limit and market orders** - market orders sweep the book and never rest
- **Real balance accounting**: funds are locked on order placement, released on
  cancel, and any surplus from filling at a better price is refunded
- **Live order book and trade tape** over WebSocket
- **TradingView-style candlestick charts** backed by TimescaleDB continuous rollups
- **Crash-safe**: the engine restores its book and balances from a snapshot on boot

## 🏗️ Architecture

![cryptoXchange Architecture](./script/cryptoXchange_Archeitecture.png)

## 🚀 Getting Started

### 1. Backend - everything in one command

```bash
docker compose up -d --build
```

That brings up TimescaleDB, Redis, the API, the matching engine and the
WebSocket server. The engine's snapshot lives in a named volume, so restarts
keep the order book and balances.

<details>
<summary>Or run the Go services from source</summary>

```bash
docker compose up -d db redis    # infrastructure only
go run ./cmd/api                 # each in its own terminal
go run ./cmd/engine
go run ./cmd/websocket
```
</details>

Configuration is optional - every value in `.env.example` has a working
localhost default.

### 2. Frontend

```bash
cd frontend && npm install && npm run dev
```

### 3. Seed a market

An empty order book is a bad first impression. This places resting depth around
200.00 and executes a few trades so the chart has history:

```bash
go run ./script/seed
```

### 4. Open the app

http://localhost:3000/trade/SOL_USD

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000/trade/SOL_USD |
| API Server | http://localhost:8080/v1 |
| WebSocket | ws://localhost:3001/v1/ws |
| Database | localhost:5432 |
| Redis | localhost:6379 |

## 🎬 Demo Walkthrough

The engine seeds demo users `1`, `2` and `5` with balances on first boot. Pick
one from the **Trading as** dropdown in the top right - no login needed.

Open two browser windows side by side:

1. **Window A, demo user 1** - place a limit **sell** of `1.5` SOL at `205`.
   It appears immediately in the ask side of both windows, and user 1's SOL
   balance moves from *available* to *locked*.
2. **Window B, demo user 2** - click the `205` level in the order book to fill
   the price, enter `0.5`, and **Buy**. The trade prints in the tape in both
   windows, the ask level shrinks to `1.00`, and both balances update.
3. **Window A** - the remaining `1.0` shows under **Open orders**. Hit **Cancel**
   and watch the locked SOL return to available.
4. **Window B** - switch the order type to **Market** and buy `2` SOL. It sweeps
   whatever the book holds and refunds the rest of the lock rather than resting.
5. Try to buy more than the account can afford - the UI shows
   *"insufficient USD: need X, have Y"* straight from the engine.

## 📚 API Documentation

Base URL: `http://localhost:8080/v1`

### Orders

`POST /order`

```json
{
  "market": "SOL_USD",
  "price": "200",
  "quantity": "0.5",
  "side": "buy",
  "type": "limit",
  "userId": "1"
}
```

Returns `201` with `{orderId, executedQty, fills}`. A rejected order returns
`400`/`404` with `{error, code}` - codes are `INSUFFICIENT_FUNDS`,
`INVALID_ORDER`, `NO_LIQUIDITY`, `NO_ORDERBOOK`.

Market orders ignore `price`; the engine reprices them through the far side of
the book and drops any unfilled remainder.

| Endpoint | Description |
|---|---|
| `DELETE /order` | Cancel an order: `{orderId, market}` |
| `GET /order/open?userId=&market=` | Open orders for a user |
| `GET /depth?market=` | Aggregated order book |
| `GET /balance/{userId}` | Available and locked balance per asset (from the engine) |
| `POST /onramp` | Credit an account: `{userId, amount}` |
| `GET /tickers` | 24h rollup per market |
| `GET /trades?market=&limit=` | Recent executed trades |
| `GET /klines/{interval}` | Candles: `1m`, `1h`, `1w` |
| `POST /authentication/user` | Signup |
| `POST /authentication/token` | Login, returns a JWT |

> The UI runs in demo mode and selects a user from a dropdown rather than logging
> in. Signup and login are implemented and work; `/users/{id}` is token-protected.

## 🧪 Tests

```bash
go test ./cmd/engine/
```

Covers the matching and balance math: fractional partial fills, fills across
multiple price levels, cancel refunds, insufficient-funds rejection, and market
order behaviour.
