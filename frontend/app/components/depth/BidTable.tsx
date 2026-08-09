
import { LEVELS_PER_SIDE } from "./AskTable";

export const BidTable = ({ bids, onPriceClick }: {bids: [string, string][], onPriceClick?: (price: string) => void}) => {
    let currentTotal = 0;
    // bids arrive descending (best first) and render in that order, so the best
    // bid sits at the top against the mid price and totals grow downward.
    const relevantBids = bids.slice(0, LEVELS_PER_SIDE);
    const bidsWithTotal: [string, string, number][] = relevantBids.map(([price, quantity]) => [price, quantity, currentTotal += Number(quantity)]);
    const maxTotal = currentTotal;

    return <div>
        {bidsWithTotal?.map(([price, quantity, total]) => <Bid maxTotal={maxTotal} total={total} key={price} price={price} quantity={quantity} onPriceClick={onPriceClick} />)}
    </div>
}

function Bid({ price, quantity, total, maxTotal, onPriceClick }: { price: string, quantity: string, total: number, maxTotal: number, onPriceClick?: (price: string) => void }) {
    return (
        <div
            onClick={() => onPriceClick?.(price)}
            style={{
                display: "flex",
                position: "relative",
                width: "100%",
                backgroundColor: "transparent",
                overflow: "hidden",
                cursor: onPriceClick ? "pointer" : "default",
            }}
        >
        <div
            style={{
            position: "absolute",
            top: 0,
            left: 0,
            width: `${(100 * total) / maxTotal}%`,
            height: "100%",
            background: "rgba(1, 167, 129, 0.325)",
            transition: "width 0.3s ease-in-out",
            }}
        ></div>
            <div className={`flex justify-between text-xs w-full`}>
                <div>
                    {price}
                </div>
                <div>
                    {quantity}
                </div>
                <div>
                    {total.toFixed(2)}
                </div>
            </div>
        </div>
    );
}
