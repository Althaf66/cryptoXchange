import { Ticker } from "./types";

export const BASE_URL = "ws://localhost:3001/v1/ws";

interface DepthData {
  b: [string, string][];
  a: [string, string][];
  id: number;
  e: string;
}

interface OutgoingMessage {
  stream: string;
  data?: DepthData;
  tickerdata?: Partial<Ticker>;
  tradedata?: any;
}

export class SignalingManager {
  private ws: WebSocket;
  private static instance: SignalingManager;
  private bufferedMessages: any[] = [];
  private callbacks: { [key: string]: { callback: (data: any) => void; id: string }[] } = {};
  private id: number;
  private initialized: boolean = false;

  private constructor() {
    this.ws = new WebSocket(BASE_URL);
    this.bufferedMessages = [];
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
      this.bufferedMessages.forEach((message) => {
        this.ws.send(JSON.stringify(message));
      });
      console.log("WebSocket connection established");
      this.bufferedMessages = [];
    };

    this.ws.onmessage = (event) => {
      const message: OutgoingMessage = JSON.parse(event.data);
      console.log("WebSocket message received:", message);

      const type = message.data?.e || "";
      console.log("Message type:", type);
      const stream = message.stream;

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
      console.log("WebSocket connection closed. Attempting to reconnect...");
      this.initialized = false;
      setTimeout(() => {
        this.ws = new WebSocket(BASE_URL);
        this.init();
      }, 100);
    };

    this.ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };
  }

  sendMessage(message: any) {
    const messageToSend = {
      ...message,
      id: this.id++,
    };
    if (!this.initialized) {
      this.bufferedMessages.push(messageToSend);
      return;
    }
    this.ws.send(JSON.stringify(messageToSend));
  }

  async registerCallback(stream: string, callback: (data: any) => void, id: string) {
    this.callbacks[stream] = this.callbacks[stream] || [];
    this.callbacks[stream].push({ callback, id });
    console.log(`Registered callback for stream: ${stream}`);
  }

  async deRegisterCallback(stream: string, id: string) {
    if (this.callbacks[stream]) {
      const index = this.callbacks[stream].findIndex((cb) => cb.id === id);
      if (index !== -1) {
        this.callbacks[stream].splice(index, 1);
        console.log(`Deregistered callback for stream: ${stream}, id: ${id}`);
      }
    }
  }
}