import Card from "antd/es/card";
import Table, { type ColumnsType } from "antd/es/table";
import Collapse from "antd/es/collapse";
import Empty from "antd/es/empty";
import { EvaluationChart } from "../../components/EvaluationChart";
import type { RagQuality } from "../../domain/types";

const metrics = ["precision", "recall", "f1"] as const;
const names = { precision: "Precision", recall: "Recall", f1: "F1 Score" };

export default function RagQualityChart({ groups }: { groups: RagQuality[] }) {
  const columns: ColumnsType<RagQuality["points"][number]> = [
    { title: "检索 K", dataIndex: "k", width: 90 },
    ...metrics.map((metric) => ({
      title: names[metric], key: metric, width: 180,
      render: (_: unknown, point: RagQuality["points"][number]) => (
        <>
          {point[metric] == null ? "—" : point[metric].toFixed(4)}
          <small className="secondary-id">
            有效观测 {point.counts[metric]?.observed ?? 0} / {point.counts[metric]?.total ?? 0}
          </small>
        </>
      ),
    })),
  ];
  return (
    <Card className="rag-quality-panel" title="检索质量随 K 的变化" size="small">
      <p className="muted">横轴为检索 K，纵轴为有效样本的均值（0–1）。F1 先逐样本计算再取均值；缺失值以断点表示。</p>
      {groups.map((group) => (
        <section key={group.id}>
          <Collapse ghost size="small" items={[{key: "identity", label: `${group.evaluator} · 评测配置`, children: <pre>{JSON.stringify(group.parameters, null, 2)}</pre>}]} />
          {group.points.some((point) => metrics.some((metric) => point[metric] != null)) ? (
            <EvaluationChart
              ariaLabel="Precision、Recall 和 F1 随检索 K 的变化；下方表格提供精确值与有效观测数"
              option={{
                color: ["#1677ff", "#13a8a8", "#722ed1"],
                tooltip: { trigger: "axis", renderMode: "richText" },
                legend: { top: 0, data: metrics.map((metric) => names[metric]) },
                grid: { left: 42, right: 24, top: 42, bottom: 42 },
                xAxis: { type: "category", name: "K", data: group.points.map((point) => String(point.k)) },
                yAxis: { type: "value", min: 0, max: 1 },
                series: metrics.map((metric) => ({
                  name: names[metric], type: "line", smooth: false, connectNulls: false,
                  symbolSize: 8, data: group.points.map((point) => point[metric]),
                })),
              }}
            />
          ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有可计算的检索指标，请查看样本的参考与检索证据。" />}
          <Table rowKey="k" columns={columns} dataSource={group.points} pagination={false} scroll={{ x: 630 }} size="small" />
        </section>
      ))}
    </Card>
  );
}
