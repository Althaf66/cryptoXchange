import {
  ColorType,
  createChart as createLightWeightChart,
  CrosshairMode,
  ISeriesApi,
  UTCTimestamp,
} from "lightweight-charts";

export class ChartManager {
  private candleSeries: ISeriesApi<"Candlestick">;
  private chart: any;

  constructor(
    ref: any,
    initialData: any[],
    layout: { background: string; color: string }
  ) {
    const chart = createLightWeightChart(ref, {
      autoSize: true,
      overlayPriceScales: {
        ticksVisible: true,
        borderVisible: true,
      },
      crosshair: {
        mode: CrosshairMode.Normal,
      },
      rightPriceScale: {
        visible: true,
        ticksVisible: true,
        entireTextOnly: true,
      },
      grid: {
        horzLines: {
          visible: false,
        },
        vertLines: {
          visible: false,
        },
      },
      layout: {
        background: {
          type: ColorType.Solid,
          color: layout.background,
        },
        textColor: "white",
      },
    });
    this.chart = chart;
    this.candleSeries = chart.addCandlestickSeries();

    this.setCandles(initialData);
  }

  /**
   * Replaces the whole series. Cheap at demo volume (<=100 candles) and it
   * handles both a new bucket appearing and the current bucket changing,
   * without tearing down the chart.
   */
  public setCandles(data: { timestamp: number; open: number; high: number; low: number; close: number }[]) {
    this.candleSeries.setData(
      data.map(({ timestamp, open, high, low, close }) => ({
        time: Math.floor(timestamp / 1000) as UTCTimestamp,
        open,
        high,
        low,
        close,
      }))
    );
  }
  /**
   * Appends a candle, or replaces the last one when the timestamp matches.
   * Used to fold live trades into the bucket currently forming.
   */
  public updateCandle(candle: {
    timestamp: number;
    open: number;
    high: number;
    low: number;
    close: number;
  }) {
    const { timestamp, open, high, low, close } = candle;
    this.candleSeries.update({
      time: Math.floor(timestamp / 1000) as UTCTimestamp,
      open,
      high,
      low,
      close,
    });
  }
  public destroy() {
    this.chart.remove();
  }
}
