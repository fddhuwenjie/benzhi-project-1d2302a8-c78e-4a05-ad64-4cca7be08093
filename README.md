# SpecimenTransitGuard

SpecimenTransitGuard 是面向生物样本接收人员、质量审核人员和整改责任人的运输温控偏差处置服务。它用一条带修订历史的流程管理运输任务登记、温度与交接证据、自动判定、偏差调查、整改提交、独立验证和关闭摘要。

服务不依赖外部数据库，默认将聚合快照写入 `data/snapshot.json`，将不可变审计事件追加到 `data/events.jsonl`。快照使用临时文件和原子替换更新，所有已有任务的写操作通过 `If-Match` 修订号执行乐观并发控制，并通过 `Idempotency-Key` 避免重试重复写入。登记幂等记录还保存规范化业务载荷的 SHA-256 指纹；同一键对应不同载荷会返回 `idempotency_payload_conflict`，重复运输编号会通过 `duplicate_shipment` 返回既有任务标识和修订号。

## 构建、运行与测试

要求 Go 1.22 或更高版本。

```text
go build ./...
go run ./cmd/specimenwatch -addr=127.0.0.1:19081 -data=data
go test ./...
```

服务默认监听 `127.0.0.1:19081`。可通过 `-addr=127.0.0.1:<port>` 覆盖；未提供 `-addr` 时，也可将 `PORT` 设置为纯端口号，服务会绑定到对应的 `127.0.0.1:<PORT>`。为避免意外暴露数据，程序拒绝非回环监听地址。

运行真实 HTTP 回环冒烟并自动退出：

```text
go run ./cmd/specimenwatch -selfcheck -addr=127.0.0.1:19081
```

## API 使用约定

API 前缀为 `/api/v1`，健康检查为 `GET /healthz`。写请求需提供 `X-Actor`、`X-Request-ID` 和 `Idempotency-Key`；除首次登记外，还需通过 `If-Match` 提供查询结果中的当前修订号。成功响应的 `ETag` 也包含修订号。

主要资源路径如下：

- `POST /api/v1/transit-cases`：登记运输任务。
- `GET /api/v1/transit-cases/{id}`：查询任务、读数、交接证据、判定、调查和整改历史。
- `POST /api/v1/transit-cases/{id}/evidence`：分批提交按时间递增的温度读数、交接文档或二者。
- `POST /api/v1/transit-cases/{id}/assessment`：执行版本化温控判定。
- `POST /api/v1/transit-cases/{id}/investigation`：提交调查和是否整改的结论。
- `POST /api/v1/transit-cases/{id}/corrective-actions`：由被指派责任人提交整改和证据。
- `POST /api/v1/transit-cases/{id}/verification`：由独立于整改责任人和调查提交人的审核人员接受或驳回当前整改版本。
- `POST /api/v1/transit-cases/{id}/close`：关闭自动判定通过的任务。
- `GET /api/v1/transit-cases/{id}/audit`：分页查询按时间排序的审计轨迹，支持 `offset` 和 `limit`。
- `GET /api/v1/transit-cases/{id}/closure-summary`：仅对已关闭任务生成关闭摘要。

证据请求首次提交时需给出 `transport_started_at` 和 `transport_ended_at`，后续批次可省略。`readings` 中每条读数包含 `recorded_at`、`temperature_c`、`sensor_serial` 和 `source_batch`；交接文档使用 `document_ref` 和 `digest_sha256`。响应的 `evidence_progress` 会返回总读数、首末采集时间及 `start_coverage`、`end_coverage`、`handoff_document` 等缺失项。有效分批写入会递增修订号，只有交接摘要有效且温度读数覆盖运输边界时才进入 `evidence_ready`。

判定规则版本为 `temperature-v2`。判定分别返回 `low_temperature`、`high_temperature`、`missing_windows`、`severity` 和可逐项引用的 `triggers`。温度偏差达到 5 摄氏度、累计暴露达到 60 分钟或缺测窗口达到 2 小时时判为 `major`，其余触发项判为 `general`；没有触发项时为 `none`。读数未覆盖运输起止边界时拒绝判定且不修改任务。

调查的 `cause_category` 采用受控值 `packaging`、`equipment`、`handling`、`handoff`、`transport`、`other`（也兼容对应的细分类和中文值）；`disposition` 使用 `correction_required` 或 `accepted_no_correction`，并须与 `needs_correction` 一致。`trigger_impacts` 必须逐项覆盖判定触发项。一般偏差无需整改时还需 `review_reason` 和 `acceptability_basis`，重大偏差必须整改。

整改提交可通过 `issue_resolutions` 回应上一版的结构化驳回问题，逾期时必须提供 `overdue_reason`。查询结果的 `deadline` 使用 `not_due`、`due_soon`（期限不足 24 小时）、`overdue`、`submitted_on_time` 或 `submitted_late`，已提交版本的履约结论不会随查询时间变化。验证驳回使用 `issues` 数组，每项包含 `id`、`category` 和 `description`；接受时必须提供 `note` 并将 `evidence_visible` 设为 `true`。

关闭摘要会返回按交接文档、温度来源批次和整改证据去重的 `evidence_catalog`，以及涵盖登记、证据齐备、判定、调查或直接通过、每轮整改验证和关闭的 `completeness_checklist`。摘要还会核对审计修订号、状态顺序和最终关闭修订；不完整历史返回 `closure_summary_incomplete` 和具体 `missing_items`，不会生成表面成功的摘要。
