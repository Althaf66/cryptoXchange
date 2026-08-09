import axios from "axios";
import { Balances, Depth, KLine, OpenOrder, OrderResult, Ticker, Trade, VirtualUser } from "./types";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/v1";

export async function getTickers(): Promise<Ticker[]> {
    const response = await axios.get(`${BASE_URL}/tickers`);
    return response.data ?? [];
}

export async function getTicker(market: string): Promise<Ticker | null> {
    const tickers = await getTickers();
    // No fallback to tickers[0]: with several markets that renders another
    // market's price under this market's name.
    return tickers.find((t) => t.symbol === market) ?? null;
}

export async function getDepth(market: string): Promise<Depth> {
    const response = await axios.get(`${BASE_URL}/depth`, { params: { market } });
    return response.data.payload;
}

export async function getTrades(market: string): Promise<Trade[]> {
    const response = await axios.get(`${BASE_URL}/trades`, { params: { market, limit: 50 } });
    return response.data.trades ?? [];
}

/** Returns candles oldest-first, which is the order lightweight-charts requires. */
export async function getKlines(market: string, interval: string): Promise<KLine[]> {
    const response = await axios.get(`${BASE_URL}/klines/${interval}`, { params: { market } });
    const data: KLine[] = response.data ?? [];
    return data.sort((x, y) => new Date(x.end).getTime() - new Date(y.end).getTime());
}

/**
 * Places an order. The API answers 4xx with {error, code} when the engine
 * rejects it (insufficient funds, no liquidity), so surface that message rather
 * than a generic failure.
 */
export async function createOrder(params: {
    market: string;
    price: string;
    quantity: string;
    side: "buy" | "sell";
    type: "limit" | "market";
    userId: string;
}): Promise<OrderResult> {
    try {
        const response = await axios.post(`${BASE_URL}/order`, params);
        return response.data;
    } catch (err: any) {
        throw new Error(err?.response?.data?.error ?? err?.message ?? "Order failed");
    }
}

export async function cancelOrder(orderId: string, market: string): Promise<void> {
    await axios.delete(`${BASE_URL}/order`, { data: { orderId, market } });
}

export async function getOpenOrders(userId: string, market: string): Promise<OpenOrder[]> {
    const response = await axios.get(`${BASE_URL}/order/open`, { params: { userId, market } });
    return response.data ?? [];
}

export async function getBalances(userId: string): Promise<Balances> {
    const response = await axios.get(`${BASE_URL}/balance/${userId}`);
    return response.data ?? {};
}

/** Credits one asset. Omit asset and the API defaults to the quote currency. */
export async function onRamp(userId: string, amount: string, asset?: string): Promise<void> {
    try {
        await axios.post(`${BASE_URL}/onramp`, { userId, amount, asset });
    } catch (err: any) {
        throw new Error(err?.response?.data?.error ?? err?.response?.data ?? err?.message ?? "Deposit failed");
    }
}

export async function getVirtualUsers(): Promise<VirtualUser[]> {
    const response = await axios.get(`${BASE_URL}/users/virtual`);
    return response.data ?? [];
}

/** amount is credited to every asset; pass "" to create the account unfunded. */
export async function createVirtualUser(name: string, amount: string): Promise<VirtualUser> {
    try {
        const response = await axios.post(`${BASE_URL}/users/virtual`, { name, amount });
        return response.data;
    } catch (err: any) {
        throw new Error(err?.response?.data?.error ?? err?.message ?? "Could not create user");
    }
}
