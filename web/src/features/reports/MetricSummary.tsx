import Table, { type ColumnsType } from "antd/es/table";
import Collapse from "antd/es/collapse";
import type { Series } from "../../domain/types";
import { metricNames, pretty } from "../../domain/presentation";
export function MetricSummary({ series }: { series: Series[] }) {
  const columns: ColumnsType<Series> = [
    {
      title: "指标",
      key: "metric",
      minWidth: 220,
      render: (_, item) => (
        <>
          <strong>{metricNames[item.metric_id] ?? item.metric_id}</strong>
          <small className="secondary-id">{item.metric_id}</small>
        </>
      ),
    },
    {
      title: "汇总值",
      key: "value",
      width: 140,
      render: (_, item) => (
        <div className="metric-value">
          {item.rate != null
            ? (item.rate * 100).toFixed(1) + "%"
            : item.mean != null
              ? item.mean.toFixed(1)
              : "—"}
          {item.p95 != null ? (
            <small className="secondary-id">p95 {item.p95.toFixed(1)}</small>
          ) : null}
        </div>
      ),
    },
    {
      title: "有效观测 / 样本",
      key: "coverage",
      width: 150,
      render: (_, item) => item.observed + " / " + item.total,
    },
    {
      title: "评测配置",
      key: "config",
      width: 180,
      render: (_, item) => (
        <Collapse
          ghost
          size="small"
          items={[
            {
              key: "config",
              label: "查看配置",
              children: (
                <pre>
                  {pretty({
                    evaluator: item.evaluator,
                    parameters: item.parameters,
                  })}
                </pre>
              ),
            },
          ]}
        />
      ),
    },
  ];
  return (
    <section className="panel">
      <div className="panel-toolbar">
        <h2>指标汇总</h2>
        <span className="muted">仅汇总可计算的观测</span>
      </div>
      <Table
        className="domain-table"
        rowKey={(item) =>
          JSON.stringify([item.metric_id, item.evaluator, item.parameters])
        }
        columns={columns}
        dataSource={series}
        pagination={false}
        scroll={{ x: 690 }}
      />
    </section>
  );
}
