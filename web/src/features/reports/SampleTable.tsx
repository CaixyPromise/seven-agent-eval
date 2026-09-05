import { useState } from "react";
import Button from "antd/es/button";
import Checkbox from "antd/es/checkbox";
import Tag from "antd/es/tag";
import Table, { type ColumnsType } from "antd/es/table";
import type { CaseResult } from "../../domain/types";
import { isAnomaly, pretty } from "../../domain/presentation";
import { StatusBadge } from "../../components/StatusBadge";
export function SampleTable({
  samples,
  onOpen,
}: {
  samples: CaseResult[];
  onOpen: (sample: CaseResult) => void;
}) {
  const [onlyAnomalies, setOnlyAnomalies] = useState(false),
    [page, setPage] = useState(1);
  const filtered = onlyAnomalies ? samples.filter(isAnomaly) : samples;
  const columns: ColumnsType<CaseResult> = [
    {
      title: "样本",
      key: "id",
      minWidth: 180,
      render: (_, sample) => (
        <>
          <Button
            type="text"
            className="identity-button"
            onClick={() => onOpen(sample)}
          >
            {sample.case.id}
          </Button>
          {isAnomaly(sample) ? (
            <div>
              <Tag color="gold" variant="filled">
                需关注
              </Tag>
            </div>
          ) : null}
        </>
      ),
    },
    {
      title: "执行状态",
      dataIndex: "status",
      width: 110,
      render: (status) => <StatusBadge status={status} />,
    },
    {
      title: "回答预览",
      key: "response",
      minWidth: 220,
      render: (_, sample) => (
        <div className="answer-preview">{pretty(sample.result.response)}</div>
      ),
    },
    {
      title: "有效指标",
      key: "metrics",
      width: 95,
      render: (_, sample) =>
        sample.metrics.filter((m) => m.status === "observed").length,
    },
    {
      title: "操作",
      key: "action",
      width: 115,
      render: (_, sample) => (
        <Button
          type="link"
          aria-label={"查看样本 " + sample.case.id}
          onClick={() => onOpen(sample)}
        >
          查看证据
        </Button>
      ),
    },
  ];
  return (
    <>
      <div className="sample-toolbar">
        <span>{samples.length} 条样本</span>
        <Checkbox
          checked={onlyAnomalies}
          onChange={(e) => {
            setOnlyAnomalies(e.target.checked);
            setPage(1);
          }}
        >
          只看异常
        </Checkbox>
      </div>
      <Table
        className="domain-table"
        rowKey={(sample) => sample.case.id}
        columns={columns}
        dataSource={filtered}
        scroll={{ x: 740 }}
        locale={{ emptyText: onlyAnomalies ? "没有异常样本" : "尚无样本结果" }}
        pagination={{
          current: Math.min(page, Math.max(1, Math.ceil(filtered.length / 10))),
          pageSize: 10,
          showSizeChanger: false,
          onChange: setPage,
          showTotal: (total) => "共 " + total + " 条",
        }}
      />
    </>
  );
}
