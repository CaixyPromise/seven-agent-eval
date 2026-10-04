import { useState } from "react";
import Button from "antd/es/button";
import Table from "antd/es/table";
import Tabs from "antd/es/tabs";
import type { Run } from "../../domain/types";
import { Dialog } from "../../components/Dialog";
import { JsonDetails } from "../../components/JsonDetails";
import { StatusBadge } from "../../components/StatusBadge";
import { RetrievalEvidence } from "../reports/RetrievalEvidence";
import { metricLabel, pretty } from "../../domain/presentation";

export const receiptLabel = (state?: string) => state==="target_confirmed"?"目标返回参数一致":state==="mismatch"?"目标返回参数不一致":"目标未确认参数";

export function ComparisonSamples({run,caseIDs=run.comparison?.case_ids ?? []}:{run:Run;caseIDs?:string[]}) {
  const [selected,setSelected]=useState<string|null>(null);
  const cases=selected?run.cases.filter(c=>c.case.metadata?.seven_eval_case_id===selected):[];
  const candidates=(run.effective as {plan?:{candidates?:{id:string}[]}})?.plan?.candidates ?? [];
  return <>
    <Table rowKey={id=>id} dataSource={caseIDs} pagination={{pageSize:10}} scroll={{x:650}} columns={[
      {title:"案例",key:"id",width:220,render:(_,id)=><Button type="link" onClick={()=>setSelected(id)}>{id}</Button>},
      ...candidates.map(candidate=>({title:candidate.id,key:candidate.id,width:210,render:(_:unknown,id:string)=>{
        const sample=run.cases.find(c=>c.case.metadata?.seven_eval_case_id===id && c.case.metadata?.seven_eval_candidate===candidate.id);
        return sample?<><StatusBadge status={sample.status}/><small className="secondary-id">{receiptLabel(sample.result.parameter_receipt?.state)}</small></>:"未执行";
      }})),
    ]}/>
    {selected?<Dialog title={`案例对照 · ${selected}`} drawer onClose={()=>setSelected(null)}>
      <Tabs items={candidates.map(candidate=>{
        const sample=cases.find(c=>c.case.metadata?.seven_eval_candidate===candidate.id);
        return {key:candidate.id,label:candidate.id,children:sample?<>
          <StatusBadge status={sample.status}/>
          <p>{receiptLabel(sample.result.parameter_receipt?.state)}</p>
          <JsonDetails label="请求参数" value={sample.result.parameter_receipt?.requested}/>
          <JsonDetails label="目标返回参数" value={sample.result.parameter_receipt?.applied}/>
          <JsonDetails label="共同输入" value={sample.case.inputs}/>
          <JsonDetails label="实际回答" value={sample.result.response}/>
          {sample.result.retrieval?<RetrievalEvidence documents={sample.result.retrieval} relevance={(sample.case.references as {qrels?:Record<string,number>})?.qrels}/>:null}
          {sample.result.tools?<JsonDetails label="工具证据" value={sample.result.tools}/>:null}
          {sample.result.citations?<JsonDetails label="引用证据" value={sample.result.citations}/>:null}
          <Table rowKey={(metric,index)=>String(index)} dataSource={sample.metrics} pagination={false} scroll={{x:460}} columns={[
            {title:"指标",key:"metric",render:(_,m)=>metricLabel(m.metric_id,m.parameters)},
            {title:"状态",dataIndex:"status",render:status=><StatusBadge status={status}/>},
            {title:"值",dataIndex:"value",render:pretty},
          ]}/>
        </>:<p>该候选尚未执行此案例。</p>};
      })}/>
    </Dialog>:null}
  </>;
}
