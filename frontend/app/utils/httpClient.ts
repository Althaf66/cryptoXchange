import axios from "axios";
import { Depth, KLine, Ticker, Trade } from "./types";

// const BASE_URL = "https://exchange-proxy.100xdevs.com/api/v1";
const BASE_URL = "http://localhost:8080/v1";

export async function getTicker(market: string): Promise<Ticker> {
    const tickers = await getTickers();
    // const ticker = tickers.find(t => t.symbol === market);
    // if (!ticker) {
    //     console.error(`No ticker found for market: ${market}`);
    //     throw new Error(`No ticker found for ${market}`);
    // }
    return tickers;
}

export async function getTickers(): Promise<Ticker> {
    const response = await axios.get(`${BASE_URL}/latestprice`);
    // console.log("Tickers response:", response.data);
    return response.data;
}


export async function getDepth(): Promise<Depth> {
    const response = await axios.get(`${BASE_URL}/depth`);
    console.log("Depth response:", response.data.payload);
    return response.data.payload;
}
export async function getTrades(market: string): Promise<Trade[]> {
    const response = await axios.get(`${BASE_URL}/trades`);
    // console.log("Trades response:", response.data);
    return response.data;
}

export async function getKlines(market: string, interval: string, startTime: number, endTime: number): Promise<KLine[]> {
    const response = await axios.get(`${BASE_URL}/klines/${interval}`);
    const data: KLine[] = response.data;
    return data.sort((x, y) => (Number(x.end) < Number(y.end) ? -1 : 1));
}
