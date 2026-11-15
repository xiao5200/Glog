# GLog · Log Analysis Workbench

[简体中文](README.md) | **English**

GLog is a local-first desktop log analysis tool for security analysts, operations engineers, and incident responders. Logs, indexes, and investigation data stay on the local machine; all AI capabilities are optional.

> Stack: Go 1.26 · Wails v2 · SolidJS · TypeScript · TailwindCSS · SQLite (modernc, pure Go) · LZ4 · Roaring Bitmap

---

## Core capabilities

| Module | Capabilities |
|---|---|
| Projects | Project isolation, archiving, import/export (`.glogpkg.zip`), statistics, offline package verification. New projects start empty (0 logs / 0 sources); collection tasks are isolated per project. On startup, common local log sources are discovered (suggestions only, never added automatically) with paths and one-click collection of all readable items. |
| Collection | Local files/wildcards/recursive directories, automatic `.gz` decompression, native Windows EVTX parsing (automatic `wevtutil` export when not elevated), Linux journalctl, SSH remote collection (compress/filter/journalctl/incremental/plain), one-click system log collection and remote log search over SSH (`/var/log`, full disk, filename/content keywords; select results to pull). Collected data is registered as a project log source and can be re-collected in one click. |
| Parsing | Built-in templates (Nginx/Apache/Syslog/MySQL/Redis/PostgreSQL/Kafka/SSH/IIS/Windows EVTX, etc.), regex/JSON/syslog, multiline merging, format detection, preview, AI-generated templates. |
| Storage & indexes | LZ4-compressed log blocks, inverted index (delta + varint), time index, Roaring Bitmap field indexes, Bloom filters, atomic persistence. |
| Query | Keywords, AND/OR/NOT, parentheses, phrases, suffix wildcards, `field:value`, ranges, time windows, sorted pagination, context, aggregation, pattern clustering, query snapshots. The list shows Windows event IDs directly; expand a row to inspect all recognized fields. Detail/full-log views show untruncated parsed JSON, raw logs, and a Chinese-labeled field table. |
| AI (optional) | OpenAI / DeepSeek / Ollama / custom OpenAI-compatible endpoints; model discovery and connectivity tests; aggregate or per-entry AI analysis by type/time/event type/source IP/destination IP/event ID; single-entry or context analysis (±5–±200 entries); anomaly detection, root-cause analysis, natural-language queries, report generation (scoped by time/log type/IP/event ID), parse-template generation, and investigation analysis. |
| Incident response | Investigation tickets (create/edit/close/delete), evidence marking and supplementation by DocID, related-log search, evidence/clue timeline, AI assessment, evidence package export (manifest/README/JSONL). |
| Threat hunting | AI detection over a time range, local listening ports and outbound exposure, live network connections, forensic package (SHA256 manifest). |
| Settings | General options, AI providers (model discovery/connectivity tests), SSH hosts, template editing/preview. |

---

## Quick start (development)

Requirements:

- Go 1.23+ (the toolchain is pinned to go 1.26 automatically)
- Node.js 18+ / npm
- Windows: WebView2 Runtime (included with Windows 11)
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

```bash
# 1. Install frontend dependencies
cd frontend
npm install

# 2. Development mode (hot reload)
wails dev

# 3. Production build
wails build
# Output: build/bin/GLog.exe
```

When using Git Bash on Windows, make sure `%USERPROFILE%\go\bin` is on `PATH`.

---

## Data directory

Resolved automatically on first launch:

1. Prefer `data/` next to the executable (portable mode).
2. If not writable, fall back to `%APPDATA%/GLog` (Windows) or the user config directory.
3. Override with the `GLOG_DATA_DIR` environment variable.

```
data/
├── config.toml          # Global configuration
├── metadata.db          # SQLite metadata (WAL)
├── secrets.key          # AES-256-GCM credential key (0600)
├── logs/glog.log        # Runtime log
├── temp/
└── projects/<projectID>/
    ├── project.meta
    ├── raw/logs.dat     # LZ4-compressed log blocks
    └── indexes/         # docs/inverted/time/bitmap/bloom/meta
```

> Back up `secrets.key` carefully. Without it, encrypted SSH passwords and AI API keys cannot be recovered.

---

## Workflow

1. **Projects**: create a project or import a `.glogpkg.zip`; startup discovers common local log sources and shows a summary with paths; after opening a project, collect all readable discoveries in one click.
2. **Collection**: start from discoveries, quick paths, or manually selected local files / Windows event logs / SSH remote hosts / file imports. Choose a parsing mode and template, inspect the preview, then collect. The SSH page supports **one-click system log collection** (scan remote `/var/log` text logs and pull them directly) and **remote log search** (`/var/log`, full disk `/`, or a custom directory; search by path or content keyword; pull selected files or only matching lines). Collected data is registered as a project log source and can be re-collected from the source detail panel.
3. **Log analysis**: full-text search and field filters, time range, level/host/source filters; Windows event IDs are shown inline. View aggregation charts, pattern clusters, and context; the context dialog supports ±5 / ±10 / ±20 / ±50 / ±100 / ±200 entries and remembers the selection. Open an independent page for full parsed details, and run AI analysis on a single entry or a context window; mark evidence or create an investigation ticket.
4. **Investigations**: organize clues, evidence chains, related logs, and a timeline; generate an AI conclusion; export an evidence package containing README/SHA256/JSONL.
5. **Threat hunting**: run AI detection over all or sampled logs in a selected time range; inspect local listening ports, outbound connections, and external IPs (live refresh supported); generate a forensic package with manifest and SHA256.
6. **Settings**: configure AI providers (model discovery and connectivity tests) and SSH hosts; edit parse templates.

---

## Local log auto-discovery

On startup, GLog scans common local log locations and shows a summary on the Projects page (without expanding log contents):

- **Windows**: System / Application / Security event channels, `C:\Windows\Logs` (CBS, DISM, setupapi, etc.), IIS, Nginx, Apache, Redis, ProgramData, and user directories.
- **Linux**: `/var/log` plus common Nginx, Apache, MySQL, PostgreSQL, Redis, and audit directories, as well as the systemd journal.
- Reads a small sample of text logs to match parse templates automatically (e.g. `windows-cbs`, `nginx-combined`).
- Discovered `.evtx` files can be collected directly; the app switches to native EVTX parsing. For protected channels such as `Setup` / `Security`, use the **Windows Event Log** page (standard users can rely on automatic `wevtutil` export).

The complete discovery list is available under **Collection → Local Files**: click an item to fill in its path and suggested template; after collection it becomes a project log source.

---

## Query syntax

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

- Whitespace is an implicit AND; `AND` / `OR` / `NOT`, parentheses, phrases, and suffix wildcards are supported.
- Fields: `level`, `host`/`source_host`, `source`, `event_id`, `event_type`, `source_ip`, `destination_ip`, `message`, `fields.<key>`. IP fields recognize common aliases such as `IpAddress`, `SourceIp`, `RemoteIp`, `RemoteAddr`, `ClientIp`, `DestinationIp`, and `DestAddress`. `event_type` prefers `event_type`/`EventType`/Windows `Channel`, then Provider, source filename, and source. Derived fields are bitmap-indexed, so field filters and aggregations do not need to scan every log entry.
- Matching is case-insensitive; results are sorted by time descending by default.

---

## Threat detection

The Threat Hunting page no longer ships with built-in YAML rules or an IOC indicator library. Detection is performed by AI: choose a time range and provider, and GLog analyzes the logs and returns risk levels, key findings, remediation advice, and anomalous entries. Results are persisted per project and are not re-run automatically when the page is reopened.

---

## Security notes

- All data is stored locally. AI requests send sampled logs only when explicitly triggered; a local Ollama endpoint can be used exclusively.
- SSH executes only whitelisted read-only commands (read, tar, grep/find search, journalctl); paths and arguments are strictly escaped and reject newlines/NUL bytes. Searches return only readable log-like files and never modify remote files. Passwords and API keys are encrypted with AES-256-GCM.
- Project/evidence package imports validate against Zip Slip, path traversal, size limits, and entry-count limits.
- Log content is marked as untrusted data in AI prompts, with instructions to ignore embedded commands (reducing prompt-injection risk).

---

## Full log view

Hover a row in the log list and click the **independent page** button (or click **Full log** in the detail drawer) to open the **Log Detail** page:

- **Parsed JSON**: the complete `LogEntry` (timestamp, level, source, event_id/provider, dynamic fields, raw log, tags/analysis flags), untruncated and copyable.
- **Raw log**: the exact text read by the collector and handed to the parser.
- **Field table**: inspect every field value in full (nested objects are formatted).
- Previous/next navigation and back-to-list, preserving the original query and scroll position; metadata and adjacent logs are shown on the right.

---

## Single-entry AI analysis

Hover a row in the log list and click the AI button, or click **AI analysis** on the Log Detail page:

- **This entry only**: sends the complete fields, message, and raw log of the target entry to the AI, returning a risk level, summary, key findings, and remediation advice.
- **Contextual analysis**: retrieves ±5 / ±10 / ±20 / ±50 / ±100 / ±200 entries around the target and analyzes related behavior together; long contexts are trimmed around the target with the truncation range noted in the prompt.
- An optional additional question can guide the analysis; related DocIDs in the result can be opened directly.

> Logs are length-limited and marked as untrusted data before being sent, mitigating prompt injection.

The **AI analysis** button on the workbench also provides a scope-oriented panel:

- Choose the log type (project source) and time range (last 15 minutes / 1 hour / 24 hours / 7 days / all / custom).
- **Aggregate analysis**: sends logs in scope to the AI and returns risks, findings, advice, and anomalous entries; choose event type / source IP / destination IP / event ID / host / level as the aggregation dimension, with the Top 20 values across the query scope submitted alongside the log samples.
- **Per-entry analysis**: load candidate logs and pick one; analyze only that entry or a ±5 / ±10 / ±20 / ±50 / ±100 / ±200 context window.
- Both result types are persisted per project; reopening the panel restores the results without calling the AI again.
- The **Aggregation charts** panel can switch dimensions (event type / source IP / destination IP / event ID / host / level) and shows the Top 10 within the current query scope.
- Report generation independently selects time range, log type, source IP, destination IP, event ID, and aggregation dimension; the log count is only an AI sampling cap and no longer defines the statistical scope.
- Double-click a row to expand **all fields**; field names are shown in Chinese with the original keys preserved for reference.

---

## Threat hunting: AI detection and network connections

- **Detection results**: select the last 15 minutes / 1 hour / 24 hours / 7 days / all, or a custom range, then run AI analysis on sampled logs. The result shows risk level, assessment summary, key findings, remediation advice, and anomalous entries; anomalous entries can jump to their log details by DocID. Results are persisted per project and are not re-run automatically when the page is reopened; only **Start detection** calls the AI again.
- **Network connections**: reads current listening ports, all/outbound connections, processes, and external IPs, marking externally exposed ports such as `0.0.0.0` / `::`; supports manual refresh and 5-second live refresh.
- The UI no longer provides an IOC library or built-in detection rules; threat detection is AI-driven.

---

## Testing and quality

```bash
# Backend (includes end-to-end integration tests)
go test ./...

# Frontend type check and build
cd frontend && npm run build

# Application build
wails build
```

Automated coverage includes:

- `internal/app`: project create/open → local collection → indexing → boolean search/filtering → aggregation/clustering → rule detection → IOC search → investigations/evidence → forensic package → project export → search after restart.
- `internal/parser`: parsing of all built-in template samples, level/time handling, JSON/syslog detection, preview, AI template generation, multiline merging.
- `internal/query`: AND/OR/NOT, phrases, wildcards, field ranges, paginated sorting, context, aggregation, clustering, index persistence across reopen.
- `internal/indexer`: tokenization (IP/CJK), append/read, statistics, finalize, and reopen.
- `internal/storage` / `internal/security` / `internal/project` / `internal/threat`: SQLite CRUD, AES-GCM, project export/import, rule seeding, and IOC import.
- `internal/ai`: OpenAI-compatible requests, error redaction, tolerant JSON parsing, prompt and response parsing.

### Host log measurements (2026-11-12, Windows 11)

- Local file: collected 20 entries from GLog's own `glog.log`; JSON parsing and keyword search succeeded.
- Windows event logs: direct reads of `winevt\Logs\System.evtx` were denied for a standard user; the app automatically fell back to `wevtutil epl` export and parsing.
- 42,957 System entries + 34,281 Application entries (77,258 total) were parsed and indexed in about 20 seconds; `level:ERROR` matched 1,782 entries.
- A 24-hour time filter collected 1,150 entries in about 10 seconds.
- Virtual scrolling, load-more (200→400), and the full-log JSON view were verified in browser integration testing.
- Local log discovery (Windows 11) returned 43 sources in about 0.1 seconds, including System/Application/Security channels, `CBS.log` (5.1 MB), `dism.log` (22.9 MB), and setupapi logs, with automatic `windows-evtx` / `windows-cbs` template matching.
- Local file collection automatically registered the file as a project log source; re-collecting it from the source details collected 31 entries successfully.

### Feature integration (2026-11-12, DeepSeek `deepseek-flash`)

- **AI configuration**: model discovery returned `deepseek-flash` / `deepseek-v4-pro`; the pre-save connectivity test succeeded.
- **Single-entry AI**: analysis of an EVTX entry with ±10 context succeeded, returning risk, summary, findings, and advice.
- **Log page AI**: aggregate analysis (up to 200 entries) and per-entry analysis (single/context) succeeded; results were persisted per project and restored without re-running.
- **Event IDs and Chinese field names**: event IDs are shown inline; mappings such as `System.EventID` → Event ID, `EventData.NormalProcessId` → Normal Process ID, and `EventData.TrustletIdentity` → Trustlet Identity were verified.
- **Threat hunting**: AI detection result persistence was verified; a local network snapshot returned 102 listening ports, 49 outbound connections, and 34 external IPs, and live refresh worked.
- **Project deletion**: when files were in use, retries returned a clear message; after releasing the lock, deletion succeeded.

> `Security.evtx` still requires administrator privileges; this is a Windows channel protection limit.

### Demo data (optional)

```bash
# Generate a demo project under build/bin/data (Nginx/SSH/JSON logs, 27 entries)
GLOG_DATA_DIR=build/bin/data go run ./cmd/seeddemo

# Import a system-exported EVTX into an existing "host log collection test" project (requires wevtutil epl first)
GLOG_DATA_DIR=build/bin/data go run ./cmd/evtxtest
```

> The app stays on the Projects page after startup and does not open any project automatically; logs are loaded only after a project is opened manually, keeping the initial view empty.

---

## Repository layout

```
internal/
├── app/            # Wails bindings, service orchestration, async collection tasks
├── ai/             # AI providers and prompts
├── collector/      # Local/EVTX/journalctl/SSH collection
├── config/         # Configuration and data directory
├── errs/ logx/     # Errors and logging
├── indexer/        # Log storage + indexes
├── logfields/      # Shared derived-field extraction (event type/IP) for indexing and querying
├── parser/         # Parsers and built-in templates
├── project/        # Project filesystem/import-export
├── query/          # Query syntax/execution/aggregation/clustering
├── security/       # Credential encryption
├── storage/        # SQLite metadata
├── investigation/  # Investigations/evidence/export
└── threat/         # IOC/rules/forensic packages
pkg/models/         # Shared models and DTOs (frontend/backend contract)
configs/            # Built-in templates, rules, middleware presets (embedded)
frontend/           # SolidJS UI
docs/               # Requirements and design analysis
```

---

## Known limitations

- The built-in regex engine uses Go `regexp` (RE2) instead of Hyperscan (C dependency conflicts with single-file distribution); it can be extended separately for very large workloads.
- In-memory indexing is currently optimized for tens of millions of logs; 50 million entries would require sharding or out-of-core indexes and has not been benchmarked in this release.
- Windows `Security.evtx` requires administrator privileges; Linux `audit.log` requires root or the `adm` group.
- SSH remote log search relies on GNU findutils (`-printf`/`-readable`), GNU grep (`-Z`), and `xargs -0` on the remote host; minimal systems such as BusyBox may not support them. Full-disk search uses `-xdev` and does not cross other mount points.

---

## License

- **Original source code**: Apache License 2.0. See [LICENSE](LICENSE).
- **Windows executable**: statically links GPL-3.0 components (`golang-evtx`, `golang-utils`), so the combined `GLog.exe` binary is distributed under **GPL-3.0**. The full license text is in [LICENSE-GPL-3.0](LICENSE-GPL-3.0); the complete corresponding source is this repository. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for details.
