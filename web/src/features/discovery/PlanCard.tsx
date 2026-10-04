import Card from "antd/es/card";
import Button from "antd/es/button";
import Tag from "antd/es/tag";
import { ClipboardList, Play } from "lucide-react";
import type { Asset } from "../../domain/types";
import { recipeName } from "../../domain/presentation";
export function PlanCard({
  asset,
  onInspect,
  onRun,
  busy,
}: {
  asset: Asset;
  onInspect: () => void;
  onRun: () => void;
  busy: boolean;
}) {
  return (
    <Card className="plan-card">
      <div className="plan-card-content">
        <div className="plan-icon">
          <ClipboardList size={24} />
        </div>
        <div className="plan-content">
          <div className="plan-title">
            <Button type="text" className="plan-name" onClick={onInspect}>
              {asset.id}
            </Button>
            <Tag color="blue" variant="filled">
              {asset.descriptor?.candidate_count?`${asset.descriptor.candidate_count} 个候选比较`:asset.descriptor?.mode === "invoke" ? "调用目标" : "已有回答"}
            </Tag>
          </div>
          <p>
            <span>{asset.descriptor?.source ? "数据源" : "数据集"}</span>{" "}
            {String(asset.descriptor?.source ?? asset.descriptor?.dataset ?? "未指定").replace(
              /^(dataset|source):/,
              "",
            )}
          </p>
          <p className="recipe">{recipeName(asset)}</p>
          {asset.descriptor?.candidate_count?<p className="muted">每个候选会重新调用目标，使用同一批案例。</p>:null}
          <div className="plan-actions">
            <Button
              type="primary"
              icon={<Play size={14} />}
              onClick={onRun}
              loading={busy}
              aria-label={"运行评测 " + asset.id}
            >
              运行评测
            </Button>
          </div>
        </div>
      </div>
    </Card>
  );
}
