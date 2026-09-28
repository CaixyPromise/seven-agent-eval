import Button from "antd/es/button";
import Card from "antd/es/card";
import Statistic from "antd/es/statistic";
import { useState } from "react";
import { RefreshCw } from "lucide-react";
import type { Asset, Discovery } from "../../domain/types";
import type { ErrorCode } from "../../api/errors";
import {
  ErrorNotice,
  LoadingState,
  EmptyState,
} from "../../components/Feedback";
import { PlanCard } from "./PlanCard";
import { AssetTable } from "./AssetTable";
import { AssetDrawer } from "./AssetDrawer";
export default function DiscoveryPage({
  discovery,
  loading,
  error,
  onRefresh,
  onRun,
  busy,
}: {
  discovery: Discovery | null;
  loading: boolean;
  error: ErrorCode | null;
  onRefresh: () => void;
  onRun: (asset: Asset) => void;
  busy: boolean;
}) {
  const [selected, setSelected] = useState<Asset | null>(null);
  const assets = discovery?.assets ?? [];
  const plans = assets.filter((a) => a.kind === "plan" && a.status === "valid");
  const stats = [
    ["可运行计划", plans.length],
    ["数据集与数据源", assets.filter((a) => a.kind === "dataset" || a.kind === "source").length],
    [
      "评测扩展",
      assets.filter((a) =>
        ["evaluator", "processor", "conclusion"].includes(a.kind),
      ).length,
    ],
    ["待处理资产", assets.filter((a) => a.status === "invalid").length],
  ];
  return (
    <>
      <header className="page-heading">
        <div>
          <h1>评测计划</h1>
          <p>选择计划开始评测，或查看已有结果。</p>
        </div>
        <Button onClick={onRefresh} disabled={loading}>
          <RefreshCw size={16} className={loading ? "spin" : ""} />
          {loading ? "扫描中…" : "重新扫描"}
        </Button>
      </header>
      {error ? <ErrorNotice code={error} onRetry={onRefresh} /> : null}
      {!discovery && loading ? (
        <LoadingState label="正在发现评测资产" />
      ) : discovery ? (
        <>
          <div className="summary-grid">
            {stats.map(([label, value]) => (
              <Card className="summary-card" key={label}>
                <Statistic title={label} value={value} />
              </Card>
            ))}
          </div>
          <section className="plans-section">
            <h2>可运行计划</h2>
            {plans.length ? (
              <div className="plan-grid">
                {plans.map((asset) => (
                  <PlanCard
                    key={asset.id}
                    asset={asset}
                    onInspect={() => setSelected(asset)}
                    onRun={() => onRun(asset)}
                    busy={busy}
                  />
                ))}
              </div>
            ) : (
              <EmptyState
                title="暂无可运行计划"
                description={
                  discovery.package.id === "viewer"
                    ? "当前为结果查看器，可导入已有结果。"
                    : "在评测目录中添加计划后重新扫描。"
                }
              />
            )}
          </section>
          <AssetTable assets={assets} onInspect={setSelected} />
        </>
      ) : null}
      {selected ? (
        <AssetDrawer
          asset={selected}
          onClose={() => setSelected(null)}
          onRun={(asset) => {
            setSelected(null);
            onRun(asset);
          }}
          busy={busy}
        />
      ) : null}
    </>
  );
}
