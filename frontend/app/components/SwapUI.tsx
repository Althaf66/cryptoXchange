"use client";
import { useEffect, useState } from "react";
import { formatAmount, formatPrecise } from "../utils/format";
import { createOrder, getBalances } from "../utils/httpClient";
import { Balances } from "../utils/types";
import { useUser } from "./UserContext";

type Side = "buy" | "sell";
type OrderType = "limit" | "market";

export function SwapUI({ market, prefillPrice }: { market: string; prefillPrice?: string }) {
    const { userId } = useUser();
    const [baseAsset, quoteAsset] = market.split("_");

    const [price, setPrice] = useState("");
    const [quantity, setQuantity] = useState("");
    const [side, setSide] = useState<Side>("buy");
    const [type, setType] = useState<OrderType>("limit");
    const [balances, setBalances] = useState<Balances>({});
    const [submitting, setSubmitting] = useState(false);
    const [notice, setNotice] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

    const refreshBalances = () => {
        getBalances(userId).then(setBalances).catch(() => setBalances({}));
    };

    // Wrapped: refreshBalances returns a promise, and an effect's return value
    // is treated as its cleanup function.
    useEffect(() => {
        refreshBalances();
    }, [userId]);

    // Clicking a level in the order book drops that price into the form.
    useEffect(() => {
        if (prefillPrice) setPrice(prefillPrice);
    }, [prefillPrice]);

    // Buying spends the quote asset, selling spends the base asset.
    const spendAsset = side === "buy" ? quoteAsset : baseAsset;
    const available = balances[spendAsset]?.available ?? 0;
    const locked = balances[spendAsset]?.locked ?? 0;

    const orderValue = Number(price || 0) * Number(quantity || 0);

    const setQuantityFromPercent = (percent: number) => {
        if (side === "sell") {
            setQuantity(formatPrecise(available * percent));
            return;
        }
        const p = Number(price);
        if (!p) return;
        setQuantity(formatPrecise((available * percent) / p));
    };

    const submit = async () => {
        setNotice(null);

        const qty = Number(quantity);
        if (!qty || qty <= 0) {
            setNotice({ kind: "error", text: "Enter a quantity greater than 0" });
            return;
        }
        if (type === "limit" && !(Number(price) > 0)) {
            setNotice({ kind: "error", text: "Enter a price greater than 0" });
            return;
        }

        setSubmitting(true);
        try {
            const result = await createOrder({
                market,
                // Market orders are repriced by the engine; send 0 as a placeholder.
                price: type === "market" ? "0" : price,
                quantity,
                side,
                type,
                userId,
            });

            const filled = result.executedQty ?? 0;
            if (filled > 0) {
                const avg =
                    (result.fills ?? []).reduce((sum, f) => sum + Number(f.price) * f.qty, 0) / filled;
                setNotice({
                    kind: "ok",
                    text: `Filled ${formatAmount(filled)} ${baseAsset} @ ${avg.toFixed(2)} ${quoteAsset}`,
                });
            } else {
                setNotice({ kind: "ok", text: `Order resting on the book (${result.orderId})` });
            }
            setQuantity("");
            refreshBalances();
        } catch (err: any) {
            setNotice({ kind: "error", text: err.message });
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="flex flex-col">
            <div className="flex flex-row h-[60px]">
                <TabButton active={side === "buy"} onClick={() => setSide("buy")} label="Buy" tone="green" />
                <TabButton active={side === "sell"} onClick={() => setSide("sell")} label="Sell" tone="red" />
            </div>

            <div className="flex flex-col gap-1">
                <div className="px-3">
                    <div className="flex flex-row flex-0 gap-5">
                        <TypeButton active={type === "limit"} onClick={() => setType("limit")} label="Limit" />
                        <TypeButton active={type === "market"} onClick={() => setType("market")} label="Market" />
                    </div>
                </div>

                <div className="flex flex-col px-3 gap-3">
                    <div className="flex items-center justify-between flex-row pt-2">
                        <p className="text-xs font-normal text-slate-400">Available</p>
                        <p className="font-medium text-xs text-white tabular-nums">
                            {formatAmount(available)} {spendAsset}
                            {locked > 0 && <span className="text-slate-500"> ({formatAmount(locked)} locked)</span>}
                        </p>
                    </div>

                    <Field
                        label="Price"
                        value={type === "market" ? "" : price}
                        placeholder={type === "market" ? "Market" : "0"}
                        disabled={type === "market"}
                        onChange={setPrice}
                        suffix={quoteAsset}
                    />

                    <Field
                        label="Quantity"
                        value={quantity}
                        placeholder="0"
                        onChange={setQuantity}
                        suffix={baseAsset}
                    />

                    <div className="flex flex-row gap-2">
                        {[0.25, 0.5, 0.75, 1].map((p) => (
                            <button
                                key={p}
                                type="button"
                                onClick={() => setQuantityFromPercent(p)}
                                className="flex-1 rounded-lg bg-slate-800 py-1 text-xs text-slate-300 hover:bg-slate-700"
                            >
                                {p * 100}%
                            </button>
                        ))}
                    </div>

                    <div className="flex justify-between flex-row">
                        <p className="text-xs text-slate-400">Order value</p>
                        <p className="text-xs text-white tabular-nums">
                            {type === "market" ? "-" : `${orderValue.toFixed(2)} ${quoteAsset}`}
                        </p>
                    </div>

                    <button
                        type="button"
                        onClick={submit}
                        disabled={submitting}
                        className={`font-semibold text-center h-12 rounded-xl text-base px-4 py-2 my-2 active:scale-98 disabled:opacity-50 ${
                            side === "buy" ? "bg-green-500 text-black" : "bg-red-500 text-black"
                        }`}
                    >
                        {submitting ? "Placing…" : `${side === "buy" ? "Buy" : "Sell"} ${baseAsset}`}
                    </button>

                    {notice && (
                        <p
                            className={`pb-3 text-xs ${
                                notice.kind === "ok" ? "text-green-400" : "text-red-400"
                            }`}
                        >
                            {notice.text}
                        </p>
                    )}
                </div>
            </div>
        </div>
    );
}

function Field({
    label,
    value,
    placeholder,
    onChange,
    suffix,
    disabled,
}: {
    label: string;
    value: string;
    placeholder: string;
    onChange: (v: string) => void;
    suffix: string;
    disabled?: boolean;
}) {
    return (
        <div className="flex flex-col gap-2">
            <p className="text-xs font-normal text-slate-400">{label}</p>
            <div className="flex flex-col relative">
                <input
                    type="text"
                    inputMode="decimal"
                    value={value}
                    disabled={disabled}
                    placeholder={placeholder}
                    onChange={(e) => {
                        // Digits and a single decimal point only.
                        const next = e.target.value;
                        if (next === "" || /^\d*\.?\d*$/.test(next)) onChange(next);
                    }}
                    className="h-12 rounded-lg border-2 border-solid border-slate-700 bg-[var(--background)] pr-16 pl-3 text-right text-2xl leading-9 text-white placeholder-slate-500 ring-0 transition focus:border-blue-500 focus:ring-0 disabled:opacity-50"
                />
                <div className="absolute right-3 top-4 text-xs text-slate-400">{suffix}</div>
            </div>
        </div>
    );
}

function TypeButton({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
    return (
        <div className="flex flex-col cursor-pointer justify-center py-2" onClick={onClick}>
            <div
                className={`text-sm font-medium py-1 border-b-2 ${
                    active
                        ? "border-blue-500 text-white"
                        : "border-transparent text-slate-400 hover:text-white"
                }`}
            >
                {label}
            </div>
        </div>
    );
}

function TabButton({
    active,
    onClick,
    label,
    tone,
}: {
    active: boolean;
    onClick: () => void;
    label: string;
    tone: "green" | "red";
}) {
    const activeClass =
        tone === "green" ? "border-b-green-500 bg-green-500/10" : "border-b-red-500 bg-red-500/10";
    const textClass = tone === "green" ? "text-green-400" : "text-red-400";
    return (
        <div
            className={`flex flex-col mb-[-2px] flex-1 cursor-pointer justify-center border-b-2 p-4 ${
                active ? activeClass : "border-b-slate-800 hover:border-b-slate-600"
            }`}
            onClick={onClick}
        >
            <p className={`text-center text-sm font-semibold ${textClass}`}>{label}</p>
        </div>
    );
}
