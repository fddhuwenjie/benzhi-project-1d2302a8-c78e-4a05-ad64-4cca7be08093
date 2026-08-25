# BENZHI_README

## 项目说明
- 项目：benzhi-project-1d2302a8-c78e-4a05-ad64-4cca7be08093
- 项目用途：已完整实现生物样本运输温控偏差处置服务，覆盖登记、证据接收、自动判定、调查整改、独立验证、审计和关闭摘要，并使用本地事件日志与原子快照持久化。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：SpecimenTransitGuard
- 项目概述：面向生物样本接收人员与质量审核人员的运输温控偏差处置服务，将运输任务登记、温度证据接收、自动判定、调查整改和验证关闭收束为一条可追溯流程。应用提供 HTTP JSON API，默认监听 127.0.0.1:19081，并支持通过 -addr=127.0.0.1:<port> 覆盖监听地址；根目录必须提供简体中文 README.md，说明项目用途以及标准构建、运行和测试方式。
- 核心工作流：运输任务从草稿登记开始，依次经过温度与交接证据齐备、自动偏差判定、人工调查处置、整改证据验证，最终进入已关闭状态；任何被驳回的验证均回到待整改状态并保留完整修订历史。
- 对外接口：HTTP JSON API 提供运输任务、温度读数、证据、判定、调查、整改和关闭资源；服务支持 -addr=127.0.0.1:<port>，默认使用 127.0.0.1:19081，禁止默认绑定 8080、80、3000 或 0.0.0.0，并提供可自行结束的 -selfcheck 模式完成回环 HTTP 冒烟。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/specimenwatch -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-1d2302a8-c78e-4a05-ad64-4cca7be08093-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-1d2302a8-c78e-4a05-ad64-4cca7be08093-arm64 linux/arm64
docker run -it benzhi-project-1d2302a8-c78e-4a05-ad64-4cca7be08093-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/specimenwatch -selfcheck -addr=127.0.0.1:19081`
