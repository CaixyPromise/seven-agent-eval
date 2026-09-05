import Button from "antd/es/button";
import Descriptions from "antd/es/descriptions";
import Alert from "antd/es/alert";
import type { Asset } from "../../domain/types";
import { assetKinds, recipeName } from "../../domain/presentation";
import { diagnosticCopy } from "../../api/errors";
import { Dialog } from "../../components/Dialog";
import { StatusBadge } from "../../components/StatusBadge";
export function AssetDrawer({
  asset,
  onClose,
  onRun,
  busy,
}: {
  asset: Asset;
  onClose: () => void;
  onRun: (asset: Asset) => void;
  busy: boolean;
}) {
  const descriptor = asset.descriptor;
  const items = [
    {
      key: "type",
      label: "类型",
      children: assetKinds[asset.kind] ?? asset.kind,
    },
    {
      key: "source",
      label: "来源",
      children: asset.source === "builtin" ? "内置" : "项目",
    },
    { key: "version", label: "版本", children: asset.version },
    { key: "path", label: "相对位置", children: asset.path },
  ];
  if (asset.kind === "plan")
    items.push(
      {
        key: "mode",
        label: "评测方式",
        children: descriptor?.mode === "invoke" ? "调用目标" : "已有回答",
      },
      {
        key: "dataset",
        label: "数据集",
        children: String(descriptor?.dataset ?? "未指定"),
      },
      { key: "recipe", label: "评测配方", children: recipeName(asset) },
    );
  return (
    <Dialog title={asset.id} drawer onClose={onClose}>
      <div className="drawer-body">
        <StatusBadge status={asset.status} />
        {asset.status === "invalid" ? (
          <Alert
            className="feedback"
            type="warning"
            showIcon
            title={
              diagnosticCopy[asset.diagnostic_code ?? ""] ??
              diagnosticCopy.ASSET_INVALID
            }
          />
        ) : null}
        <Descriptions column={1} items={items} />
        {asset.kind === "plan" && asset.status === "valid" ? (
          <Button type="primary" loading={busy} onClick={() => onRun(asset)}>
            运行评测
          </Button>
        ) : null}
      </div>
    </Dialog>
  );
}
