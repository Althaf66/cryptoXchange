
export const LEVELS_PER_SIDE = 15;

export const AskTable = ({ asks, onPriceClick }: { asks: [string, string][], onPriceClick?: (price: string) => void }) => {
    let currentTotal = 0;
    // asks arrive ascending (best first). Accumulate in that order so the level
    // touching the mid price has the smallest total, then reverse once for
    // display: worst ask on top, best ask at the bottom against the mid.
    const relevantAsks = asks.slice(0, LEVELS_PER_SIDE);
    const asksWithTotal: [string, string, number][] = relevantAsks.map(([price, quantity]) => [price, quantity, currentTotal += Number(quantity)]);
    const maxTotal = currentTotal;
    asksWithTotal.reverse();

    // mt-auto bottom-aligns the block so a market with only a few levels still
    // puts its best ask against the mid price instead of leaving a gap. Not
    // justify-end on the parent: that bottom-aligns too, but makes the
    // overflowing top rows unreachable in a scroll container.
    return <div className="mt-auto">
        {asksWithTotal.map(([price, quantity, total]) => <Ask maxTotal={maxTotal} key={price} price={price} quantity={quantity} total={total} onPriceClick={onPriceClick} />)}
    </div>
}
function Ask({price, quantity, total, maxTotal, onPriceClick}: {price: string, quantity: string, total: number, maxTotal: number, onPriceClick?: (price: string) => void}) {
    return <div
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
        background: "rgba(228, 75, 68, 0.325)",
        transition: "width 0.3s ease-in-out",
        }}
    ></div>
    <div className="flex justify-between text-xs w-full">
        <div>
            {price}
        </div>
        <div>
            {quantity}
        </div>
        <div>
            {total?.toFixed(2)}
        </div>
    </div>
    </div>
}