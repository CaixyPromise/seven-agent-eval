package core

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path"
	"path/filepath"
	"seven-agent-eval/protocol"
	"sort"
	"strings"
)

type Manifest struct {
	Format        string            `json:"format"`
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Status        string            `json:"status"`
	Files         map[string]string `json:"files"`
	Scope         *WindowScope      `json:"scope,omitempty"`
}

const reportTemplate = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Seven Eval · {{.ID}}</title><style>body{font:16px/1.65 Georgia,serif;color:#173c34;background:#f6f5ef;max-width:1100px;margin:45px auto;padding:20px}h1{font-size:38px}small{color:#65756f}table{width:100%;border-collapse:collapse;background:white}td,th{text-align:left;border-bottom:1px solid #ddd;padding:12px}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#e9eee8;padding:15px}section{margin:32px 0}.tag{background:#dfece2;padding:6px 12px}summary{cursor:pointer}a{color:#196849}</style>
<small>SEVEN / EVALUATION REPORT</small><h1>{{.Package.Name}}</h1><p><span class="tag">{{.Status}}</span> {{.Completed}} / {{.Total}} processed · {{.CreatedAt}}</p>
{{with .Scope}}<section><h2>Window / 窗口快照</h2><p>{{.PlanID}} · {{.Start}} — {{.End}} · {{.State}} · 修订 {{.Revision}}</p><p>评分延迟：从接收证据到完成评分计算，包含排队；不等于业务调用耗时。</p></section>{{end}}
<p>{{.Message}}</p><p>Basic checks describe verified rules, not overall business correctness.</p>
<section><h2>Conclusions / 结论</h2>{{range .Conclusions}}<p><strong>{{.Code}}</strong> — {{.Summary}}</p>{{else}}<p>Conclusions disabled or unavailable.</p>{{end}}</section>
<section><h2>Statistics / 统计</h2>{{range .Outputs}}<h3>{{.Processor}}</h3><pre>{{pretty .Data}}</pre>{{else}}<p>Processors disabled or unavailable.</p>{{end}}</section>
<section><h2>Cases / 样本证据</h2>{{range .Cases}}<details><summary>{{.Case.ID}} · {{.Status}} · {{.Reason}}</summary><h4>Input</h4><pre>{{pretty .Case.Inputs}}</pre><h4>Reference</h4><pre>{{pretty .Case.References}}</pre><h4>Response</h4><pre>{{pretty .Result.Response}}</pre><h4>Contexts</h4><pre>{{pretty .Result.Contexts}}</pre><table><tr><th>Metric</th><th>Status</th><th>Value</th><th>Reason</th></tr>{{range .Metrics}}<tr><td>{{.MetricID}}</td><td>{{.Status}}</td><td>{{pretty .Value}}</td><td>{{.Reason}}</td></tr>{{end}}</table></details>{{end}}</section>
<section><h2>Structured Evidence / 检索、工具与引用</h2>{{range .Cases}}<details><summary>{{.Case.ID}}</summary><pre>{{pretty .Result}}</pre></details>{{end}}</section>
{{with .DatasetRevision}}<section><h2>Dataset Revision / 数据版本</h2><pre>{{pretty .}}</pre></section>{{end}}
{{with .Episodes}}<section><h2>Episodes / 有状态轮次</h2><pre>{{pretty .}}</pre><p>只展示已保留的 Turn 结果；计划轮次与已处理轮次不同表示部分执行，不补造后续回答。</p></section>{{end}}
<section><h2>Catalogs / 码表</h2><pre>{{pretty .Catalogs}}</pre></section><section><h2>Effective plan / 执行配置</h2><pre>{{pretty .Effective}}</pre></section>
<footer>Seven Eval · self-contained report · {{.ID}}</footer></html>`

func Report(r Run) ([]byte, error) {
	t, err := template.New("report").Funcs(template.FuncMap{"pretty": func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }}).Parse(reportTemplate)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, r)
	return b.Bytes(), err
}
func ValidateRun(r Run) error {
	if err := protocol.Validate("run", r); err != nil {
		return err
	}
	if !validID.MatchString(r.ID) || r.Completed != len(r.Cases) || r.Completed > r.Total {
		return errors.New("invalid run identity/counts")
	}
	if r.Status == "completed" && r.Completed != r.Total {
		return errors.New("completed run has missing cases")
	}
	seen := map[string]bool{}
	for _, c := range r.Cases {
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
	}
	return nil
}
func Export(r Run, dest string) error {
	if !strings.HasSuffix(dest, ".seven.eval") {
		return errors.New("output must end in .seven.eval")
	}
	if r.Status == "running" || r.Status == "processing" {
		return errors.New("cancel or finish before exporting this offline run")
	}
	if err := ValidateRun(r); err != nil {
		return err
	}
	files := map[string][]byte{}
	files["run.json"], _ = json.MarshalIndent(r, "", "  ")
	files["catalogs.json"], _ = json.MarshalIndent(r.Catalogs, "", "  ")
	files["effective.json"], _ = json.MarshalIndent(r.Effective, "", "  ")
	schema, err := protocol.Files.ReadFile("schemas/exchange.json")
	if err != nil {
		return err
	}
	files["schemas/exchange.json"] = schema
	files["report.html"], err = Report(r)
	if err != nil {
		return err
	}
	m := Manifest{Format: "seven.eval", SchemaVersion: Version, RunID: r.ID, Status: r.Status, Files: map[string]string{}, Scope: r.Scope}
	total := 0
	for name, b := range files {
		m.Files[name] = Hash(b)
		total += len(b)
	}
	if total > 64<<20 {
		return errors.New("export exceeds 64 MiB uncompressed limit")
	}
	files["manifest.json"], _ = json.MarshalIndent(m, "", "  ")
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	if _, err = os.Lstat(dest); err == nil {
		return errors.New("destination exists; choose a new filename")
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".seven-eval-export-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	zw := zip.NewWriter(f)
	names := []string{}
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		w, e := zw.Create(name)
		if e != nil {
			zw.Close()
			f.Close()
			return e
		}
		if _, e = w.Write(files[name]); e != nil {
			zw.Close()
			f.Close()
			return e
		}
	}
	if err = zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Link publishes atomically and refuses a destination created concurrently.
	if err = os.Link(tmp, dest); err != nil {
		return err
	}
	return nil
}
func Import(reader io.ReaderAt, size int64) (Run, error) {
	var run Run
	if size > 64<<20 {
		return run, errors.New("archive exceeds limit")
	}
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return run, errors.New("invalid ZIP")
	}
	if len(z.File) > 32 {
		return run, errors.New("too many archive entries")
	}
	files := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		n := f.Name
		if strings.Contains(n, "\\") || strings.Contains(n, ":") || strings.HasPrefix(n, "/") || path.Clean(n) != n || n == ".." || strings.HasPrefix(n, "../") || f.Mode()&os.ModeSymlink != 0 || f.FileInfo().IsDir() {
			return run, errors.New("unsafe archive path")
		}
		if _, ok := files[n]; ok {
			return run, errors.New("duplicate archive entry")
		}
		if f.UncompressedSize64 > 64<<20 {
			return run, errors.New("archive entry too large")
		}
		total += f.UncompressedSize64
		if total > 64<<20 {
			return run, errors.New("archive expands beyond limit")
		}
		rd, e := f.Open()
		if e != nil {
			return run, e
		}
		b, e := io.ReadAll(io.LimitReader(rd, int64(f.UncompressedSize64)+1))
		rd.Close()
		if e != nil || uint64(len(b)) != f.UncompressedSize64 {
			return run, errors.New("corrupt archive entry")
		}
		files[n] = b
	}
	var m Manifest
	if err = decodeJSON(files["manifest.json"], &m); err != nil {
		return run, errors.New("manifest missing or malformed")
	}
	if err = protocol.Validate("manifest", m); err != nil {
		return run, err
	}
	allowed := map[string]bool{"run.json": true, "catalogs.json": true, "effective.json": true, "schemas/exchange.json": true, "report.html": true}
	if len(m.Files) != len(allowed) || len(files) != len(allowed)+1 {
		return run, errors.New("archive file set mismatch")
	}
	for name, digest := range m.Files {
		if !allowed[name] || Hash(files[name]) != digest {
			return run, fmt.Errorf("invalid digest or unexpected entry: %s", name)
		}
	}
	if err = decodeJSON(files["run.json"], &run); err != nil {
		return run, errors.New("invalid run data")
	}
	if err = ValidateRun(run); err != nil {
		return run, err
	}
	if run.ID != m.RunID || run.Status != m.Status || run.Status == "running" || run.Status == "processing" {
		return run, errors.New("manifest/result mismatch or unsealed run")
	}
	var cat, eff any
	if decodeJSON(files["catalogs.json"], &cat) != nil || Digest(cat) != Digest(run.Catalogs) {
		return run, errors.New("catalog mismatch")
	}
	if decodeJSON(files["effective.json"], &eff) != nil || Digest(eff) != Digest(run.Effective) {
		return run, errors.New("effective config mismatch")
	}
	return run, nil
}
func ImportFile(p string) (Run, error) {
	f, err := os.Open(p)
	if err != nil {
		return Run{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Run{}, err
	}
	return Import(f, stat.Size())
}
