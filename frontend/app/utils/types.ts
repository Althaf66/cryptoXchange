
/** One candle. `start`/`end` are RFC3339 strings; prices are numbers. */
export interface KLine {
    start: string;
    end: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
}

export interface Trade {
    "id": number,
    "isBuyerMaker": boolean,
    "price": string,
    "quantity": string,
    "quoteQuantity": string,
    "timestamp": number
}

export interface Depth {
    bids: [string, string][],
    asks: [string, string][],
    lastUpdateId: string
}

export interface UserBalance {
    available: number;
    locked: number;
}

/** asset symbol -> balance, as returned by the engine via GET /balance/{userId} */
export type Balances = Record<string, UserBalance>;

/** A demo account: the engine user id, plus the name to show for it. */
export interface VirtualUser {
    id: string;
    name: string;
}

export interface OpenOrder {
    orderId: string;
    price: number;
    quantity: number;
    filled: number;
    side: "buy" | "sell";
    userId: string;
}

export interface Fill {
    price: string;
    qty: number;
    tradeId: number;
    otherUserId: string;
    markerOrderId: string;
}

export interface OrderResult {
    orderId: string;
    executedQty: number;
    fills: Fill[] | null;
}

export interface Ticker {
    "firstPrice": string,
    "high": string,
    "lastPrice": string,
    "price": string,
    "low": string,
    "priceChange": string,
    "priceChangePercent": string,
    "quoteVolume": string,
    "symbol": string,
    "trades": string,
    "volume": string
}