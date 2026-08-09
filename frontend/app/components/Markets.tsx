"use client";

import { useEffect, useState } from "react";
import { Ticker } from "../utils/types";
import { getTickers } from "../utils/httpClient";
import { formatAmount } from "../utils/format";
import { useRouter } from "next/navigation";

const CARD = "rounded-xl border border-baseBorderLight bg-baseBackgroundL1 p-4";

export const Markets = () => {
  const [tickers, setTickers] = useState<Ticker[]>();

  useEffect(() => {
    getTickers()
      .then(setTickers)
      .catch((err) => {
        console.error("Failed to fetch tickers:", err);
        // Empty rather than undefined, so the UI shows the notice instead of
        // spinning forever.
        setTickers([]);
      });
  }, []);

  if (!tickers) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className={`${CARD} h-[132px] animate-pulse`} />
        ))}
      </div>
    );
  }

  if (tickers.length === 0) {
    return (
      <p className={`${CARD} text-sm text-slate-400`}>
        No markets available. Is the engine running?
      </p>
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {tickers.map((m) => <MarketCard key={m.symbol} market={m} />)}
    </div>
  );
};

function MarketCard({ market }: { market: Ticker }) {
  const router = useRouter();
  const [base, quote] = market.symbol.split("_");
  const change = Number(market.priceChangePercent);
  const up = !(change < 0);

  return (
    <button
      type="button"
      onClick={() => router.push(`/trade/${market.symbol}`)}
      className={`${CARD} w-full text-left transition hover:bg-white/5 focus:outline-none focus:ring-1 focus:ring-blue-500`}
    >
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 flex-none items-center justify-center rounded-full bg-slate-700 text-xs font-medium text-white">
          {base}
        </div>
        <div className="min-w-0">
          <p className="truncate text-[0.95rem] font-medium text-white">
            {base}/{quote}
          </p>
          <p className="text-xs text-slate-400">{quote} market</p>
        </div>
      </div>

      <div className="flex items-end justify-between pt-4">
        <p className="text-[1.35rem] font-medium tabular-nums leading-none text-white">
          {formatAmount(Number(market.lastPrice))}
        </p>
        <span
          className={`rounded-full px-2 py-1 text-xs font-medium tabular-nums ${
            up
              ? "bg-greenBackgroundTransparent text-greenPrimaryButtonBackground"
              : "bg-redBackgroundTransparent text-red-400"
          }`}
        >
          {up ? "+" : ""}
          {Number.isFinite(change) ? change.toFixed(2) : "0.00"}%
        </span>
      </div>

      <div className="grid grid-cols-3 gap-2 pt-4 text-xs">
        <Stat label="24h High" value={market.high} />
        <Stat label="24h Low" value={market.low} />
        <Stat label="Volume" value={market.volume} />
      </div>
    </button>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-slate-500">{label}</p>
      <p className="pt-0.5 tabular-nums text-slate-300">
        {formatAmount(Number(value))}
      </p>
    </div>
  );
}
