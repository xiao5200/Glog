# GLog · 日志分析工作台

[简体中文](README.md) | [English](README.en.md)

GLog 是一个本地运行的桌面日志分析工具，面向个人安全分析师、运维工程师与应急响应人员。日志、索引与调查数据全部留在本机，AI 能力完全可选。

> 技术栈：Go 1.26 · Wails v2 · SolidJS · TypeScript · TailwindCSS · SQLite（modernc，纯 Go） · LZ4 · Roaring Bitmap

![日志分析工作台](docs/assets/log-workspace.png)

---

## 核心能力

| 模块 | 能力 |
|---|---|
| 项目 | 项目隔离、归档、导入导出（`.glogpkg.zip`）、统计、离线包校验；新建项目为空项目（0 日志/0 来源），采集任务队列按项目隔离；启动时自动扫描本机常见日志来源（仅建议，不自动加入项目），展示发现路径并支持一键采集全部可读项 |
| 采集 | 本地文件/通配符/目录递归、`.gz` 自动解压、Windows EVTX 原生解析（非管理员时自动 `wevtutil` 导出）、Linux journalctl、SSH 远程（compress/filter/journalctl/incremental/plain）、SSH 一键采集系统日志与远程日志检索（/var/log、全盘、文件名/内容关键词，结果勾选后拉取）；采集结果自动登记为项目日志源，可一键再次采集 |
| 解析 | 内置模板（Nginx/Apache/Syslog/MySQL/Redis/PostgreSQL/Kafka/SSH/IIS/Windows EVTX 等）、正则/JSON/syslog、多行合并、格式检测、预览、AI 生成模板 |
| 存储索引 | LZ4 压缩日志段、倒排索引（delta+varint）、时间索引、Roaring Bitmap 字段索引、Bloom 过滤器、原子落盘 |
| 查询 | 关键词、AND/OR/NOT、括号、短语、后缀通配、`field:value`、范围、时间窗、排序分页、上下文、聚合、模式聚类、查询快照；列表直接显示 Windows 事件 ID，展开行可查看全部识别字段；详情/完整日志视图可查看不截断的解析 JSON、原始日志与中文命名字段表 |
| AI（可选） | OpenAI / DeepSeek / Ollama / 自定义兼容接口；模型列表识别与连通性测试、日志页按类型/时间/事件类型/来源 IP/目的 IP/事件 ID 聚合或逐条 AI 分析、单条/上下文分析（±5–±200 条）、异常检测、根因分析、自然语言查询、报告生成（按时间/日志类型/IP/事件 ID 定义范围）、解析模板生成、调查分析 |
| 应急响应 | 调查工单（创建/编辑/结案/删除）、证据标记与按 DocID 补充、关联日志检索、证据/线索时间线、AI 研判、证据包导出（含 manifest/README/JSONL） |
| 威胁狩猎 | 按时间范围 AI 检测、本机监听端口与外联暴露面、实时网络连接、取证包（含 SHA256 manifest） |
| 设置 | 通用参数、AI 提供商（模型识别/连通性测试）、SSH 主机、模板编辑/预览 |

---

## 快速开始（开发）

环境要求：

- Go 1.23+（自动 toolchain 已配置 go 1.26）
- Node.js 18+ / npm
- Windows 需 WebView2 Runtime（Windows 11 自带）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

```bash
# 1. 安装前端依赖
cd frontend
npm install

# 2. 开发模式（热更新）
wails dev

# 3. 生产构建
wails build
# 产物：build/bin/GLog.exe
```

Windows 下若使用 Git Bash，确保 `%USERPROFILE%\go\bin` 在 PATH 中。

---

## 数据目录

首次启动自动解析：

1. 优先使用可执行文件旁的 `data/`（便携模式）；
2. 若不可写，回退到 `%APPDATA%/GLog`（Windows）或用户配置目录；
3. 可通过环境变量 `GLOG_DATA_DIR` 强制指定。

```
data/
├── config.toml          # 全局配置
├── metadata.db          # SQLite 元数据（WAL）
├── secrets.key          # AES-256-GCM 凭据密钥（0600）
├── logs/glog.log        # 运行日志
├── temp/
└── projects/<projectID>/
    ├── project.meta
    ├── raw/logs.dat     # LZ4 压缩日志块
    └── indexes/         # docs/inverted/time/bitmap/bloom/meta
```

> 请妥善备份 `secrets.key`，丢失后已加密的 SSH 密码与 AI API Key 无法解密。

---

## 使用流程

1. **项目总览**：新建项目，或导入 `.glogpkg.zip`；启动时自动发现本机常见日志来源，展示数量摘要与日志路径；打开项目后可一键采集全部可读发现项。
2. **数据采集**：从发现结果、快捷路径或手动选择本地文件 / Windows 事件日志 / SSH 远程 / 文件导入；选择解析模式与模板；查看解析预览后开始采集。SSH 页支持“一键采集系统日志”（扫描远端 `/var/log` 文本日志后直接拉取）和“远程日志查找”（`/var/log`、全盘 `/` 或自定义目录，按路径/内容关键词检索，勾选结果后拉取选中文件或仅拉取匹配行）。采集完成的数据自动登记为项目日志源，可在右侧“日志源”详情中再次采集。
3. **日志分析**：全文检索与字段筛选、时间范围、级别/主机/来源过滤；Windows 日志在列表中直接显示事件 ID；查看聚合图表、模式聚类、上下文；上下文弹窗可选择前后各 ±5 / ±10 / ±20 / ±50 / ±100 / ±200 条并记住选择；在独立页面查看单条日志的完整解析结果，并对单条日志或多条上下文执行 AI 分析；标记证据或创建调查工单。
4. **调查工单**：整理线索、证据链、关联日志与时间线，调用 AI 生成分析结论，导出含 README/SHA256/JSONL 的证据包。
5. **威胁狩猎**：选择时间范围调用 AI 检测全部/采样日志；查看本机监听端口、外联连接与外部 IP（支持实时刷新）；生成含 manifest 与 SHA256 的取证包。
6. **系统设置**：配置 AI 提供商（支持模型识别与连通性测试）与 SSH 主机，编辑解析模板。

---

## 本机日志自动发现

应用启动后会自动扫描当前主机常见日志位置，并在“项目总览”显示数量摘要（不展开日志内容）：

- Windows：System / Application / Security 事件通道、`C:\Windows\Logs`（CBS、DISM、setupapi 等）、IIS、Nginx、Apache、Redis、ProgramData 与用户目录；
- Linux：`/var/log` 及 Nginx、Apache、MySQL、PostgreSQL、Redis、audit 等常见目录，以及 systemd journal；
- 对文本日志读取少量样本自动匹配解析模板（如 `windows-cbs`、`nginx-combined`）。
- 发现的 `.evtx` 文件可直接加入本地采集，应用会自动切换为原生 EVTX 解析；`Setup` / `Security` 等受保护系统通道请使用“Windows 事件日志”页采集（普通用户通过 `wevtutil` 自动导出）。

在“数据采集 → 本地文件”中可看到完整发现列表：点击条目即可填入路径并套用建议模板；采集完成后该日志自动成为项目日志源。

![本机日志发现](docs/assets/local-discovery.png)

---

## 查询语法

```
error AND timeout
"exact phrase"
level:ERROR AND host:web-01
event_id:4625
source_ip:10.0.0.5
destination_ip:10.0.0.9
event_type:Microsoft-Windows-Security-Auditing
fields.status:[400 TO 499]
@timestamp:[2024-01-01T00:00:00Z TO 2024-01-02T00:00:00Z]
(level:ERROR OR level:CRITICAL) NOT debug
error*
```

- 空格为隐式 AND，支持 `AND` / `OR` / `NOT`、括号、短语、后缀通配；
- 字段：`level`、`host`/`source_host`、`source`、`event_id`、`event_type`、`source_ip`、`destination_ip`、`message`、`fields.<key>`；IP 字段兼容 `IpAddress` / `SourceIp` / `RemoteIp` / `RemoteAddr` / `ClientIp` / `DestinationIp` / `DestAddress` 等常见命名；`event_type` 优先取 `event_type`/`EventType`/Windows `Channel`，其次 Provider、源文件名与来源；派生字段已纳入 bitmap 索引，字段过滤与聚合无需逐条读取日志；
- 大小写不敏感；结果默认按时间倒序。

---

## 威胁检测

威胁狩猎界面不再内置 YAML 检测规则或 IOC 指标库。检测由 AI 完成：选择时间范围与提供商后对日志进行研判，结果包含风险级别、关键发现、处置建议与异常条目，并按项目持久化保存，重新打开页面不会自动重跑。

---

## 安全说明

- 所有数据本地存储；AI 请求仅在用户显式触发时发送采样日志，可仅使用本地 Ollama。
- SSH 只执行白名单内的只读命令（读取、tar、grep/find 检索、journalctl）；路径与参数严格转义，拒绝换行/NUL；搜索仅返回可读的日志类文件，不修改远程文件；密码与 API Key 使用 AES-256-GCM 加密。
- 项目包导入/证据包导入做 Zip Slip、路径穿越、大小与条目数校验。
- 日志内容在 AI Prompt 中标记为不可信数据，要求模型忽略其中指令（降低 Prompt Injection 风险）。

---

## 完整日志查看

在日志列表中悬停某行并点击右侧“独立页面”按钮（或在详情抽屉中点击“完整日志”），进入独立的 **日志详情** 页面：

- **解析结果 JSON**：完整 `LogEntry`（时间、级别、来源、event_id/provider、动态 Fields、原始日志、标签/分析标记），不截断，可一键复制；
- **原始日志**：采集器读取并交给解析器的原始文本，完整显示；
- **字段表**：逐项查看完整字段值（嵌套对象格式化）；
- 支持上一条/下一条、返回列表，并保留原查询与滚动状态；右侧展示元数据与相邻日志。

![完整日志视图](docs/assets/log-detail.png)

---

## 单条日志 AI 分析

在日志列表悬停某行点击 AI 按钮，或在“日志详情”页点击 **AI 分析**：

- **仅分析本条**：将目标日志完整字段、消息与原始日志交给 AI，输出风险等级、摘要、关键发现与处置建议；
- **上下文综合分析**：按 ±5 / ±10 / ±20 / ±50 / ±100 / ±200 条检索目标日志前后的上下文，一并分析关联行为；上下文过长时围绕目标日志截取并在提示词中注明截断范围；
- 可填写补充问题引导关注点，结果中的相关 DocID 可直接跳转查看。

> 发送前会对日志做长度限制并标注为不可信数据，避免 Prompt Injection。

日志工作台的 **AI 分析** 按钮还提供面向范围的完整分析面板：

- 选择日志类型（项目日志源）与时间范围（近 15 分钟 / 1 小时 / 24 小时 / 7 天 / 全部 / 自定义）；
- **聚合分析**：抽取范围内日志交给 AI 输出风险、发现、建议与异常条目；可选择事件类型 / 来源 IP / 目的 IP / 事件 ID / 主机 / 级别作为聚合维度，按维度统计查询范围内全部日志的 Top 20 并随日志样本一起提交给 AI；
- **逐条分析**：加载候选日志后选择一条，仅分析该条或结合 ±5 / ±10 / ±20 / ±50 / ±100 / ±200 条上下文；
- 两种结果均按项目持久化，重新打开面板只恢复展示，不会自动重新调用 AI；
- 日志工作台的“聚合图表”面板可切换维度（事件类型 / 来源 IP / 目的 IP / 事件 ID / 主机 / 级别），按当前查询范围展示 Top 10；
- 生成分析报告时可独立选择时间范围、日志类型、来源 IP、目的 IP、事件 ID 与聚合维度；日志条数仅作为 AI 采样上限，不再决定统计范围；
- 列表行双击展开“全部字段”，字段名以中文展示（保留原始键作参考）。

---

## 威胁狩猎：AI 检测与网络连接

- **检测结果**：选择近 15 分钟 / 1 小时 / 24 小时 / 7 天 / 全部或自定义时间范围，调用 AI 对范围内日志采样分析，展示风险级别、研判摘要、关键发现、处置建议与异常条目；异常条目可按 DocID 跳转日志详情。结果按项目持久化，重新打开页面不会自动重跑，只有点击“开始检测”才会重新调用 AI。
- **网络连接**：读取当前主机监听端口、全部/外联连接状态、进程与外部 IP，标记 `0.0.0.0` / `::` 等对外暴露端口；支持手动刷新与 5 秒实时刷新。
- 界面不再提供 IOC 指标库与内置检测规则，威胁检测统一由 AI 完成。

![AI 检测结果](docs/assets/threat-detection.png)

![网络连接与外部 IP](docs/assets/network-connections.png)

---

## 测试与质量

```bash
# 后端（含端到端集成测试）
go test ./...

# 前端类型检查与构建
cd frontend && npm run build

# 应用构建
wails build
```

已覆盖的自动化测试：

- `internal/app`：项目创建/打开 → 本地采集 → 索引 → 布尔检索/过滤 → 聚合/聚类 → 规则检测 → IOC 检索 → 调查工单/证据 → 取证包 → 项目导出 → 重启后检索；
- `internal/parser`：全部内置模板示例解析、级别/时间、JSON/syslog 检测、预览、AI 模板生成、多行合并；
- `internal/query`：AND/OR/NOT、短语、通配、字段范围、分页排序、上下文、聚合、聚类、索引重开持久化；
- `internal/indexer`：分词（IP/CJK）、追加/读取、统计、Finalize 与重开；
- `internal/storage` / `internal/security` / `internal/project` / `internal/threat`：SQLite CRUD、AES-GCM、项目导出导入、规则播种与 IOC 导入；
- `internal/ai`：OpenAI 兼容请求、错误脱敏、JSON 容错解析、提示词与响应解析。

### 主机日志实测（2026-11-12，Windows 11）

- 本地文件：采集 GLog 自身运行日志 `glog.log` 20 条，JSON 解析与关键词检索成功；
- Windows 事件日志：普通用户直接读取 `winevt\Logs\System.evtx` 被拒绝，应用自动改用 `wevtutil epl` 导出后解析；
- 实测 System 42,957 条 + Application 34,281 条（共 77,258 条）在约 20 秒内完成解析/索引，`level:ERROR` 命中 1,782 条；
- 24 小时时间过滤场景：System+Application 采集 1,150 条，耗时约 10 秒；
- 虚拟滚动、滚动加载更多（200→400）、完整日志 JSON 视图均已在浏览器联调中验证；
- 本机日志自动发现实测（Windows 11）：约 0.1 秒内返回 43 个来源，包含 System/Application/Security 事件通道、`CBS.log`（5.1 MB）、`dism.log`（22.9 MB）与 setupapi 等，并自动匹配 `windows-evtx` / `windows-cbs` 模板；
- 本机文件采集后自动登记为项目日志源，源详情“再次采集”二次采集 31 条日志验证通过。

### 功能联调（2026-11-12，DeepSeek `deepseek-flash`）

- **AI 配置**：模型识别返回 `deepseek-flash` / `deepseek-v4-pro`，保存前模型连通性测试成功；
- **单条日志 AI**：对 EVTX 日志结合 ±10 条上下文分析成功，返回风险、摘要、发现与建议；
- **日志页 AI**：聚合分析（最多 200 条）与逐条分析（仅本条/上下文）均成功，结果按项目持久化，重新打开面板直接恢复且不自动重跑；
- **事件 ID 与中文字段**：列表直接显示事件 ID；`System.EventID` → 事件 ID、`EventData.NormalProcessId` → 正常进程 ID、`EventData.TrustletIdentity` → 信任组件标识等映射验证通过；
- **威胁狩猎**：AI 检测结果持久化验证通过；本机网络快照返回 102 个监听端口、49 条外联连接、34 个外部 IP，实时刷新正常；
- **删除项目**：文件被占用时重试后返回明确提示，释放占用后删除成功。

> `Security.evtx` 仍需要管理员权限；这是 Windows 的通道保护限制。

### 演示数据（可选）

```bash
# 在 build/bin/data 生成演示项目（Nginx/SSH/JSON 日志，27 条）
GLOG_DATA_DIR=build/bin/data go run ./cmd/seeddemo

# 在已有“主机日志采集测试”项目中导入系统导出的 EVTX（需先 wevtutil epl）
GLOG_DATA_DIR=build/bin/data go run ./cmd/evtxtest
```

> 应用启动后停留在项目总览，不自动打开任何项目；只有手动打开项目后才加载日志，保证初始界面无日志。

---

## 目录结构

```
internal/
├── app/            # Wails 绑定、服务编排、异步采集任务
├── ai/             # AI 提供商与提示词
├── collector/      # 本地/EVTX/journalctl/SSH 采集
├── config/         # 配置与数据目录
├── errs/ logx/     # 错误与日志
├── indexer/        # 日志存储 + 索引
├── logfields/      # 事件类型/IP 等派生字段统一提取（索引与查询共用）
├── parser/         # 解析器与内置模板
├── project/        # 项目文件系统/导入导出
├── query/          # 查询语法/执行/聚合/聚类
├── security/       # 凭据加密
├── storage/        # SQLite 元数据
├── investigation/  # 调查工单/证据/导出
└── threat/         # IOC/规则引擎/取证包
pkg/models/         # 共享模型与 DTO（前后端契约）
configs/            # 内置模板、规则、中间件预设（embed）
frontend/           # SolidJS 界面
docs/               # 需求与设计分析
```

---

## 已知边界

- 内置正则引擎使用 Go `regexp`（RE2），未引入 Hyperscan（C 依赖与单文件分发冲突）；超大规模下可另行扩展。
- 千万级日志的内存索引为当前实现基线；5000 万条规模需分片/外存索引扩展，本版本未压测。
- Windows `Security.evtx` 需要管理员权限；Linux `audit.log` 需要 root 或 adm 组。
- SSH 远程日志检索依赖远端 GNU findutils（`-printf`/`-readable`）、GNU grep（`-Z`）与 `xargs -0`；BusyBox 等精简系统可能不支持。全盘检索使用 `-xdev`，不跨其它挂载点。

---

## 许可证

- **原创源代码**：Apache License 2.0，详见 [LICENSE](LICENSE)。
- **Windows 可执行文件**：静态链接 GPL-3.0 组件（`golang-evtx`、`golang-utils`），因此合并后的 `GLog.exe` 按 **GPL-3.0** 分发。GPL-3.0 全文见 [LICENSE-GPL-3.0](LICENSE-GPL-3.0)，对应源代码即本仓库；第三方组件明细见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
