"use client";

import { useEffect, useState } from "react";
import { getDepth, getTicker, getTrades } from "../../utils/httpClient";
import { SignalingManager } from "../../utils/SignalingManager";
import { AskTable } from "./AskTable";
import { BidTable } from "./BidTable";

// Define interfaces for type safety
interface DepthData {
  bids: [string, string][];
  asks: [string, string][];
}

interface Ticker {
  price: string;
}

interface Trade {
  price: string;
}

export function Depth({ market }: { market: string }) {
  const [bids, setBids] = useState<[string, string][] | undefined>();
  const [asks, setAsks] = useState<[string, string][] | undefined>();
  const [price, setPrice] = useState<string | undefined>();

  useEffect(() => {
    console.log(`Depth component mounted for market: ${market}`);

    const stream = `depth@${market}`;
    const callbackId = `DEPTH-${market}`;

    // Register callback for depth updates
    SignalingManager.getInstance().registerCallback(
      stream,
      (data: DepthData) => {

        // Update bids
        setBids((originalBids = []) => {
          // Keep only bids that are in the new data or have non-zero quantity
          const bidsAfterUpdate = originalBids.filter((bid) =>
            data.bids.some((newBid) => newBid[0] === bid[0] && Number(newBid[1]) !== 0)
          );

          // Update quantities and add new bids
          for (const newBid of data.bids) {
            if (Number(newBid[1]) !== 0) {
              const existingIndex = bidsAfterUpdate.findIndex((bid) => bid[0] === newBid[0]);
              if (existingIndex >= 0) {
                bidsAfterUpdate[existingIndex][1] = newBid[1];
              } else {
                bidsAfterUpdate.push(newBid);
              }
            }
          }

          // Sort bids in descending order
          const sortedBids = bidsAfterUpdate.sort((x, y) => Number(y[0]) - Number(x[0]));
          return sortedBids;
        });

        // Update asks
        setAsks((originalAsks = []) => {
          // Keep only asks that are in the new data or have non-zero quantity
          const asksAfterUpdate = originalAsks.filter((ask) =>
            data.asks.some((newAsk) => newAsk[0] === ask[0] && Number(newAsk[1]) !== 0)
          );

          // Update quantities and add new asks
          for (const newAsk of data.asks) {
            if (Number(newAsk[1]) !== 0) {
              const existingIndex = asksAfterUpdate.findIndex((ask) => ask[0] === newAsk[0]);
              if (existingIndex >= 0) {
                asksAfterUpdate[existingIndex][1] = newAsk[1];
              } else {
                asksAfterUpdate.push(newAsk);
              }
            }
          }

          // Sort asks in ascending order
            asksAfterUpdate.sort((x, y) => Number(y[0]) > Number(x[0]) ? 1 : -1);
            return asksAfterUpdate;
        });
      },
      callbackId
    );

    // Subscribe to depth@{market} channel
    const subscribe = () => {
      SignalingManager.getInstance().sendMessage({
        method: "SUBSCRIBE",
        params: [stream],
      });
    };

    // Initial subscription
    subscribe();

    // Re-subscribe on WebSocket reconnect
    const reconnectInterval = setInterval(() => {
      if (!SignalingManager.getInstance()) {
        console.warn(`SignalingManager not initialized for ${stream}`);
        return;
      }
      subscribe();
    }, 50);

    // Fetch initial depth data
    getDepth()
      .then((d: DepthData) => {
        setBids(d.bids.reverse());
        setAsks(d.asks);
        console.log(`Initial depth fetched for ${market}:`, JSON.stringify(d, null, 2));
      })
      .catch((err) => console.error(`Failed to fetch depth for ${market}:`, err));

    // Fetch initial price
    getTicker(market)
      .then((t: Ticker) => {
        setPrice(t.price);
        console.log(`Initial ticker price for ${market}:`, t.price);
      })
      .catch((err) => console.error(`Failed to fetch ticker for ${market}:`, err));
    getTrades(market)
      .then((t: Trade[]) => {
        setPrice(t[0].price);
        console.log(`Initial trade price for ${market}:`, t[0].price);
      })
      .catch((err) => console.error(`Failed to fetch trades for ${market}:`, err));

    // Cleanup on unmount
    return () => {
      console.log(`Unsubscribing from ${stream}`);
      SignalingManager.getInstance().sendMessage({
        method: "UNSUBSCRIBE",
        params: [stream],
      });
      SignalingManager.getInstance().deRegisterCallback(stream, callbackId);
      clearInterval(reconnectInterval);
    };
  }, []);

return <div>
        <TableHeader />
        {asks && <AskTable asks={asks} />}
        {price && <div>{price}</div>}
        {bids && <BidTable bids={bids} />}
    </div>
}

function TableHeader() {
    return <div className="flex justify-between text-xs">
    <div className="text-white">Price</div>
    <div className="text-slate-500">Size</div>
    <div className="text-slate-500">Total</div>
</div>
}