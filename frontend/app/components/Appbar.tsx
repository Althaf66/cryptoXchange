"use client";

import { usePathname } from "next/navigation";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { getTickers } from "../utils/httpClient";
import { AddFund } from "./AddFund";
import { useUser } from "./UserContext";

export const Appbar = () => {
    const route = usePathname();
    const router = useRouter()
    const { userId, setUserId, users } = useUser();
    const [markets, setMarkets] = useState<string[]>([]);

    // Browse pages have nothing to trade with yet - account controls only earn
    // their space once you're in a market.
    const isBrowse = route === "/" || route === "/markets";

    // Sourced from /tickers rather than a hardcoded list, so the header always
    // matches the markets the engine actually booted with.
    useEffect(() => {
        getTickers()
            .then((tickers) => setMarkets(tickers.map((t) => t.symbol)))
            .catch((err) => console.error("Failed to fetch markets:", err));
    }, []);

    return <div className="text-white border-b border-slate-800">
        <div className="flex flex-wrap gap-y-2 justify-between items-center p-2">
            <div className="flex min-w-0 flex-1 items-center">
                <div className={`text-xl pl-2 sm:pl-4 flex flex-col justify-center cursor-pointer text-white shrink-0`} onClick={() => router.push('/')}>
                    CryptoXchange
                </div>
                {/* Hidden below sm - it costs width the market list needs. */}
                <div className={`text-sm pt-1 hidden sm:flex flex-col justify-center pl-8 cursor-pointer ${route === '/markets' || route.startsWith('/trade') ? 'text-white' : 'text-slate-500'}`} onClick={() => router.push('/markets')}>
                    Spot Trading
                </div>
                {/* Scrolls sideways rather than overflowing the viewport, reusing
                    the no-scrollbar utility from globals.css. */}
                <div className="flex items-center gap-1 pl-3 sm:pl-6 overflow-x-auto no-scrollbar">
                    {markets.map((symbol) => (
                        <div
                            key={symbol}
                            onClick={() => router.push(`/trade/${symbol}`)}
                            className={`text-sm px-2 py-1 rounded cursor-pointer whitespace-nowrap hover:bg-white/10 ${route === `/trade/${symbol}` ? 'text-white' : 'text-slate-500'}`}
                        >
                            {symbol.replace("_", "/")}
                        </div>
                    ))}
                </div>
            </div>
            {!isBrowse && <div className="flex items-center shrink-0">
                <div className="flex items-center gap-2 pr-2 sm:pr-4">
                    <span className="hidden sm:inline text-xs text-slate-400">Trading as</span>
                    <select
                        value={userId}
                        onChange={(e) => setUserId(e.target.value)}
                        className="rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-white"
                    >
                        {users.map((user) => (
                            <option key={user.id} value={user.id}>{user.name}</option>
                        ))}
                    </select>
                </div>
                <div className="p-2 mr-2">
                    <AddFund markets={markets} />
                </div>
            </div>}
        </div>
    </div>
}