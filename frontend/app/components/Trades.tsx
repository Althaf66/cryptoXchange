"use client";

import { useEffect, useState } from "react";
import { getTrades } from "../utils/httpClient";
import { SignalingManager } from "../utils/SignalingManager";

interface TradeRow {
    id: string;
    price: string;
    quantity: string;
    isBuyerMaker: boolean;
    timestamp: number;
}

// 23 rows is what fills the fixed panel height in page.tsx alongside the
// 15-per-side order book. Change both together.
const MAX_ROWS = 23;

export function Trades({ market }: { market: string }) {
    const [trades, setTrades] = useState<TradeRow[]>([]);

    useEffect(() => {
        const stream = `trade@${market}`;
        const callbackId = `TRADE-${market}`;

        SignalingManager.getInstance().registerCallback(
            stream,
            (data: any) => {
                setTrades((prev) =>
                    [
                        {
                            id: String(data.id),
                            price: data.price,
                            quantity: data.quantity,
                            isBuyerMaker: data.isBuyerMaker,
                            timestamp: data.timestamp,
                        },
                        ...prev,
                    ].slice(0, MAX_ROWS)
                );
            },
            callbackId
        );

        getTrades(market)
            .then((history) =>
                setTrades(
                    history.slice(0, MAX_ROWS).map((t: any) => ({
                        id: String(t.id),
                        price: String(t.price),
                        quantity: String(t.volume ?? t.quantity),
                        isBuyerMaker: t.is_buyer_maker ?? t.isBuyerMaker,
                        // REST returns an RFC3339 string, the WS stream sends epoch ms.
                        timestamp: new Date(t.timestamp).getTime(),
                    }))
                )
            )
            .catch((err) => console.error(`Failed to fetch trades for ${market}:`, err));

        return () => {
            SignalingManager.getInstance().deRegisterCallback(stream, callbackId);
        };
    }, [market]);

    return (
        <div className="flex flex-col flex-1 min-h-0 px-3 py-3">
            <p className="pb-2 text-sm font-medium text-white">Recent trades</p>
            <div className="flex justify-between text-xs text-slate-400">
                <div>Price</div>
                <div>Size</div>
                <div>Time</div>
            </div>
            {/* flex-1 + min-h-0 so the list takes the rest of the column and
                scrolls. A fixed max-height left the bottom of the panel blank
                while rows sat scrolled out of view. */}
            <div className="flex flex-col flex-1 min-h-0 overflow-y-auto no-scrollbar">
                {trades.length === 0 && <p className="py-3 text-xs text-slate-500">No trades yet.</p>}
                {trades.map((trade, i) => (
                    <div key={`${trade.id}-${i}`} className="flex justify-between text-xs py-[2px] tabular-nums">
                        <div className={trade.isBuyerMaker ? "text-red-400" : "text-green-400"}>
                            {trade.price}
                        </div>
                        <div className="text-white">{trade.quantity}</div>
                        <div className="text-slate-500">{formatTime(trade.timestamp)}</div>
                    </div>
                ))}
            </div>
        </div>
    );
}

function formatTime(ms: number) {
    if (!ms || Number.isNaN(ms)) return "-";
    return new Date(ms).toLocaleTimeString();
}
