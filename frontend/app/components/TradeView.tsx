"use client";

import { useEffect, useRef, useState } from "react";
import { ChartManager } from "../utils/ChartManager";
import { getKlines } from "../utils/httpClient";
import { SignalingManager } from "../utils/SignalingManager";
import { KLine } from "../utils/types";

// The intervals backed by materialized views (klines_1m/1h). Adding another one
// here without adding its view returns an "invalid interval" error.
//
// 1w was removed: sol_prices only retains two days, so that view could never
// hold a complete bucket and the chart drew it as one candle.
const INTERVALS = ["1m", "1h"] as const;
type Interval = (typeof INTERVALS)[number];

// Tooltip text - "1m" alone is ambiguous between minute and month.
const INTERVAL_LABELS: Record<Interval, string> = {
  "1m": "1 minute",
  "1h": "1 hour",
};

// The materialized views only refresh on a 10s cron, so REST is the slow,
// authoritative source. Live trades drive the chart between reconciles; this
// just heals anything missed while the socket was down.
const RECONCILE_MS = 30_000;

interface Candle {
  timestamp: number; // bucket END, matching the API's `end` field
  start: number; // bucket START, so the live path can derive the width
  open: number;
  high: number;
  low: number;
  close: number;
}

function toCandles(klines: KLine[]): Candle[] {
  return klines
    .map((k) => ({
      open: Number(k.open),
      high: Number(k.high),
      low: Number(k.low),
      close: Number(k.close),
      timestamp: new Date(k.end).getTime(),
      start: new Date(k.start).getTime(),
    }))
    .filter((c) => Number.isFinite(c.timestamp) && Number.isFinite(c.start))
    .sort((a, b) => a.timestamp - b.timestamp);
}

export function TradeView({ market }: { market: string }) {
  const chartRef = useRef<HTMLDivElement>(null);
  const chartManagerRef = useRef<ChartManager | null>(null);
  const lastCandleRef = useRef<Candle | null>(null);
  // Not named setInterval: that shadows window.setInterval, which the reconcile
  // poll below depends on, and it fails silently rather than as a type error.
  const [chartInterval, setChartInterval] = useState<Interval>("1m");

  useEffect(() => {
    let cancelled = false;
    const stream = `trade@${market}`;
    const callbackId = `KLINE-${market}`;

    const reconcile = async () => {
      let klineData: KLine[] = [];
      try {
        klineData = await getKlines(market, chartInterval);
      } catch (e) {
        console.error(`Failed to fetch ${chartInterval} klines for ${market}:`, e);
        return;
      }
      if (cancelled || !chartRef.current) return;

      const candles = toCandles(klineData);
      const live = lastCandleRef.current;

      // Don't let a stale view overwrite the bucket we're building live.
      if (live && candles.length && candles[candles.length - 1].timestamp <= live.timestamp) {
        candles[candles.length - 1] =
          candles[candles.length - 1].timestamp === live.timestamp
            ? live
            : candles[candles.length - 1];
        if (candles[candles.length - 1].timestamp < live.timestamp) candles.push(live);
      }

      lastCandleRef.current = candles[candles.length - 1] ?? null;

      if (chartManagerRef.current) {
        chartManagerRef.current.setCandles(candles);
        return;
      }
      // Reuse the chart across refreshes so it doesn't flicker or lose zoom.
      chartManagerRef.current = new ChartManager(chartRef.current, candles, {
        background: "#0e0f14",
        color: "white",
      });
    };

    // Fold each executed trade into the candle currently forming.
    //
    // Boundaries come from the server's own start/end rather than epoch maths.
    // time_bucket('1 week') originates on a Monday while Math.floor(ms / WEEK)
    // originates on a Thursday, so computing weekly buckets here would draw a
    // live candle beside the real one instead of on top of it.
    const onTrade = (data: any) => {
      const price = Number(data?.price);
      const ts = Number(data?.timestamp);
      if (!Number.isFinite(price) || !Number.isFinite(ts)) return;

      // Nothing to anchor to until the first reconcile lands; it runs on mount,
      // so this only skips trades arriving during that initial fetch.
      const prev = lastCandleRef.current;
      if (!prev) return;

      // Older than the bar already drawn. The series is append-only.
      if (ts < prev.start) return;

      let candle: Candle;
      if (ts < prev.timestamp) {
        candle = {
          ...prev,
          high: Math.max(prev.high, price),
          low: Math.min(prev.low, price),
          close: price,
        };
      } else {
        // The bucket rolled over. Advance by whole widths so a gap with no
        // trades still lands on the server's phase.
        const width = prev.timestamp - prev.start;
        if (width <= 0) return;
        const steps = Math.ceil((ts - prev.timestamp + 1) / width);
        const start = prev.timestamp + (steps - 1) * width;
        candle = {
          start,
          timestamp: start + width,
          open: price,
          high: price,
          low: price,
          close: price,
        };
      }

      lastCandleRef.current = candle;
      chartManagerRef.current?.updateCandle(candle);
    };

    SignalingManager.getInstance().registerCallback(stream, onTrade, callbackId);

    reconcile();
    const interval = setInterval(reconcile, RECONCILE_MS);

    return () => {
      cancelled = true;
      clearInterval(interval);
      SignalingManager.getInstance().deRegisterCallback(stream, callbackId);
      chartManagerRef.current?.destroy();
      chartManagerRef.current = null;
      lastCandleRef.current = null;
    };
  }, [market, chartInterval]);

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-3 px-3 py-2">
        <span className="text-xs text-slate-500">Interval</span>
        {/* Segmented control: the three options are always visible and their
            widths are equalised, so the group doesn't reflow when the active
            one changes. */}
        <div
          role="group"
          aria-label="Chart interval"
          className="inline-flex gap-0.5 rounded-lg border border-baseBorderLight bg-baseBackgroundL1 p-0.5"
        >
          {INTERVALS.map((value) => (
            <IntervalButton
              key={value}
              label={INTERVAL_LABELS[value]}
              short={value}
              active={value === chartInterval}
              onClick={() => setChartInterval(value)}
            />
          ))}
        </div>
      </div>
      {/* Classes, not an inline style: inline styles cannot carry breakpoints.
          488 + the 32px selector above + 4px margin fits the 560px panel row in
          trade/[market]/page.tsx; shorter when the layout stacks. ChartManager
          is autoSize, so it follows the container. */}
      <div ref={chartRef} className="mt-1 h-[300px] w-full sm:h-[380px] lg:h-[488px]"></div>
    </div>
  );
}

function IntervalButton({
  active,
  onClick,
  label,
  short,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  short: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      title={label}
      className={`min-w-[2.75rem] rounded-md px-3 py-1.5 text-sm font-medium transition ${
        active
          ? "bg-white/10 text-white"
          : "text-slate-400 hover:bg-white/5 hover:text-white"
      }`}
    >
      {short}
    </button>
  );
}
