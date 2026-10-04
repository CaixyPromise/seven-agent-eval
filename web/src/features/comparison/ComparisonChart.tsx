import { EvaluationChart } from "../../components/EvaluationChart";
import type { ComparisonSummary } from "../../domain/types";
import { pretty } from "../../domain/presentation";

export default function ComparisonChart({ groups }: { groups: ComparisonSummary["quality_series"] }) {
  return <>{groups.map((group) => (
    <section key={group.id}>
      <p className="muted">{group.evaluator} · {pretty(group.parameters)}</p>
      <EvaluationChart ariaLabel="各候选配置的 Precision、Recall、F1 有效均值；精确值和分母见指标对照表" option={{
        color: ["#1677ff", "#13a8a8", "#722ed1"],
        legend: {top:0}, tooltip:{trigger:"axis",renderMode:"richText"},
        grid:{left:42,right:24,top:42,bottom:55},
        xAxis:{type:"category",data:group.points.map(p=>p.id),axisLabel:{width:100,overflow:"truncate"}},
        yAxis:{type:"value",min:0,max:1},
        series:(["precision","recall","f1"] as const).map(metric=>({name:metric==="f1"?"F1 Score":metric==="precision"?"Precision":"Recall",type:"line",smooth:false,connectNulls:false,symbolSize:8,data:group.points.map(p=>p[metric])})),
      }} />
    </section>
  ))}</>;
}
