# 安装与独立运行

当前为本地 Alpha 候选，尚无公开下载地址。先按本文末尾开发入口构建目录包；公开发布仍需确认许可证。

发行目录包含 CLI/server、内置配方、Python/TS SDK、worker、协议、编译后的 UI、示例，以及按 `uv.lock` 摘要校验的 Python 3.11 wheels。下载/构建时需要网络，安装过程离线；不需要 Go、uv、pnpm 或 npm。用户数据必须放在安装目录外。

## 从目录包安装

先解压匹配 OS/架构的 `seven-agent-eval-OS-ARCH.tar.gz`，检查旁边的 `.sha256`。以下以 macOS arm64 为例（Linux 对应包步骤相同）：

```bash
tar -xzf seven-agent-eval-darwin-arm64.tar.gz
mkdir -p "$HOME/Applications"
./seven-agent-eval-darwin-arm64/bin/seven-eval install --python /path/to/python3.11 "$HOME/Applications/seven-eval"
"$HOME/Applications/seven-eval/bin/seven-eval" doctor
```

`--python` 必须指向外部 Python 3.11（含 venv/ensurepip）。环境在最终安装路径的 `sdk/python/.venv` 创建，不复制开发环境。执行 Python 配方不需要 Node；TS/JS 扩展需 Node >=20.19，可通过 `SEVEN_EVAL_NODE` 指定。默认使用 PATH 中的 Node。第三方 npm 依赖仍不支持。

安装位置必须是尚不存在的专属子目录，父目录必须已存在且可写。不会覆盖已有目录，也不会自动修改 PATH 或 shell 配置。可自行将 `安装目录/bin` 加入 PATH。**执行安装后不要移动安装目录**：venv 绑定最终位置；要换位置请重新安装。Evaluation Package 则可以自由移动。

## 默认报告与 TS 统计

在安装目录外运行：

```bash
SEVEN=/absolute/install/seven-eval/bin/seven-eval
"$SEVEN" init ./my-evaluation
"$SEVEN" --package ./my-evaluation discover --json
"$SEVEN" --package ./my-evaluation run smoke
"$SEVEN" --package ./my-evaluation serve
# 浏览器 http://127.0.0.1:7431
# 用 run 输出的 run_id 导出：
"$SEVEN" --package ./my-evaluation export RUN_ID ./demo.seven.eval

cp -R /absolute/install/seven-eval/examples/typescript ./team-evaluation
"$SEVEN" --package ./team-evaluation run typescript
"$SEVEN" --package ./team-evaluation serve
```

TS 示例的 KPI 来自 `team-summary@2.0.0`，不是前端补算。`examples/http` 和 `examples/rag-http` 提供 HTTP 计划；按 [RAG 使用说明](phase3-usage.md) 配置 endpoint，reference 不进入默认请求。状态默认在 Package 的 `.seven-evaluate/`，不写进安装目录。无凭据 HTTP 目标随包提供：运行 `python3.11 /absolute/install/seven-eval/scripts/http_fixture.py`，复制 `examples/http` 到外部目录后运行 `smoke`（目标绑定回环端口7432）。RAG 受控目标为 `scripts/rag_target.py`。

## 不安装 Python/Node，只查看结果

```bash
./seven-agent-eval-darwin-arm64/bin/seven-eval install --viewer-only /absolute/new-viewer
/absolute/new-viewer/bin/seven-eval doctor --viewer-only
mkdir -p ./clean-viewer-state
/absolute/new-viewer/bin/seven-eval --package ./clean-viewer-state serve ./demo.seven.eval
```

查看器不启动 Python/Node，不执行包内代码；完整 UI 与 Go/SQLite 查询仍可用。当前结果格式为 2.0，旧 1.0 已硬切，不支持导入。

## 失败、重新安装与卸载

- `INSTALL_EXISTS`：选新目录；如果确实要重新安装，先显式卸载原目录再安装。
- `PYTHON_REQUIRED` / `PYTHON_UNAVAILABLE`：指定 Python 3.11；查看报告用 `--viewer-only`。
- `NODE_UNAVAILABLE`：只有 TS/JS 资产不可用，Python 资产与结果查看不受影响。安装不强制准备 Node。
- `INSTALL_RESOURCES_MISSING/CHANGED`：发行资源缺失/摘要不一致，重新解压完整包；不要只复制二进制。
- `INSTALL_PARENT_UNAVAILABLE/NOT_WRITABLE`：建立自己可写的专属父目录。
- `PYTHON_ENV_FAILED/DEPENDENCIES_FAILED`：检查 Python venv/ensurepip、匹配平台及磁盘空间；失败安装保留并标记 failed，可显式卸载后重试，不假报成功。
- `INSTALL_RELOCATED`：重新安装到最终位置，不搬迁 venv。

```bash
/absolute/extracted-release/bin/seven-eval uninstall /absolute/install/seven-eval
```

先停止使用这份安装的 CLI/server，再执行卸载。卸载只针对 receipt 所属的精确目录。任何新增文件/目录、修改资源都会使卸载拒绝，并保留整个目录；先移出用户文件或恢复改动。外部 Package、状态、导出文件不删除。没有自动升级、后台下载或全局卸载脚本。

## 开发构建与实际支持矩阵

```bash
make setup
make build
make release RELEASE_PYTHON=/path/to/python3.11
make acceptance-install RELEASE_PYTHON=/path/to/python3.11
# Docker 可用时，实际执行 Linux arm64 容器安装和无运行时查看器：
make acceptance-install-linux RELEASE_PYTHON=/path/to/python3.11
```

| 环境 | 实际验证范围 |
|---|---|
| macOS 26.5.2 / arm64，Python 3.11.14、Node 20.19.5 | 原生目录安装、默认/HTTP/TS 评测、浏览器报告及独立查看器 |
| Debian bookworm / arm64 容器，Python 3.11.17、Node 20.19.5 | 禁外网、无源码挂载的实际安装/执行/重装/卸载；不是仅交叉编译 |
| Debian bookworm / arm64 独立容器，无 Python/Node | 从 macOS 带走的包安装查看器后重开；完整 Run、证据与 HTML 一致 |
| 独立物理第二机器、Windows、macOS x86_64、Linux x86_64 | NOT_RUN，不作支持承诺 |

以 `docs/phase5-progress.md` 的实际验收记录为准。同机不同目录不算跨机器；Linux 容器也不等于独立物理 Linux 主机。发行包是本地目录产物，未发布、未验证 Apple Developer ID 签名/notarization，也没有全局自动升级。
