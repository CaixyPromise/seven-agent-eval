import { useState } from "react";
import Button from "antd/es/button";
import Input from "antd/es/input";
import Table, { type ColumnsType } from "antd/es/table";
import { RefreshCw, Search } from "lucide-react";
import type { RunSummary } from "../../domain/types";
import type { ErrorCode } from "../../api/errors";
import { formatDate, isLive } from "../../domain/presentation";
import { reportHref } from "../../app/routes";
import { StatusBadge } from "../../components/StatusBadge";
import {
  EmptyState,
  ErrorNotice,
  LoadingState,
} from "../../components/Feedback";
export default function RunsPage({
  runs,
  loading,
  error,
  onRefresh,
  reports = false,
}: {
  runs: RunSummary[] | null;
  loading: boolean;
  error: ErrorCode | null;
  onRefresh: () => void;
  reports?: boolean;
}) {
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(1);
  const filtered = (runs ?? []).filter(
    (r) =>
      (!reports || !isLive(r.status)) &&
      (r.id + " " + r.package.name).toLowerCase().includes(query.toLowerCase()),
  );
  const columns: ColumnsType<RunSummary> = [
    {
      title: "评测 / 运行编号",
      key: "name",
      minWidth: 280,
      render: (_, run) => (
        <>
          <Button
            type="link"
            className="identity-button"
            href={reportHref(run.id)}
          >
            {run.package.name}
          </Button>
          <small className="secondary-id">{run.id}</small>
        </>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 110,
      render: (status) => <StatusBadge status={status} />,
    },
    {
      title: "已处理样本",
      key: "progress",
      width: 125,
      render: (_, run) => run.completed + " / " + run.total,
    },
    {
      title: "开始时间",
      dataIndex: "created_at",
      width: 150,
      render: formatDate,
    },
    {
      title: "操作",
      key: "action",
      width: 120,
      render: (_, run) => (
        <Button type="link" href={reportHref(run.id)}>
          {isLive(run.status) ? "查看进度" : "查看报告"}
        </Button>
      ),
    },
  ];
  return (
    <>
      <header className="page-heading">
        <div>
          <h1>{reports ? "评测报告" : "运行记录"}</h1>
          <p>
            {reports
              ? "查看已完成或部分完成的评测结果。"
              : "查看进度，或打开一次运行的结果。"}
          </p>
        </div>
        <Button
          loading={loading}
          icon={<RefreshCw size={16} />}
          onClick={onRefresh}
        >
          刷新记录
        </Button>
      </header>
      {error ? <ErrorNotice code={error} onRetry={onRefresh} /> : null}
      {!runs && loading ? (
        <LoadingState />
      ) : (
        <section className="panel">
          <div className="panel-toolbar">
            <h2>
              {reports ? "全部报告" : "全部运行"}{" "}
              <span className="count">{filtered.length}</span>
            </h2>
            <Input
              className="search-field"
              prefix={<Search size={16} />}
              allowClear
              aria-label="搜索运行"
              placeholder="搜索名称或运行编号"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(1);
              }}
            />
          </div>
          <Table
            className="domain-table"
            rowKey="id"
            dataSource={filtered}
            columns={columns}
            scroll={{ x: 800 }}
            loading={loading}
            locale={{
              emptyText: (
                <EmptyState
                  title={query ? "没有匹配的运行" : "暂无评测结果"}
                  action={
                    query ? (
                      <Button onClick={() => setQuery("")}>清除搜索</Button>
                    ) : (
                      <Button type="primary" href="#/plans">
                        选择评测计划
                      </Button>
                    )
                  }
                />
              ),
            }}
            pagination={{
              current: Math.min(
                page,
                Math.max(1, Math.ceil(filtered.length / 10)),
              ),
              pageSize: 10,
              showSizeChanger: false,
              onChange: setPage,
              showTotal: (total) => "共 " + total + " 项",
            }}
          />
        </section>
      )}
    </>
  );
}
