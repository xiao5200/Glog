# GLog · 日志分析工作台

[简体中文](README.md) | [English](README.en.md)

GLog 是一个本地运行的桌面日志分析工具，面向安全分析师、运维工程师与应急响应人员。日志、索引与调查数据全部保存在本机，AI 能力完全可选。

> Go 1.26 · Wails v2 · SolidJS · TypeScript · SQLite · LZ4 · Roaring Bitmap

## 主要能力

- **采集**：本地文件 / 通配符 / 目录递归、`.gz` 自动解压、Windows EVTX 原生解析、Linux journalctl、SSH 远程采集、一键采集系统日志与远程日志检索。
- **解析**：Nginx / Apache / Syslog / MySQL / Redis / PostgreSQL / Kafka / SSH / IIS / EVTX 等内置模板；正则、JSON、syslog、多行合并、格式检测与 AI 生成模板。
- **检索**：关键词、AND/OR/NOT、短语、通配、`field:value`、范围、时间窗、排序分页、上下文（±5–±200）、聚合、模式聚类与查询快照。
- **分析**：单条 / 上下文 / 聚合 AI 分析、异常检测、根因分析、报告生成；调查工单与证据链；威胁狩猎、本机端口与外联连接查看。
- **安全**：凭据 AES-256-GCM 加密、SSH 只读命令白名单、项目包/证据包校验、AI Prompt Injection 防护。

## 下载与运行

- [最新版本 v1.0.0](https://github.com/xiao5200/Glog/releases/latest)（Windows x64 便携版）
- 解压后直接运行 `GLog.exe`；需要 WebView2 Runtime（Windows 11 自带）。
- 便携数据保存在 exe 旁的 `data/` 目录；迁移时务必保留 `secrets.key`。

## 快速开始（开发）

```bash
cd frontend && npm install && cd ..

wails dev      # 开发模式（热更新）
wails build    # 生产构建，产物 build/bin/GLog.exe
```

环境要求：Go 1.23+、Node.js 18+、Wails CLI、Windows WebView2 Runtime。

## 数据与安全

- 数据目录优先级：exe 旁 `data/` → `%APPDATA%/GLog` → 环境变量 `GLOG_DATA_DIR`。
- 所有日志与调查数据本地存储；AI 仅在显式触发时发送采样日志，可只使用本地 Ollama。
- SSH 仅执行白名单内的只读命令；密码与 API Key 使用 AES-256-GCM 加密保存。
- 详细用法见 [使用说明](docs/使用说明.md)，设计与需求见 [需求与设计分析](docs/需求与设计分析.md)。

## 许可证

- 原创源代码：Apache License 2.0，详见 [LICENSE](LICENSE)。
- Windows 可执行文件静态链接 GPL-3.0 组件（`golang-evtx`、`golang-utils`），合并后的 `GLog.exe` 按 GPL-3.0 分发；详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 与 [LICENSE-GPL-3.0](LICENSE-GPL-3.0)。
