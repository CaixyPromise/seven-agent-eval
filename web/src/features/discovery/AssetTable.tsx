import { useState } from "react";
import Button from "antd/es/button";
import Input from "antd/es/input";
import Segmented from "antd/es/segmented";
import Table, { type ColumnsType } from "antd/es/table";
import { Search } from "lucide-react";
import type { Asset } from "../../domain/types";
import { assetKinds } from "../../domain/presentation";
import { StatusBadge } from "../../components/StatusBadge";
const filters = [
  ["all", "全部"],
  ["dataset", "数据集"],
  ["adapter", "适配器"],
  ["evaluator", "评测器"],
  ["processor", "处理器"],
  ["conclusion", "结论"],
  ["invalid", "需处理"],
].map(([value, label]) => ({ value, label }));
export function AssetTable({
  assets,
  onInspect,
}: {
  assets: Asset[];
  onInspect: (asset: Asset) => void;
}) {
  const [filter, setFilter] = useState("all"),
    [query, setQuery] = useState(""),
    [page, setPage] = useState(1);
  const needle = query.trim().toLocaleLowerCase();
  const filtered = assets.filter(
    (a) =>
      (filter === "all" ||
        a.kind === filter ||
        (filter === "invalid" && a.status === "invalid")) &&
      [a.id, assetKinds[a.kind] ?? a.kind, a.path].some((v) =>
        v.toLocaleLowerCase().includes(needle),
      ),
  );
  const columns: ColumnsType<Asset> = [
    {
      title: "资产名称",
      key: "id",
      minWidth: 240,
      render: (_, asset) => (
        <Button
          type="text"
          className="identity-button"
          onClick={() => onInspect(asset)}
        >
          {asset.id}
        </Button>
      ),
    },
    {
      title: "类型",
      dataIndex: "kind",
      width: 110,
      render: (kind) => assetKinds[kind] ?? kind,
    },
    {
      title: "来源",
      dataIndex: "source",
      width: 100,
      render: (source) => (source === "builtin" ? "内置" : "项目"),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 115,
      render: (status) => <StatusBadge status={status} />,
    },
    {
      title: "操作",
      key: "action",
      width: 90,
      render: (_, asset) => (
        <Button
          type="link"
          onClick={() => onInspect(asset)}
          aria-label={"查看资产 " + asset.id}
        >
          查看
        </Button>
      ),
    },
  ];
  return (
    <section className="panel">
      <div className="panel-toolbar">
        <h2>
          全部资产 <span className="count">{assets.length}</span>
        </h2>
        <div className="filter-scroll">
          <Segmented
            aria-label="资产类型"
            value={filter}
            options={filters}
            onChange={(value) => {
              setFilter(value);
              setPage(1);
            }}
          />
        </div>
        <Input
          className="search-field"
          prefix={<Search size={16} />}
          allowClear
          aria-label="搜索资产"
          placeholder="搜索资产"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(1);
          }}
        />
      </div>
      <Table<Asset>
        className="domain-table"
        columns={columns}
        dataSource={filtered}
        rowKey={(a) => a.kind + ":" + a.source + ":" + a.path}
        scroll={{ x: 680 }}
        locale={{ emptyText: "没有匹配的资产" }}
        pagination={{
          current: Math.min(page, Math.max(1, Math.ceil(filtered.length / 8))),
          pageSize: 8,
          showSizeChanger: false,
          onChange: setPage,
          showTotal: (total) => "共 " + total + " 项",
        }}
      />
    </section>
  );
}
