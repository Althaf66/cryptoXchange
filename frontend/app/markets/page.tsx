import { Markets } from "../components/Markets";

export default function Page() {
    return <main className="flex min-h-screen flex-col px-4 py-12 sm:px-8 lg:px-12">
        <div className="mx-auto w-full max-w-[1280px]">
            <h1 className="text-[1.75rem] font-semibold text-white">Choose a market</h1>
            <p className="pb-6 pt-2 text-sm text-slate-400">
                Every market runs on the same live matching engine. Pick one to see its
                order book, depth and candles - then place your first order.
            </p>
            <Markets />
        </div>
    </main>
}
