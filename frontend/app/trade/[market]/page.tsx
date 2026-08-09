"use client";
import { Balances } from "@/app/components/Balances";
import { MarketBar } from "@/app/components/MarketBar";
import { OpenOrders } from "@/app/components/OpenOrders";
import { SwapUI } from "@/app/components/SwapUI";
import { Tour } from "@/app/components/Tour";
import { TradeView } from "@/app/components/TradeView";
import { Trades } from "@/app/components/Trades";
import { Depth } from "@/app/components/depth/Depth";
import { useParams } from "next/navigation";
import { useState } from "react";

export default function Page() {
    const { market } = useParams();
    // Lifted so clicking a level in the order book fills the price in SwapUI.
    const [prefillPrice, setPrefillPrice] = useState<string>();
    // Bumped on cancel so Depth re-fetches the book from the engine.
    const [depthNonce, setDepthNonce] = useState(0);

    return <div className="flex flex-col lg:flex-row flex-1 text-white">
        <div className="flex flex-col flex-1 min-w-0">
            <MarketBar market={market as string} />
            {/* On desktop a fixed height stops the row being sized by whichever
                column has the most rows; each panel scrolls within it. Sized to
                fit the 15-per-side book (~548px) and 23 trades (~546px), with
                ~10px slack so sub-pixel rounding can't clip a row.
                Stacked below lg, the height is released and each panel carries
                its own - Depth and Trades are flex-1/min-h-0 inside, so an
                auto-height parent would collapse them to nothing. */}
            <div className="flex flex-col lg:flex-row lg:h-[560px] border-y border-slate-800">
                <div data-tour="chart" className="flex flex-col flex-1 min-w-0">
                    <TradeView market={market as string} />
                </div>
                <div data-tour="depth" className="flex flex-col w-full h-[360px] lg:h-auto lg:w-[250px] overflow-hidden border-t lg:border-t-0 lg:border-l border-slate-800">
                    <Depth market={market as string} onPriceClick={setPrefillPrice} refreshKey={depthNonce} />
                </div>
                <div data-tour="trades" className="flex flex-col w-full h-[360px] lg:h-auto lg:w-[220px] overflow-hidden border-t lg:border-t-0 lg:border-l border-slate-800">
                    <Trades market={market as string} />
                </div>
            </div>
            <div data-tour="orders">
                <OpenOrders market={market as string} onCancelled={() => setDepthNonce((n) => n + 1)} />
            </div>
        </div>
        <div className="hidden lg:flex w-[10px] flex-col border-slate-800 border-l"></div>
        <div className="border-t lg:border-t-0 border-slate-800">
            <div data-tour="swap" className="flex flex-col w-full lg:w-[280px]">
                <SwapUI market={market as string} prefillPrice={prefillPrice} />
                <Balances market={market as string} />
            </div>
        </div>
        <Tour />
    </div>
}
