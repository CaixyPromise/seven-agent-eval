package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"seven-agent-eval/protocol"
)

type Manifest struct {
	Format        string            `json:"format"`
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Status        string            `json:"status"`
	Files         map[string]string `json:"files"`
	InputCount    int               `json:"input_count"`
	ResultCount   int               `json:"result_count"`
	Scope         *WindowScope      `json:"scope,omitempty"`
}

const reportTemplate = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Seven Eval · {{.ID}}</title><style>body{font:16px/1.65 Georgia,serif;color:#173c34;background:#f6f5ef;max-width:1100px;margin:45px auto;padding:20px}h1{font-size:38px}small{color:#65756f}table{width:100%;border-collapse:collapse;background:white}td,th{text-align:left;border-bottom:1px solid #ddd;padding:12px}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#e9eee8;padding:15px}section{margin:32px 0}.tag{background:#dfece2;padding:6px 12px}summary{cursor:pointer}a{color:#196849}</style>
<small>SEVEN / EVALUATION REPORT</small><h1>{{.Package.Name}}</h1><p><span class="tag">{{.Status}}</span> {{.Completed}} / {{.Total}} processed · {{.CreatedAt}}</p>
{{with .Scope}}<section><h2>Window / 窗口快照</h2><p>{{.PlanID}} · {{.Start}} — {{.End}} · {{.State}} · 修订 {{.Revision}}</p><p>评分延迟：从接收证据到完成评分计算，包含排队；不等于业务调用耗时。</p></section>{{end}}
<p>{{.Message}}</p><p>Basic checks describe verified rules, not overall business correctness.</p>
{{$kpi := kpis .Outputs}}<section><h2>Report summary / 首页指标</h2>{{if $kpi.Conflict}}<p>首页指标存在多个来源，请在计划中仅保留一个首页指标提供者。原始统计仍可查看。</p>{{else}}<p>有有效指标的样本：{{display $kpi.Scored}} · 评分覆盖率（0–1）：{{display $kpi.Coverage}} · 异常样本：{{display $kpi.Anomalies}}</p>{{end}}</section>
<section><h2>Conclusions / 结论</h2>{{range .Conclusions}}<p><strong>{{.Code}}</strong> — {{.Summary}}</p>{{else}}<p>Conclusions disabled or unavailable.</p>{{end}}</section>
{{with .Comparison}}<section><h2>Candidate comparison / 候选参数比较</h2><p>基线 {{.BaselineID}} · 共同案例 {{len .CaseIDs}} 条</p><pre>{{.InputDigest}}</pre></section>{{end}}
{{range .Outputs}}{{with .Data.comparison}}<section><h2>Paired statistics / 配对统计</h2><p>{{.note}}</p><table><tr><th>候选</th><th>目标参数</th><th>已处理 / 计划</th><th>参数确认</th></tr>{{range .candidates}}<tr><td>{{.id}}</td><td><pre>{{pretty .target_parameters}}</pre></td><td>{{.processed}} / {{.total}}</td><td>{{pretty .receipts}}</td></tr>{{end}}</table><table><tr><th>指标 / 参数</th><th>候选</th><th>有效均值 / 观测数</th><th>配对案例数</th><th>配对差值</th></tr>{{range .rows}}<tr><td>{{.metric_id}}<pre>{{pretty .parameters}}</pre></td><td>{{.candidate_id}}</td><td>{{pretty .mean}} · {{.observed}} / {{.total}}</td><td>{{.paired}}</td><td>{{pretty .delta}}</td></tr>{{end}}</table></section>{{end}}{{end}}
<section><h2>Statistics / 统计</h2>{{range .Outputs}}<h3>{{.Processor}}</h3><pre>{{pretty .Data}}</pre>{{else}}<p>Processors disabled or unavailable.</p>{{end}}</section>
<section><h2>样本证据</h2><p>此文件是基础汇总报告。完整输入、检索、工具和逐项指标保存在结果包的分块记录中；使用 seven-eval 打开 .seven.eval 后可分页查询和下钻。</p></section>
{{with .DatasetRevision}}<section><h2>Dataset Revision / 数据版本</h2><pre>{{pretty .}}</pre></section>{{end}}
{{with .Episodes}}<section><h2>Episodes / 有状态轮次</h2><pre>{{pretty .}}</pre><p>只展示已保留的 Turn 结果；计划轮次与已处理轮次不同表示部分执行，不补造后续回答。</p></section>{{end}}
<section><h2>Catalogs / 码表</h2><pre>{{pretty .Catalogs}}</pre></section><section><h2>Effective plan / 执行配置</h2><pre>{{pretty .Effective}}</pre></section>
<footer>Seven Eval · self-contained report · {{.ID}}</footer></html>`

func Report(r Run) ([]byte, error) {
	r = runMetadata(r)
	t, err := template.New("report").Funcs(template.FuncMap{"kpis": ResolveReportKPIs, "display": func(v *float64) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprint(*v)
	}, "pretty": func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }}).Parse(reportTemplate)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, r)
	return b.Bytes(), err
}
func ValidateRun(r Run) error {
	if err := protocol.Validate("run", runMetadata(r)); err != nil {
		return err
	}
	for _, output := range r.Outputs {
		if err := validateProcessorOutput(output); err != nil {
			return err
		}
	}
	if !validID.MatchString(r.ID) || r.Completed < 0 || r.Completed > r.Total || r.Total > MaxCases {
		return errors.New("invalid run identity/counts")
	}
	if r.Status == "completed" && r.Completed != r.Total {
		return errors.New("completed run has missing cases")
	}
	seen := map[string]bool{}
	comparisonInputs := map[string]string{}
	if r.Comparison == nil && len(r.Effective.Plan.Candidates) > 0 {
		return errors.New("comparison metadata missing")
	}
	if r.Comparison != nil {
		if err := validateCandidates(r.Effective.Plan, Discovery{Assets: r.Effective.Assets}); err != nil {
			return err
		}
		if len(r.Effective.Plan.Candidates) < 2 || r.Comparison.BaselineID != r.Effective.Plan.Candidates[0].ID || r.Total != len(r.Comparison.CaseIDs)*len(r.Effective.Plan.Candidates) {
			return errors.New("invalid comparison identity/counts")
		}
	}
	count := 0
	err := r.ForEachCase(func(c CaseResult) error {
		count++
		if r.Comparison != nil {
			original, ok := c.Case.Metadata["seven_eval_case_id"].(string)
			if !ok {
				return errors.New("comparison case identity missing")
			}
			candidate, ok := c.Case.Metadata["seven_eval_candidate"].(string)
			if !ok {
				return errors.New("comparison candidate missing")
			}
			found := false
			for _, id := range r.Comparison.CaseIDs {
				if id == original {
					found = true
				}
			}
			if !found {
				return errors.New("comparison case not in frozen input")
			}
			found = false
			for _, item := range r.Effective.Plan.Candidates {
				if item.ID == candidate {
					found = true
				}
			}
			if !found || c.Case.ID != candidate+"/"+original {
				return errors.New("comparison candidate identity mismatch")
			}
			input := Digest([]any{c.Case.Inputs, c.Case.References})
			if previous, ok := comparisonInputs[original]; ok && previous != input {
				return errors.New("comparison candidates have different case inputs/references")
			}
			comparisonInputs[original] = input
		}
		if seen[c.Case.ID] {
			return errors.New("duplicate case in result")
		}
		seen[c.Case.ID] = true
		if err := protocol.Validate("case_result", c); err != nil {
			return err
		}
		if r.Status == "completed" && c.Status != "completed" {
			return errors.New("completed run contains failed case")
		}
		evidenceIDs := map[string]bool{}
		for _, ev := range c.Evidence {
			if evidenceIDs[ev.ID] {
				return errors.New("duplicate evidence id")
			}
			evidenceIDs[ev.ID] = true
		}
		metricIDs := map[string]bool{}
		if err := protocol.Validate("case", c.Case); err != nil {
			return err
		}
		if err := protocol.Validate("subject_result", c.Result); err != nil {
			return err
		}
		for _, m := range c.Metrics {
			if metricIDs[m.ID] {
				return errors.New("duplicate metric id")
			}
			metricIDs[m.ID] = true
			for _, ref := range m.EvidenceRefs {
				if !evidenceIDs[ref] {
					return errors.New("missing evidence reference")
				}
			}
			if err := protocol.Validate("metric", m); err != nil {
				return err
			}
			if m.CaseID != c.Case.ID {
				return errors.New("metric belongs to wrong case")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if count != r.Completed {
		return errors.New("run case count mismatch")
	}
	return nil
}
