import { Ticker } from "./types";

// Zerops' generated subdomain variable only ever yields an https:// URL, so the
// scheme is rewritten here rather than hardcoded: https -> wss, http -> ws, and
// an already-ws:// value is left alone.
export const BASE_URL = (process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:3001/v1/ws")
  .replace(/^http/, "ws");

interface DepthData {
  b: [string, string][];
  a: [string, string][];
  id: number;
  e: string;
}

interface TradeData {
  e: string;
  id: string;
  isBuyerMaker: boolean;
  price: string;
  quantity: string;
  timestamp: number;
  market: string;
}

interface OutgoingMessage {
  stream: string;
  data?: DepthData | null;
  tickerdata?: Partial<Ticker>;
  tradeData?: TradeData | null;
}

export class SignalingManager {
  private ws: WebSocket;
  private static instance: SignalingManager;
  private callbacks: { [key: string]: { callback: (data: any) => void; id: string }[] } = {};
  private id: number;
  private initialized: boolean = false;
  // Streams are remembered so a reconnect can re-subscribe. Without this the
  // components had to re-send SUBSCRIBE on a 50ms timer.
  private subscriptions: Set<string> = new Set();
  private reconnectDelay: number = 500;

  private constructor() {
    this.ws = new WebSocket(BASE_URL);
    this.id = 1;
    this.init();
    console.log("SignalingManager initialized");
  }

  public static getInstance() {
    if (!this.instance) {
      this.instance = new SignalingManager();
    }
    return this.instance;
  }

  init() {
    this.ws.onopen = () => {
      this.initialized = true;
      this.reconnectDelay = 500;
      // `subscriptions` is the desired end state, so resending it is enough.
      // Replaying a buffered log of intents on top used to undo it: a stale
      // UNSUBSCRIBE queued during the outage would kill a stream the page had
      // just re-established, leaving the book blank until a reload.
      this.subscriptions.forEach((stream) => {
        this.ws.send(JSON.stringify({ method: "SUBSCRIBE", params: [stream], id: this.id++ }));
      });
      console.log("WebSocket connection established");
    };

    this.ws.onmessage = (event) => {
      let message: OutgoingMessage;
      try {
        message = JSON.parse(event.data);
      } catch (e) {
        console.error("Discarding unparseable WebSocket frame:", e);
        return;
      }
      console.log("WebSocket message received:", message);

      const stream = message.stream;

      // Trades arrive under `tradeData` with `data` null, so they can't be
      // routed by data.e like depth and ticker are.
      if (message.tradeData && this.callbacks[stream]) {
        this.callbacks[stream].forEach(({ callback }) => callback(message.tradeData));
        return;
      }

      const type = message.data?.e || "";
      console.log("Message type:", type);

      if (type === "depth" && this.callbacks[stream]) {
        console.log("Depth update received for stream:", stream);
        this.callbacks[stream].forEach(({ callback }) => {
          callback({ bids: message.data?.b, asks: message.data?.a });
        });
      } else if (type === "ticker" && this.callbacks[stream]) {
        console.log("Ticker update received for stream:", stream);
        const newTicker: Partial<Ticker> = {
          lastPrice: message.tickerdata?.lastPrice,
          high: message.tickerdata?.high,
          low: message.tickerdata?.low,
          volume: message.tickerdata?.volume,
          quoteVolume: message.tickerdata?.quoteVolume,
          symbol: message.tickerdata?.symbol,
        };
        this.callbacks[stream].forEach(({ callback }) => callback(newTicker));
      }
    };

    this.ws.onclose = () => {
      // Back off instead of hammering a server that is down; reset once a
      // connection sticks.
      const delay = this.reconnectDelay;
      this.reconnectDelay = Math.min(delay * 2, 10000);
      console.log(`WebSocket connection closed. Reconnecting in ${delay}ms...`);
      this.initialized = false;
      setTimeout(() => {
        this.ws = new WebSocket(BASE_URL);
        this.init();
      }, delay);
    };

    this.ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };
  }

  sendMessage(message: any) {
    if (message.method === "SUBSCRIBE") {
      (message.params ?? []).forEach((s: string) => this.subscriptions.add(s));
    } else if (message.method === "UNSUBSCRIBE") {
      (message.params ?? []).forEach((s: string) => this.subscriptions.delete(s));
    }

    const messageToSend = {
      ...message,
      id: this.id++,
    };
    // Dropped while disconnected on purpose: onopen replays `subscriptions`,
    // which already reflects the SUBSCRIBE/UNSUBSCRIBE applied just above.
    if (!this.initialized) return;
    this.ws.send(JSON.stringify(messageToSend));
  }

  /**
   * Subscriptions are reference counted by callback. Several components share a
   * stream (Trades and TradeView both consume trade@{market}), so the socket
   * must only unsubscribe once the last consumer is gone.
   */
  async registerCallback(stream: string, callback: (data: any) => void, id: string) {
    this.callbacks[stream] = this.callbacks[stream] || [];

    // ids are deterministic per market, and React StrictMode mounts every
    // component twice in dev. Without this the count never returns to zero,
    // so the stream is never unsubscribed and the first callback fires against
    // an unmounted component for the rest of the session.
    const existing = this.callbacks[stream].findIndex((cb) => cb.id === id);
    if (existing !== -1) {
      this.callbacks[stream][existing] = { callback, id };
      return;
    }

    this.callbacks[stream].push({ callback, id });

    if (this.callbacks[stream].length === 1) {
      this.sendMessage({ method: "SUBSCRIBE", params: [stream] });
    }
  }

  async deRegisterCallback(stream: string, id: string) {
    if (!this.callbacks[stream]) return;

    const index = this.callbacks[stream].findIndex((cb) => cb.id === id);
    if (index === -1) return;
    this.callbacks[stream].splice(index, 1);

    if (this.callbacks[stream].length === 0) {
      delete this.callbacks[stream];
      this.sendMessage({ method: "UNSUBSCRIBE", params: [stream] });
    }
  }
}