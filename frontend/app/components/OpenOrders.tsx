"use client";

import { useCallback, useEffect, useState } from "react";
import { formatAmount } from "../utils/format";
import { cancelOrder, getOpenOrders } from "../utils/httpClient";
import { OpenOrder } from "../utils/types";
import { useUser } from "./UserContext";

/**
 * ponytail: polls every 2s rather than opening a per-user WebSocket stream.
 * Swap to internal/ws/user.go if the latency ever shows.
 */
export function OpenOrders({ market, onCancelled }: { market: string; onCancelled?: () => void }) {
    const { userId } = useUser();
    const [orders, setOrders] = useState<OpenOrder[]>([]);
    const [cancelling, setCancelling] = useState<string | null>(null);

    const refresh = useCallback(() => {
        getOpenOrders(userId, market)
            .then(setOrders)
            .catch(() => setOrders([]));
    }, [userId, market]);

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 2000);
        return () => clearInterval(interval);
    }, [refresh]);

    const onCancel = async (orderId: string) => {
        setCancelling(orderId);
        try {
            await cancelOrder(orderId, market);
            setOrders((prev) => prev.filter((o) => o.orderId !== orderId));
            onCancelled?.();
        } finally {
            setCancelling(null);
        }
    };

    return (
        <div className="flex flex-col px-4 py-3">
            <p className="pb-2 text-sm font-medium text-white">
                Open orders <span className="text-slate-500">({orders.length})</span>
            </p>

            {orders.length === 0 ? (
                <p className="py-3 text-xs text-slate-500">No open orders.</p>
            ) : (
                // Scrolls itself rather than forcing the whole page sideways on
                // a phone; the order id drops out below sm, where six columns
                // will not fit.
                <div className="overflow-x-auto">
                    <table className="w-full table-auto text-xs">
                        <thead>
                            <tr className="text-left text-slate-400">
                                <th className="py-2 font-normal">Side</th>
                                <th className="py-2 font-normal">Price</th>
                                <th className="py-2 font-normal">Quantity</th>
                                <th className="py-2 font-normal">Filled</th>
                                <th className="hidden py-2 font-normal sm:table-cell">Order</th>
                                <th className="py-2 font-normal"></th>
                            </tr>
                        </thead>
                        <tbody>
                            {orders.map((order) => (
                                <tr key={order.orderId} className="border-t border-slate-800 text-white">
                                    <td className={`py-2 ${order.side === "buy" ? "text-green-400" : "text-red-400"}`}>
                                        {order.side}
                                    </td>
                                    <td className="py-2 tabular-nums">{formatAmount(order.price)}</td>
                                    <td className="py-2 tabular-nums">{formatAmount(order.quantity)}</td>
                                    <td className="py-2 tabular-nums">{formatAmount(order.filled)}</td>
                                    <td className="hidden py-2 text-slate-500 sm:table-cell">{order.orderId}</td>
                                    <td className="py-2 text-right">
                                        <button
                                            type="button"
                                            onClick={() => onCancel(order.orderId)}
                                            disabled={cancelling === order.orderId}
                                            className="whitespace-nowrap rounded bg-red-500 px-3 py-1 font-medium text-black hover:bg-red-600 disabled:opacity-50"
                                        >
                                            {cancelling === order.orderId ? "Cancelling…" : "Cancel order"}
                                        </button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}
