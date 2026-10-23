# 第一阶段验收记录

> 历史记录，仅描述第一阶段当时的提交和测试状态。当前安装与容量记录见 [第五阶段](phase5-progress.md)，首发准备的新验收见 [开源准备](open-source-readiness.md)。以下旧分支、未提交状态和测试数量不是当前状态。

日期：2026-09-05。环境：macOS arm64，Go 1.25.1，uv Python 3.11.14，Node 20.19.5，pnpm 10.24.0。
仓库：独立本地 Git，分支 `codex/offline-eval-phase1`；未创建远程、未提交、未推送。

## 自动验证

```sh
make setup
make test
go vet ./...
go build -o bin/seven-eval ./cmd/seven-eval
python3 scripts/acceptance.py
./bin/seven-eval verify artifacts/phase1/browser.seven.eval
```

- Go race：core 15 个顶层测试、server 3 个顶层测试及其子用例通过。
- Python：7 个 unittest 通过。
- TypeScript 类型检查、Vite 生产构建通过；Go vet 通过。
- acceptance：init/discover/replay/真实 HTTP/导出/原目录删除后 verify 通过。
- HTTP 验收收到 1 次真实调用，body 只有输入问题；历史结果路径调用次数为 0。
- 浏览器下载的五样本结果包通过 CLI verify。

回归覆盖默认继承/替换/关闭、目录迁移、无效引用、冲突与 helper 忽略、超时与崩溃、
脱敏不改变评分、缺失不为零、指标身份分组、取消/重启恢复、源码快照、包损坏/路径/摘要/引用校验、
大整数保真，以及 journal 已落盘而查询投影失败时的恢复。

## 浏览器实测

使用 Playwright CLI 驱动真实 Chrome，服务来自本次构建产物。

1. `http://127.0.0.1:7431`：历史示例发现 9 项资产、3 个计划、4 项内置资产、0 项无效。
2. 页面启动 smoke，观察到 processing 与逐条结果，最终 5/5 completed。
3. 默认报告：3 项显式规则检查、1 项失败、2 个异常样本；覆盖率单独注明不是业务正确率。
4. 打开 refund 样本，查看输入、参考、实际回答、上下文及逐指标状态；打开码表。
5. 点击导出，实际下载 `artifacts/phase1/browser.seven.eval`。
6. 在 `artifacts/phase1/viewer` 干净目录启动第二个 server，设置
   `SEVEN_EVAL_PYTHON=/nonexistent/python`，导入 demo 结果包后浏览器自动打开 1/1 报告。
7. 在第二个查看器通过文件选择器导入浏览器下载的五样本包，查看同一客服报告。

截图保存在 `output/playwright/`：discovery.png、report.png、sample.png、catalogs.png、
portable-reopened.png、browser-imported.png。重新打开页面的控制台错误和警告均为 0。
初次开发页面曾有 favicon 404，最终构建已添加 favicon，重新验证无该错误。

## 可带走产物

- `artifacts/phase1/demo.seven.eval`：init 最小历史结果演示。
- `artifacts/phase1/http.seven.eval`：真实受控 HTTP 调用结果。
- `artifacts/phase1/browser.seven.eval`：页面执行并下载的客服五样本报告。
- `artifacts/phase1/report.html`：demo 包中的无外部依赖基础 HTML。
- `artifacts/phase1/acceptance.json`：自动验收摘要。

上述产物不纳入 Git。结果包无需原 Package、原 SQLite 或用户扩展代码；交互页面仍需要
已构建的 seven-eval 及 Web 资源，直接阅读包内 HTML 则不需要运行服务。

## 交付边界

这是 T00–T04 的本地开发环境验收，不是 T13 跨平台安装发行验收。
生产事件目前只有协议和归一化 fixture；持续生产评分、完整 Agent/RAG/judge 配方、
更多行业 adapter、TS worker、参数对比仍是后续任务。基础确定性规则不是完整语义质量评测。
自定义 Python 是可信代码，没有不可信脚本沙箱；server 仅回环监听。
独立后端代码复核及修复记录见 `docs/review.md`。
