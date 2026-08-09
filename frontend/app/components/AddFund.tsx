"use client";

import { useState } from "react";
import { onRamp } from "../utils/httpClient";
import { SuccessButton } from "./core/Button";
import { useUser } from "./UserContext";

/**
 * Deposits virtual funds into the account currently selected in the header.
 * One deposit credits one asset: the transfers table records a single asset per
 * row, and that row is the idempotency key. To fund everything at once, create
 * the user from the home page instead.
 */
export function AddFund({ markets }: { markets: string[] }) {
    const { userId } = useUser();
    const [open, setOpen] = useState(false);
    const [asset, setAsset] = useState("");
    const [amount, setAmount] = useState("1000");
    const [submitting, setSubmitting] = useState(false);
    const [notice, setNotice] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

    // Quote first, then each market's base - derived from the symbols the header
    // already fetched, so there is no second source of truth for the asset list.
    const quote = markets[0]?.split("_")[1];
    const assets = Array.from(
        new Set([...(quote ? [quote] : []), ...markets.map((symbol) => symbol.split("_")[0])])
    );
    const selected = asset || assets[0] || "";

    const submit = async () => {
        setSubmitting(true);
        setNotice(null);
        try {
            await onRamp(userId, amount, selected);
            setNotice({ kind: "ok", text: `Added ${amount} ${selected}.` });
        } catch (err: any) {
            setNotice({ kind: "error", text: err.message });
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="relative">
            <SuccessButton onClick={() => setOpen((v) => !v)}>Add fund</SuccessButton>

            {open && (
                <div className="absolute right-4 z-10 mt-1 flex w-64 flex-col gap-3 rounded-xl border border-slate-700 bg-slate-900 p-4">
                    <div className="flex flex-col gap-1">
                        <p className="text-xs text-slate-400">Market</p>
                        <select
                            value={selected}
                            onChange={(e) => setAsset(e.target.value)}
                            className="h-9 rounded-lg border border-slate-700 bg-[var(--background)] px-2 text-sm text-white"
                        >
                            {assets.map((a) => (
                                <option key={a} value={a}>{a}</option>
                            ))}
                        </select>
                    </div>

                    <div className="flex flex-col gap-1">
                        <p className="text-xs text-slate-400">Amount</p>
                        <input
                            type="text"
                            inputMode="decimal"
                            value={amount}
                            onChange={(e) => {
                                // Digits and a single decimal point only, as in SwapUI.
                                const next = e.target.value;
                                if (next === "" || /^\d*\.?\d*$/.test(next)) setAmount(next);
                            }}
                            className="h-9 rounded-lg border border-slate-700 bg-[var(--background)] px-2 text-sm text-white placeholder-slate-500 focus:border-blue-500"
                        />
                    </div>

                    <button
                        type="button"
                        onClick={submit}
                        disabled={submitting || !amount}
                        className="h-9 cursor-pointer rounded-lg bg-green-500 text-sm font-semibold text-black transition hover:bg-green-400 active:scale-95 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {submitting ? "Adding…" : "Add fund"}
                    </button>

                    {notice && (
                        <p className={`text-xs ${notice.kind === "ok" ? "text-green-400" : "text-red-400"}`}>
                            {notice.text}
                        </p>
                    )}
                </div>
            )}
        </div>
    );
}
