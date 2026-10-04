import { useCallback, useState } from "react";
import Button from "antd/es/button";
import Checkbox from "antd/es/checkbox";
import Table, {type ColumnsType} from "antd/es/table";
import Tag from "antd/es/tag";
import { evaluationApi, type CaseSummary } from "../../api/client";
import { useResource } from "../../api/useResource";
import { ErrorNotice } from "../../components/Feedback";
import { StatusBadge } from "../../components/StatusBadge";

export function PagedSamples({runID,completed,onOpen}:{runID:string;completed:number;onOpen:(id:string)=>void}) {
  const [page,setPage]=useState(1);
  const [anomalies,setAnomalies]=useState(false);
  const load=useCallback((signal:AbortSignal)=>evaluationApi.cases(runID,new URLSearchParams({offset:String((page-1)*25),limit:"25",anomalies:String(anomalies)}),signal),[runID,page,anomalies,completed]);
  const {data,loading,error,refresh}=useResource(load,"LOAD_CASES");
  const columns:ColumnsType<CaseSummary>=[
    {title:"样本",dataIndex:"id",minWidth:180,render:(id:string,row)=><><Button type="text" className="identity-button" onClick={()=>onOpen(id)}>{id}</Button>{row.anomaly?<Tag color="gold">需关注</Tag>:null}</>},
    {title:"执行状态",dataIndex:"status",width:110,render:status=><StatusBadge status={status}/>},
    {title:"回答预览",dataIndex:"response_preview",minWidth:220,render:text=><div className="answer-preview">{text}</div>},
    {title:"有效指标",dataIndex:"observed",width:95},
    {title:"操作",key:"action",width:115,render:(_,row)=><Button type="link" aria-label={"查看样本 "+row.id} onClick={()=>onOpen(row.id)}>查看证据</Button>},
  ];
  return <>
    <div className="sample-toolbar"><span>{completed} 条已处理</span><Checkbox checked={anomalies} onChange={event=>{setAnomalies(event.target.checked);setPage(1);}}>只看异常</Checkbox></div>
    {error?<ErrorNotice code={error} onRetry={()=>void refresh()}/>:null}
    <Table<CaseSummary> className="domain-table" rowKey="id" columns={columns} loading={loading} dataSource={data?.items??[]} scroll={{x:740}}
      locale={{emptyText:anomalies?"没有异常样本":"尚无样本结果"}}
      pagination={{current:page,pageSize:25,total:data?.total??0,showSizeChanger:false,onChange:setPage,showTotal:total=>"共 "+total+" 条"}}/>
  </>;
}
