import { Markets } from "./components/Markets";
import { VirtualUsers } from "./components/VirtualUsers";

const STEPS = [
  {
    title: "Create a virtual account",
    body: "Name it, and it's funded instantly - 100,000 of USD and every market's base asset.",
  },
  {
    title: "Open a market",
    body: "Live order book, depth chart and candles, streamed over websockets as trades happen.",
  },
  {
    title: "Place an order",
    body: "Limit or market, buy or sell. Watch it hit the book and match in real time.",
  },
];

export default function Home() {
  return (
    <main className="flex min-h-screen flex-col gap-16 px-4 py-12 sm:px-8 lg:px-12">
      <section className="mx-auto grid w-full max-w-[1280px] items-center gap-10 md:grid-cols-2">
        <div>
          <h1 className="text-[2.25rem] font-semibold leading-[1.1] text-white sm:text-[3rem]">
            Trade crypto.
            <br />
            <span className="text-greenPrimaryButtonBackground">Risk nothing.</span>
          </h1>
          <p className="max-w-md pt-4 text-sm leading-6 text-slate-400">
            A real limit-order matching engine with a live order book, depth and
            candles funded entirely with vitual money. No signup, no wallet, no
            way to lose a cent.
          </p>
          <div className="flex flex-wrap gap-2 pt-6">
            {["Real matching engine", "Live websocket feed", "Virtual money only"].map((chip) => (
              <span
                key={chip}
                className="rounded-full border border-baseBorderLight px-3 py-1 text-xs text-slate-400"
              >
                {chip}
              </span>
            ))}
          </div>
        </div>
        <VirtualUsers />
      </section>

      <section className="mx-auto grid w-full max-w-[1280px] gap-4 sm:grid-cols-3">
        {STEPS.map((step, i) => (
          <div
            key={step.title}
            className="rounded-xl border border-baseBorderLight bg-baseBackgroundL1 p-5"
          >
            <span className="flex h-7 w-7 items-center justify-center rounded-full bg-greenBackgroundTransparent text-xs font-semibold text-greenPrimaryButtonBackground">
              {i + 1}
            </span>
            <p className="pt-3 text-sm font-medium text-white">{step.title}</p>
            <p className="pt-1 text-xs leading-5 text-slate-400">{step.body}</p>
          </div>
        ))}
      </section>

      <section className="mx-auto w-full max-w-[1280px]">
        <h2 className="text-[1.25rem] font-semibold text-white">Live markets</h2>
        <p className="pb-4 pt-1 text-xs text-slate-400">
          Click any row to open its trading view.
        </p>
        <Markets />
      </section>
    </main>
  );
}
