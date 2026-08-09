"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

/**
 * First-run tour of the trading view. Armed by creating a demo account (see
 * VirtualUsers), shown on the first trade page opened after that, then never
 * again for this browser.
 */
const TOUR_KEY = "cryptoxchange:tour";

/** Called after an account is created, so the next trade page runs the tour. */
export function armTour() {
    localStorage.setItem(TOUR_KEY, "pending");
}

const STEPS = [
    {
        target: "chart",
        title: "Price chart",
        body: "Live candles, redrawn as trades execute. Switch between 1m, 1h and 1w above it.",
    },
    {
        target: "depth",
        title: "Order book",
        body: "Every resting bid and ask. Click a price to load it into the order form.",
    },
    {
        target: "trades",
        title: "Recent trades",
        body: "The tape - each fill the matching engine has made, newest first.",
    },
    {
        target: "swap",
        title: "Place an order",
        body: "Buy or sell, limit or market. Your demo account is already funded, so try one.",
    },
    {
        target: "orders",
        title: "Your open orders",
        body: "Anything that hasn't filled rests here until it matches. Cancel it any time.",
    },
];

/** Breathing room around the highlighted panel, in px. */
const PAD = 6;
const CARD_W = 300;
/** Enough to decide flip-above vs below before the card has been measured. */
const CARD_H = 170;

type Rect = { top: number; left: number; width: number; height: number };

export function Tour() {
    // Which steps actually exist in this layout. Empty until mounted, which also
    // keeps the server render and first client render identical.
    const [steps, setSteps] = useState<typeof STEPS>([]);
    const [index, setIndex] = useState(0);
    const [rect, setRect] = useState<Rect | null>(null);
    const cardRef = useRef<HTMLDivElement>(null);

    const finish = useCallback(() => {
        localStorage.setItem(TOUR_KEY, "done");
        setSteps([]);
    }, []);

    // localStorage is read after mount: it does not exist during the server
    // render, the same reason UserContext defers its read.
    useEffect(() => {
        if (localStorage.getItem(TOUR_KEY) !== "pending") return;
        const present = STEPS.filter((s) => document.querySelector(`[data-tour="${s.target}"]`));
        // A panel missing from this layout is skipped rather than spotlighting
        // an empty rect.
        if (present.length) setSteps(present);
        else localStorage.setItem(TOUR_KEY, "done");
    }, []);

    const step = steps[index];

    // Layout effect so the highlight is positioned in the same frame the step
    // changes - otherwise the hole flashes at the previous panel.
    useLayoutEffect(() => {
        if (!step) return;
        const el = document.querySelector(`[data-tour="${step.target}"]`);
        if (!el) return;

        const measure = () => {
            const r = el.getBoundingClientRect();
            setRect({ top: r.top, left: r.left, width: r.width, height: r.height });
        };

        el.scrollIntoView({ block: "center", behavior: "smooth" });
        measure();
        // Smooth scrolling means the rect keeps moving after the first measure.
        const timer = window.setInterval(measure, 100);
        const stop = window.setTimeout(() => window.clearInterval(timer), 700);

        window.addEventListener("resize", measure, { passive: true });
        window.addEventListener("scroll", measure, { passive: true });
        return () => {
            window.clearInterval(timer);
            window.clearTimeout(stop);
            window.removeEventListener("resize", measure);
            window.removeEventListener("scroll", measure);
        };
    }, [step]);

    useEffect(() => {
        if (!step) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") finish();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [step, finish]);

    useEffect(() => {
        cardRef.current?.focus();
    }, [index]);

    if (!step || !rect) return null;

    const last = index === steps.length - 1;

    // Below the panel by default, above when it would fall off the bottom.
    const below = rect.top + rect.height + PAD + CARD_H < window.innerHeight;
    const cardTop = below ? rect.top + rect.height + PAD + 10 : rect.top - PAD - CARD_H - 10;
    const cardLeft = Math.min(
        Math.max(8, rect.left + rect.width / 2 - CARD_W / 2),
        window.innerWidth - CARD_W - 8,
    );

    return (
        <>
            {/* Swallows clicks on the dimmed UI while the tour is running. */}
            <div className="fixed inset-0 z-40" onClick={(e) => e.stopPropagation()} />

            {/* One element does the whole spotlight: a huge spread shadow dims
                everything outside this rect, and the rect itself stays clear. */}
            <div
                className="pointer-events-none fixed z-40 rounded-lg"
                style={{
                    top: rect.top - PAD,
                    left: rect.left - PAD,
                    width: rect.width + PAD * 2,
                    height: rect.height + PAD * 2,
                    boxShadow: "0 0 0 9999px rgba(0,0,0,0.72)",
                }}
            />

            <div
                ref={cardRef}
                tabIndex={-1}
                role="dialog"
                aria-label={step.title}
                className="fixed z-50 rounded-xl border border-baseBorderLight bg-baseBackgroundL2 p-4 shadow-xl focus:outline-none"
                style={{ top: Math.max(8, cardTop), left: cardLeft, width: CARD_W }}
            >
                <p className="text-xs text-slate-500">
                    Step {index + 1} of {steps.length}
                </p>
                <p className="pt-1 text-sm font-medium text-white">{step.title}</p>
                <p className="pt-1 text-xs leading-5 text-slate-400">{step.body}</p>

                <div className="flex items-center justify-between pt-4">
                    <button
                        type="button"
                        onClick={finish}
                        className="text-xs text-slate-500 hover:text-white"
                    >
                        Skip
                    </button>
                    <div className="flex gap-2">
                        {index > 0 && (
                            <button
                                type="button"
                                onClick={() => setIndex(index - 1)}
                                className="rounded-lg border border-baseBorderLight px-3 py-1.5 text-xs text-slate-300 hover:text-white"
                            >
                                Back
                            </button>
                        )}
                        <button
                            type="button"
                            onClick={() => (last ? finish() : setIndex(index + 1))}
                            className="rounded-lg bg-greenPrimaryButtonBackground px-3 py-1.5 text-xs font-semibold text-greenPrimaryButtonText"
                        >
                            {last ? "Done" : "Next"}
                        </button>
                    </div>
                </div>
            </div>
        </>
    );
}
