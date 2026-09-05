import { useEffect, useRef } from "react";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsCoreOption } from "echarts/core";

echarts.use([
  BarChart,
  CanvasRenderer,
  GridComponent,
  LegendComponent,
  LineChart,
  TooltipComponent,
]);

export function LiveChart({
  ariaLabel,
  option,
  onPoint,
}: {
  ariaLabel: string;
  option: EChartsCoreOption;
  onPoint?: (index: number) => void;
}) {
  const node = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!node.current) return;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const chart = echarts.init(node.current, undefined, { renderer: "canvas" });
    chart.setOption({ ...option, animation: !reduceMotion });
    const resize = new ResizeObserver(() => chart.resize());
    resize.observe(node.current);
    const click = (event: { dataIndex?: number }) => {
      if (typeof event.dataIndex === "number") onPoint?.(event.dataIndex);
    };
    chart.on("click", click);
    return () => {
      resize.disconnect();
      chart.off("click", click);
      chart.dispose();
    };
  }, [onPoint, option]);
  return <div className="live-chart" role="img" aria-label={ariaLabel} ref={node} />;
}
