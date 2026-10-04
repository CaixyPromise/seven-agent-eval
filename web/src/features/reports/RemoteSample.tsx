import { useCallback } from "react";
import { evaluationApi } from "../../api/client";
import { useResource } from "../../api/useResource";
import { Dialog } from "../../components/Dialog";
import { ErrorNotice, LoadingState } from "../../components/Feedback";
import { SampleDrawer } from "./SampleDrawer";

export function RemoteSample({runID,caseID,onClose}:{runID:string;caseID:string;onClose:()=>void}) {
  const load=useCallback((signal:AbortSignal)=>evaluationApi.case(runID,caseID,signal),[runID,caseID]);
  const {data,loading,error,refresh}=useResource(load,"LOAD_CASE");
  if(data) return <SampleDrawer sample={data} onClose={onClose}/>;
  return <Dialog title={"样本 · "+caseID} drawer onClose={onClose}>
    {error?<ErrorNotice code={error} onRetry={error==="CASE_NOT_FOUND"?undefined:()=>void refresh()}/>:loading?<LoadingState label="正在加载样本证据"/>:null}
  </Dialog>;
}
