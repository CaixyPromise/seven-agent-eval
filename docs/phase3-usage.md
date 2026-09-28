# 第三阶段使用说明

## 通用 HTTP RAG

```sh
make setup
make acceptance-rag
python3 scripts/rag_target.py --port 7453
```

另一个终端运行 `./bin/seven-eval --package examples/rag-http --addr 127.0.0.1:7431 serve`，在评测计划页运行 `rag`。或直接 `./bin/seven-eval --package examples/rag-http run rag`。

Source 中所有路径相对 evaluation.yaml，不相对 descriptor。JSONL/CSV 使用 `path`；BEIR/TREC 使用 `queries`（BEIR `_id/text` JSONL）、可选 `corpus`（BEIR 必须提供）、`qrels` 和 `run`（TREC 必填六列排名，BEIR 可选）。所有关联先校验后执行。源文件合计最多 32 MiB，最多 1000 Case/Turn。

Mapping 的 `fields` 支持 `$`、`$.field` 与 JSON 对象组合，不支持表达式；`conversions` 对目标顶层字段显式选择 json/number/boolean/string。CSV 空转换单元格保留缺失，必需字段缺失会在调用前失败。

## 指标定义

`parameters.k: [1, 5]` 选择多个截断。Precision 分母为 K，Recall 分母为全部正相关 qrels，MRR 为 K 内首个正相关排名倒数。NDCG 使用线性相关性增益，AP@K 用全部正相关文档数作分母；MAP 是 Processor 对有效 Case AP 的平均。没有 qrels 或没有正相关参考不可计算；有参考而检索为空为零。

定义与 [NIST map_cut](https://github.com/usnistgov/trec_eval/blob/main/m_map_cut.c)、[NIST ndcg_cut](https://github.com/usnistgov/trec_eval/blob/main/m_ndcg_cut.c) 保持该口径；[BEIR 数据格式](https://github.com/beir-cellar/beir/wiki/Load-your-custom-dataset) 是格式依据。未声明通用业务及格线。

## 可选 judge

显式 `evaluators: [evaluator:basic, evaluator:rag, evaluator:rag-judge]`，通过 `SEVEN_EVAL_JUDGE_ENDPOINT`（完整 chat-completions URL）、`SEVEN_EVAL_JUDGE_MODEL`、`SEVEN_EVAL_JUDGE_KEY` 配置。也可在 `parameters.judge` 指定 endpoint_env/model_env/auth_env。只接受 JSON score ∈ [0,1]，不自动重试，不跟随重定向。只在配置 `price.currency/input_per_1000/output_per_1000` 且返回实际 prompt/completion tokens 时计算成本。stream 仍要求显式采样率和预算。

## Seven Workspace SSE 与 Episode

Adapter YAML 使用 `type: seven-workspace`，`endpoint` 为包含部署前缀的 API 基础地址（例如 `https://development.example/api/v1`），`workspace_id` 为 ID，`auth_env` 为 bearer 凭据环境变量名。不得把凭据写入 YAML。

单 Case 创建一个新会话。Episode JSONL 使用：

```json
{"id":"episode-1","inputs":{},"episode":{"id":"episode-1","turns":[{"id":"first","case":{"id":"first","inputs":{"text":"记住名字"}}},{"id":"second","case":{"id":"second","inputs":{"text":"刚才的名字？"}}}]}}
```

Episode 内串行共享新会话，Episode 间隔离；调用失败停止剩余 Turns，评分错误不会重放生产请求。报告保留逐 Turn 指标与 Processor 的 Episode 聚合。

默认保留审计会话。当前核对的 Seven 公开路由没有可确认的逐会话归档动作，因此 native Seven adapter 拒绝 `cleanup_sessions: true`，不以删除或全局归档替代。Python Subject 可提供 `session_create` / `session_cleanup` 函数，显式 `parameters.cleanup_sessions: true` 只把本轮新创建的 Episode session 传入 cleanup。

当前 SSE 子集为 conversation.started/patch/completed/failed/cancelled/paused、part.replace、part.append_text 及工具 Part。paused 不自动继续或批准。最终回答按现有 runner 选择最后一个 message Part；未知事件只保存事件类型，不推断成功。当前公开 SSE 的 context-window usedTokens 不等同计费用量，不填入 usage.total_tokens。尚无确认的通用 Retrieval/Citation Part，未提供时保持证据缺失，不从任意工具文本推断检索。

无凭据受控演示：`make acceptance-seven`；或先 `python3 scripts/seven_target.py --port 7454`，另一个终端运行 `./bin/seven-eval --package examples/seven-fixture --addr 127.0.0.1:7431 serve`。示例有意包含失败轮次，生成 3/4 的 partial 包；CLI exit 1 表示没有完成全部计划，不表示包损坏。

## 旧 runner 历史导入

Source 使用 `format: seven-history`、`path: saved/run1`，目录含 run.json/response.json/trace.json/checks.json；可选 `case` 指向复制到 Package 的旧 Case YAML。计划必须 replay，不运行旧脚本、不跟随旧输出记录的外部路径。旧 checker 原样保留在审计信息，不冒充当前指标。

`./bin/seven-eval --package examples/seven-history run history` 可执行无凭据历史样例。该样例是旧 runner 格式的受控 fixture，不是用户真实生产历史。
