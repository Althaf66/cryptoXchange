"use client";

import { useCallback, useEffect, useState } from "react";
import { formatAmount } from "../utils/format";
import { getBalances } from "../utils/httpClient";
import { Balances as BalancesMap } from "../utils/types";
import { useUser } from "./UserContext";

/**
 * Balances come straight from the matching engine. "Locked" is the amount
 * reserved by resting orders - the visible proof the engine does real
 * accounting rather than just printing trades.
 */
export function Balances({ market }: { market: string }) {
    const { userId } = useUser();
    const [balances, setBalances] = useState<BalancesMap>({});

    const refresh = useCallback(() => {
        getBalances(userId)
            .then(setBalances)
            .catch(() => setBalances({}));
    }, [userId]);

    useEffect(() => {
        refresh();
        const interval = setInterval(refresh, 2000);
        return () => clearInterval(interval);
    }, [refresh]);

    // Always show both sides of the pair, even at zero.
    const assets = Array.from(new Set([...market.split("_"), ...Object.keys(balances)]));

    return (
        <div className="flex flex-col border-t border-slate-800 px-3 py-3">
            <p className="pb-2 text-sm font-medium text-white">Balances</p>
            <table className="w-full table-auto text-xs">
                <thead>
                    <tr className="text-left text-slate-400">
                        <th className="py-1 font-normal">Asset</th>
                        <th className="py-1 text-right font-normal">Available</th>
                        <th className="py-1 text-right font-normal">Locked</th>
                    </tr>
                </thead>
                <tbody>
                    {assets.map((asset) => (
                        <tr key={asset} className="text-white">
                            <td className="py-1">{asset}</td>
                            <td className="py-1 text-right tabular-nums">
                                {formatAmount(balances[asset]?.available)}
                            </td>
                            <td className="py-1 text-right tabular-nums text-slate-400">
                                {formatAmount(balances[asset]?.locked)}
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
