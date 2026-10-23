# 持续评分快速开始

这条路径评分已经发生的请求，不调用 Subject，也不重放业务请求。
先使用无凭据受控事件，再接经过授权和脱敏的真实来源。

从源码构建后运行（两个终端）：

```sh
./bin/seven-eval --package examples/live serve
```

```sh
python3 scripts/live_fixture.py --count 12
```

打开 `http://127.0.0.1:7431/#/live`，选择 `production-support`。
观察窗口、覆盖率、延迟和原因；暂停评分后继续发新事件，接收仍继续，恢复后消化积压。
重传应保留原事件时间与完整正文，不能重新生成不同的内容冒充 duplicate。
从详情导出明确选中的 revision，再按 [查看器说明](installation.md) 独立重开。

安装版先将 `examples/live` 复制到安装目录外，再使用安装 CLI；
事件源脚本位于发行目录 `scripts/live_fixture.py`。服务只绑定回环地址。

EEP 为 `POST /api/ingest/events`，body 包含 `plan_id` 与 `events`。
结构见 [事件样例](../protocol/examples) 和 [stream 计划](../examples/live/plans/production-support.yaml)。
202 中必须逐条检查 accepted/duplicate/rejected；ACK 在 journal 同步后返回。

OTLP 仅支持 `/v1/traces` JSON 和明确的 OpenInference 子集：root/AGENT/CHAIN、
RETRIEVER 文档、LLM token 属性、trace/resource turn ID。
protobuf 和任意跨 Turn span 图不支持；见 [受控 JSON](../protocol/examples/otlp-openinference.json)。

确定性评测默认全量；judge 必须显式配置采样率和每窗口预算。
窗口结束后仍允许迟到证据形成新 revision；导出是固定快照。
评分延迟是接收证据到评分完成（含排队），不等于业务响应耗时。
整体覆盖率需与源端 sent/drop 对账，接收端只能说明观察到的流量。

Python [异步 exporter](../sdk/python/src/seven_evaluate/exporter.py) 队列有界，
业务路径只尝试入队；内存队列不保证业务进程退出后恢复。
服务没有公网认证层；清理默认不执行，显式维护需停服。
详细上限见 [有界执行](bounded-evaluation.md)。真实生产接入尚待 T14。
