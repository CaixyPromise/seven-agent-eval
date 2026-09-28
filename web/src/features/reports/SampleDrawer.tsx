import Card from "antd/es/card";
import Collapse from "antd/es/collapse";
import type { CaseResult } from "../../domain/types";
import { metricLabel, pretty, reasonNames } from "../../domain/presentation";
import { Dialog } from "../../components/Dialog";
import { StatusBadge } from "../../components/StatusBadge";
import { JsonDetails } from "../../components/JsonDetails";
import { RetrievalEvidence } from "./RetrievalEvidence";
export function SampleDrawer({
  sample,
  onClose,
}: {
  sample: CaseResult;
  onClose: () => void;
}) {
  const references=sample.case.references as {qrels?: Record<string,number>} | undefined;
  return (
    <Dialog title={sample.case.id} drawer onClose={onClose}>
      <div className="drawer-body">
        <StatusBadge status={sample.status} />
        {sample.case.metadata?.episode_id ? <p>Episode：{sample.case.metadata.episode_id} · Turn：{sample.case.metadata.turn_id}</p> : null}
        {sample.redaction_applied ? (
          <p className="muted">展示证据已脱敏，指标基于实际回答计算。</p>
        ) : null}
        <JsonDetails label="输入" value={sample.case.inputs} />
        <JsonDetails label="实际回答" value={sample.result.response} />
        {sample.result.retrieval ? <RetrievalEvidence documents={sample.result.retrieval} relevance={references?.qrels} /> : null}
        {sample.result.citations ? <JsonDetails label="引用与定位" value={sample.result.citations} /> : null}
        {sample.result.tools ? <JsonDetails label="工具调用" value={sample.result.tools} /> : null}
        <Collapse
          className="evidence-details"
          items={[
            {
              key: "evidence",
              label: "参考信息与检索上下文",
              children: (
                <>
                  <JsonDetails
                    label="参考信息"
                    value={sample.case.references}
                  />
                  <JsonDetails
                    label="检索上下文"
                    value={sample.result.contexts}
                  />
                </>
              ),
            },
          ]}
        />
        <section>
          <h3>逐项指标</h3>
          <div className="observation-list">
            {sample.metrics.map((metric, index) => (
              <Card
                size="small"
                key={metric.metric_id + ":" + index}
                title={metricLabel(metric.metric_id,metric.parameters)}
                extra={<StatusBadge status={metric.status} />}
              >
                <p>{pretty(metric.value)}</p>
                {metric.reason ? (
                  <small className="muted">
                    {reasonNames[metric.reason] ?? "详见指标码表"}
                  </small>
                ) : null}
                <Collapse
                  ghost
                  size="small"
                  items={[
                    {
                      key: "identity",
                      label: "指标身份与参数",
                      children: (
                        <JsonDetails
                          value={{
                            metric_id: metric.metric_id,
                            evaluator: metric.evaluator,
                            parameters: metric.parameters,
                          }}
                        />
                      ),
                    },
                  ]}
                />
              </Card>
            ))}
          </div>
        </section>
      </div>
    </Dialog>
  );
}
