import { lazy, Suspense, useState } from "react";
import Card from "antd/es/card";
import Table, {type ColumnsType} from "antd/es/table";
import Collapse from "antd/es/collapse";
import Tabs from "antd/es/tabs";
import Segmented from "antd/es/segmented";
import type { ComparisonSummary, ComparisonRow, Run } from "../../domain/types";
import { metricLabel, pretty, isLive } from "../../domain/presentation";
import { LoadingState } from "../../components/Feedback";
import { ComparisonSamples } from "./ComparisonSamples";
const ComparisonChart=lazy(()=>import("./ComparisonChart"));
const number=(value:number|null)=>value==null?"—":value.toFixed(4);

export function ComparisonPanel({run,summary}:{run:Run;summary?:ComparisonSummary}) {
  const [metricScope,setMetricScope]=useState("key");
  const rows=summary?.rows.filter(row=>metricScope==="all" || ["rag.f1","rag.precision","rag.recall","latency.ms","usage.cost"].includes(row.metric_id)) ?? [];
  const columns:ColumnsType<ComparisonRow>=[
    {title:"指标与配置",key:"metric",width:260,render:(_,row)=><>{metricLabel(row.metric_id,row.parameters)}<Collapse ghost size="small" items={[{key:"id",label:"指标身份",children:<pre>{pretty({evaluator:row.evaluator,parameters:row.parameters})}</pre>}]}/></>},
    {title:"候选",dataIndex:"candidate_id",width:160},
    {title:"有效均值",dataIndex:"mean",width:120,render:number},
    {title:"有效 / 计划案例",key:"count",width:140,render:(_,row)=>`${row.observed} / ${row.total}`},
    {title:"配对案例",dataIndex:"paired",width:100},
    {title:"配对基线均值",dataIndex:"baseline_mean",width:130,render:number},
    {title:"配对候选均值",dataIndex:"candidate_mean",width:130,render:number},
    {title:"配对差值",dataIndex:"delta",width:120,render:(value:number|null)=>value==null?"—":`${value>0?"+":""}${value.toFixed(4)}`},
  ];
  return <Card className="rag-quality-panel" title="候选参数比较" size="small">
    <p>共同案例 {run.comparison?.case_ids.length} 条 · 基线 {run.comparison?.baseline_id}</p>
    <Collapse ghost items={[{key:"frozen",label:"冻结数据版本",children:<pre>{run.comparison?.input_digest}</pre>}]}/>
    {summary?<>
      <p className="muted">{summary.note}</p>
      <Table rowKey="id" dataSource={summary.candidates} pagination={false} scroll={{x:800}} columns={[
        {title:"候选",dataIndex:"id",width:160},
        {title:"目标参数",dataIndex:"target_parameters",width:250,render:value=><pre>{pretty(value)}</pre>},
        {title:"已处理 / 计划",key:"processed",width:130,render:(_,p)=>`${p.processed} / ${p.total}`},
        {title:"返回一致 / 不一致 / 未确认",key:"receipt",width:210,render:(_,p)=>`${p.receipts.target_confirmed??0} / ${p.receipts.mismatch??0} / ${p.receipts.unconfirmed??0}`},
        {title:"执行异常",dataIndex:"failed",width:100},
      ]}/>
      <Tabs items={[
        {key:"metrics",label:"指标与配对差值",children:<><Segmented aria-label="指标范围" options={[{label:"质量、耗时与成本",value:"key"},{label:"全部指标",value:"all"}]} value={metricScope} onChange={value=>setMetricScope(String(value))}/><Table rowKey="id" dataSource={rows} columns={columns} scroll={{x:1260}} pagination={{pageSize:12}}/></>},
        {key:"quality",label:"检索质量对照",children:summary.quality_series.length?<><p className="muted">横轴为候选配置；曲线展示各自有效均值，分母见指标表，不代表配对差值。</p><Suspense fallback={<LoadingState label="正在加载对照图表"/>}><ComparisonChart groups={summary.quality_series}/></Suspense></>:<p>当前没有可展示的检索质量统计。</p>},
        {key:"samples",label:"同案例结果",children:<ComparisonSamples run={run}/>},
        {key:"advice",label:"调优依据",children:summary.suggestions.length?<>
          <p className="muted">质量分数越高越好，耗时和同币种成本越低越好；以下为配对观测，不是显著性结论或自动参数搜索。</p>
          {summary.suggestions.map((item,index)=><Collapse key={index} items={[{key:"evidence",label:`${item.candidate_id} · ${metricLabel(item.metric_id,item.parameters)} · ${item.paired} 个配对案例 · 差值 ${number(item.delta)}`,children:<ComparisonSamples run={run} caseIDs={item.case_ids}/>}]} />)}
        </>:<p>尚无可计算的配对调优依据，请查看案例证据。</p>},
      ]}/>
    </>:<><p className="muted">{isLive(run.status)?"候选调用进行中，完成后生成比较统计。":"当前处理器未提供比较统计；可查看已保留的候选案例和参数证据。"}</p><ComparisonSamples run={run}/></>}
  </Card>;
}
