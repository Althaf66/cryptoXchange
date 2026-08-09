"use client";

import { useEffect, useRef, useState } from "react";
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

export function Depth({ market, onPriceClick, refreshKey }: { market: string; onPriceClick?: (price: string) => void; refreshKey?: number }) {
  const [bids, setBids] = useState<[string, string][] | undefined>();
  const [asks, setAsks] = useState<[string, string][] | undefined>();
  const [price, setPrice] = useState<string | undefined>();
  const asksRef = useRef<HTMLDivElement>(null);

  // Keep the best ask (last row) in view as levels come and go.
  useEffect(() => {
    const el = asksRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [asks]);

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

            // Descending, best (highest) bid first.
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

            // Ascending, best (lowest) ask first - same order the REST fetch
            // returns, so the book doesn't reorder itself on the first trade.
            asksAfterUpdate.sort((x, y) => Number(x[0]) - Number(y[0]));
            return asksAfterUpdate;
        });
      },
      callbackId
    );

    // SignalingManager subscribes on the first registered callback and replays
    // subscriptions on reconnect. This used to re-send SUBSCRIBE 20x a second.

    // The last traded price only moves when a trade happens, and depth@ never
    // carries it. Without this the number between the two sides of the book
    // stays frozen at whatever it was on page load.
    const tradeStream = `trade@${market}`;
    const tradeCallbackId = `DEPTH-PRICE-${market}`;

    SignalingManager.getInstance().registerCallback(
      tradeStream,
      (data: { price: string }) => setPrice(String(data.price)),
      tradeCallbackId
    );

    // Last traded price, shown between the two sides of the book.
    getTrades(market)
      .then((t: any[]) => {
        if (t.length) setPrice(String(t[0].price));
      })
      .catch((err) => console.error(`Failed to fetch trades for ${market}:`, err));

    // Cleanup on unmount
    return () => {
      SignalingManager.getInstance().deRegisterCallback(stream, callbackId);
      SignalingManager.getInstance().deRegisterCallback(tradeStream, tradeCallbackId);
    };
  }, [market]);

  // The book, straight from the engine. Runs on mount and again whenever the
  // parent bumps refreshKey (a cancel) - the depth stream's incremental updates
  // can't express a removed level, so the REST snapshot is what repairs it.
  // Kept out of the effect above so a refresh doesn't re-register the
  // WebSocket callbacks.
  useEffect(() => {
    getDepth(market)
      .then((d: DepthData) => {
        // The API already returns bids descending and asks ascending, both
        // best-first. Keep them that way; the tables handle display order.
        setBids(d.bids);
        setAsks(d.asks);
      })
      .catch((err) => console.error(`Failed to fetch depth for ${market}:`, err));
  }, [market, refreshKey]);

return <div className="flex flex-col flex-1 min-h-0 px-2">
        <TableHeader />
        {/* Asks render worst-to-best downward, so the best ask is the last row.
            Two separate things keep it against the mid price: this pane is
            scrolled to its bottom when levels overflow (see the effect above),
            and AskTable carries mt-auto for when they don't. flex flex-col is
            what makes that auto margin do anything. */}
        <div ref={asksRef} className="flex flex-col flex-1 min-h-0 overflow-y-auto no-scrollbar">
            {asks && <AskTable asks={asks} onPriceClick={onPriceClick} />}
        </div>
        {price && <div className="py-1 text-sm font-medium text-white tabular-nums">{price}</div>}
        <div className="flex-1 min-h-0 overflow-y-auto no-scrollbar">
            {bids && <BidTable bids={bids} onPriceClick={onPriceClick} />}
        </div>
    </div>
}

function TableHeader() {
    return <div className="flex justify-between text-xs">
    <div className="text-white">Price</div>
    <div className="text-slate-500">Size</div>
    <div className="text-slate-500">Total</div>
</div>
}