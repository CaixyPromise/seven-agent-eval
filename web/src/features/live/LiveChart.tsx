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
  const instance = useRef<echarts.EChartsType | null>(null);
  const clickHandler = useRef(onPoint);
  clickHandler.current = onPoint;
  useEffect(() => {
    if (!node.current) return;
    const chart = echarts.init(node.current, undefined, { renderer: "canvas" });
    instance.current = chart;
    const preference = window.matchMedia("(prefers-reduced-motion: reduce)");
    const motion = () => chart.setOption({ animation: !preference.matches });
    preference.addEventListener("change", motion);
    const resize = new ResizeObserver(() => chart.resize());
    resize.observe(node.current);
    const click = (event: { dataIndex?: number }) => {
      if (typeof event.dataIndex === "number") clickHandler.current?.(event.dataIndex);
    };
    chart.on("click", click);
    return () => {
      resize.disconnect();
      preference.removeEventListener("change", motion);
      chart.off("click", click);
      chart.dispose();
      instance.current = null;
    };
  }, []);
  useEffect(() => {
    instance.current?.setOption({ ...option, animation: !window.matchMedia("(prefers-reduced-motion: reduce)").matches }, { notMerge: true });
  }, [option]);
  return <div className="live-chart" role="img" aria-label={ariaLabel} ref={node} />;
}
