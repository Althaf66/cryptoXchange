"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { createVirtualUser } from "../utils/httpClient";
import { armTour } from "./Tour";
import { useUser } from "./UserContext";

/** What a new account starts with, in every asset. */
const DEFAULT_AMOUNT = "100000";

type Notice = { kind: "ok" | "error"; text: string };

/**
 * Demo accounts live in the engine, not in a users table - creating one mints an
 * id and credits the amount to USD and every market's base asset alike.
 */
export function VirtualUsers() {
    const { setUserId, refreshUsers } = useUser();
    const router = useRouter();

    const [name, setName] = useState("");
    const [createAmount, setCreateAmount] = useState(DEFAULT_AMOUNT);
    const [creating, setCreating] = useState(false);
    const [createNotice, setCreateNotice] = useState<Notice | null>(null);

    const create = async () => {
        setCreating(true);
        setCreateNotice(null);
        try {
            const user = await createVirtualUser(name.trim(), createAmount);
            await refreshUsers();
            // Switch to the account just made, so the next click trades as them.
            setUserId(user.id);
            setName("");
            setCreateAmount(DEFAULT_AMOUNT);
            // Hand off to the market picker - the account is only useful once a
            // market is chosen, so don't make them find the next step.
            armTour();
            router.push("/markets");
        } catch (err: any) {
            setCreateNotice({ kind: "error", text: err.message });
        } finally {
            setCreating(false);
        }
    };

    return (
        <div className="w-full rounded-xl border border-baseBorderLight bg-baseBackgroundL1 p-5">
            <p className="text-sm font-medium text-white">Start with a demo account</p>
            <p className="pt-1 text-xs text-slate-400">
                The amount is credited to every asset - USD and each market&apos;s base alike.
            </p>

            <div className="flex flex-col gap-3 pt-4">
                <Field label="Name" value={name} placeholder="Alice" onChange={setName} />
                <Field
                    label="Starting amount (optional)"
                    value={createAmount}
                    placeholder="0"
                    onChange={setCreateAmount}
                    numeric
                />
                <Button onClick={create} disabled={creating || !name.trim()}>
                    {creating ? "Creating…" : "Create account & start trading"}
                </Button>
                <NoticeText notice={createNotice} />
            </div>
        </div>
    );
}

function Field({
    label,
    value,
    placeholder,
    onChange,
    numeric,
}: {
    label: string;
    value: string;
    placeholder: string;
    onChange: (v: string) => void;
    numeric?: boolean;
}) {
    return (
        <div className="flex flex-col gap-2">
            <p className="text-xs font-normal text-slate-400">{label}</p>
            <input
                type="text"
                inputMode={numeric ? "decimal" : "text"}
                value={value}
                placeholder={placeholder}
                onChange={(e) => {
                    const next = e.target.value;
                    // Digits and a single decimal point only, as in SwapUI.
                    if (!numeric || next === "" || /^\d*\.?\d*$/.test(next)) onChange(next);
                }}
                className="h-10 rounded-lg border-2 border-solid border-slate-700 bg-[var(--background)] px-3 text-sm text-white placeholder-slate-500 transition focus:border-blue-500 focus:ring-0"
            />
        </div>
    );
}

function Button({
    onClick,
    disabled,
    children,
}: {
    onClick: () => void;
    disabled: boolean;
    children: React.ReactNode;
}) {
    return (
        <button
            type="button"
            onClick={onClick}
            disabled={disabled}
            className="h-10 rounded-lg bg-greenPrimaryButtonBackground px-4 text-sm font-semibold text-greenPrimaryButtonText active:scale-98 disabled:opacity-50"
        >
            {children}
        </button>
    );
}

function NoticeText({ notice }: { notice: Notice | null }) {
    if (!notice) return null;
    return (
        <p className={`text-xs ${notice.kind === "ok" ? "text-green-400" : "text-red-400"}`}>
            {notice.text}
        </p>
    );
}
