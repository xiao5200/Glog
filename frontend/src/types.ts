// 与 Go 端 pkg/models JSON 契约一一对应的 TypeScript 类型。
// 时间统一为 ISO 字符串（Wails JSON 序列化 time.Time）。

export type LogLevel =
  | 'DEBUG'
  | 'INFO'
  | 'WARN'
  | 'ERROR'
  | 'CRITICAL'
  | 'FATAL'
  | 'UNKNOWN'
  | string

export interface TimeRange {
  start: string
  end: string
}

export interface LogEntry {
  doc_id: number
  id: string
  timestamp: string
  level: LogLevel
  message: string
  source: string
  source_host: string
  source_file: string
  source_offset: number
  event_id?: number
  provider?: string
  computer?: string
  user_id?: string
  fields?: Record<string, unknown>
  raw_log: string
  tags?: string[]
  is_anomaly: boolean
  risk_score?: number
  is_evidence: boolean
}

export interface IndexStatistics {
  total_documents: number
  unique_terms: number
  index_size_bytes: number
  log_size_bytes: number
  level_counts: Record<string, number>
  time_range?: TimeRange
  cache_hit_rate: number
  last_build_time: string
}

export interface LogSource {
  id: string
  type: string
  host: string
  path: string
  parse_mode: string
  template_id: string
  log_count: number
  size_bytes: number
  time_range?: TimeRange
  created_at: string
  ai_provider?: string
}

export interface Project {
  id: string
  name: string
  description: string
  created_at: string
  updated_at: string
  status: 'active' | 'archived' | string
  project_path: string
  log_count: number
  size_bytes: number
  time_range?: TimeRange
  sources: LogSource[]
  level_distribution: Record<string, number>
  tags?: string[]
  priority: 'high' | 'medium' | 'low' | string
}

export interface CreateProjectRequest {
  name: string
  description: string
  tags: string[]
  priority: string
}

export interface ProjectFilter {
  status?: string
  tags?: string[]
  sort_by?: string
  sort_desc?: boolean
  search?: string
}

export interface ProjectUpdates {
  name?: string
  description?: string
  tags?: string[]
  priority?: string
  status?: string
}

export interface HostStat {
  host: string
  log_count: number
}

export interface ProjectStatistics {
  total_logs: number
  size_bytes: number
  time_range?: TimeRange
  level_distribution: Record<string, number>
  top_hosts: HostStat[]
  source_count: number
  index_size_bytes: number
  unique_terms: number
}

export interface FileInfo {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
}

export interface DetectedLog {
  id: string
  name: string
  path: string
  type: 'file' | 'evtx' | 'eventlog' | 'journal' | string
  channel?: string
  category: string
  size_bytes: number
  mod_time: string
  readable: boolean
  template_id?: string
  format?: string
  preview?: string
}

export interface LocalLogDiscovery {
  scanned_at: string
  roots: string[]
  items: DetectedLog[]
  errors: string[]
  truncated: boolean
}

export interface MiddlewarePreset {
  name: string
  description: string
  category: string
  windows_paths: string[]
  linux_paths: string[]
  template_id: string
  multiline: boolean
}

export interface LevelExtraction {
  field: string
  rules: LevelRule[]
  default: string
}

export interface LevelRule {
  range?: number[]
  match?: string
  level: string
}

export interface MultiLineConfig {
  enabled: boolean
  pattern: string
  negate: boolean
  match: 'after' | 'before' | string
  max_lines: number
  timeout_ms: number
}

export interface ParseTemplate {
  id: string
  name: string
  description: string
  type: string
  category: string
  format?: string
  priority?: number
  regex: string
  field_map: Record<string, number>
  time_format: string
  time_field: string
  level_map: Record<string, string>
  level_field: string
  level_extraction?: LevelExtraction
  multiline?: MultiLineConfig
  examples?: string[]
  created_at?: string
  updated_at?: string
  usage_count?: number
  source?: string
}

export interface TemplateFilter {
  type?: string
  category?: string
  search?: string
}

export interface ParsedSample {
  raw_log: string
  parsed?: LogEntry
  success: boolean
  error?: string
}

export interface PreviewResult {
  samples: ParsedSample[]
  success_rate: number
}

export interface PreviewParseRequest {
  template: ParseTemplate
  samples: string[]
}

export interface FormatDetection {
  detected_format: string
  confidence: number
  suggested_templates: string[]
}

export interface AITemplateRequest {
  samples: string[]
  provider: string
  template_name: string
  category: string
}

export interface ParseConfig {
  mode: 'builtin' | 'ai' | 'native_evtx' | string
  template_id?: string
  ai_provider?: string
  save_template?: boolean
  sample_size?: number
}

export interface LocalCollectRequest {
  project_id: string
  file_paths: string[]
  recursive: boolean
  source_type?: string
  mode?: string
  file_pattern?: string
  exclude_paths?: string[]
  concurrency?: number
  buffer_size?: number
  parse: ParseConfig
}

export interface SSHCollectRequest {
  project_id: string
  host_id: string
  remote_paths: string[]
  strategy: string
  filter_pattern?: string
  compression?: string
  concurrency?: number
  since?: string
  until?: string
  unit?: string
  parse: ParseConfig
}

export interface WindowsEventLogRequest {
  project_id: string
  channels: string[]
  start_time?: string
  end_time?: string
  event_ids?: number[]
  levels?: string[]
  parse: ParseConfig
}

export interface SystemdJournalRequest {
  project_id: string
  since?: string
  until?: string
  unit?: string
  priority?: string
  lines?: number
  follow: boolean
  parse: ParseConfig
}

export interface ImportFileRequest {
  project_id: string
  file_path: string
  parse: ParseConfig
}

export interface CollectResult {
  task_id: string
  project_id: string
  source_id: string
  source_ids: string[]
  file_count: number
  total_bytes: number
  log_count: number
  failed_count: number
  duration: number
  status: string
  error_message?: string
  finished_at: string
}

export interface TaskInfo {
  id: string
  type: string
  project_id: string
  source_id?: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'stopped' | string
  progress: number
  message: string
  processed: number
  total: number
  error?: string
  started_at: string
  finished_at?: string
  result?: CollectResult
}

export interface CollectionProgress {
  task_id: string
  progress: number
  message: string
  processed: number
  total: number
}

export interface SSHHost {
  id: string
  name: string
  host: string
  port: number
  username: string
  auth_method: 'password' | 'key' | string
  key_path?: string
  proxy_jump?: string
  has_password: boolean
  last_used_at?: string
}

export interface SaveSSHHostRequest {
  id?: string
  name: string
  host: string
  port: number
  username: string
  auth_method: string
  password?: string
  key_path?: string
  proxy_jump?: string
}

/** 远端日志查找请求，对应后端 SearchSSHLogs。 */
export interface SSHLogSearchRequest {
  host_id: string
  roots?: string[]
  full_scan: boolean
  keyword?: string
  search_content: boolean
  include_rotated: boolean
  max_results?: number
}

export interface SSHRemoteLogFile {
  path: string
  size: number
  mod_time: string
}

export interface SSHLogSearchResult {
  files: SSHRemoteLogFile[]
  truncated: boolean
  took_ms: number
}

export interface FieldFilter {
  field: string
  operator:
    | 'eq'
    | 'ne'
    | 'gt'
    | 'lt'
    | 'gte'
    | 'lte'
    | 'in'
    | 'contains'
    | 'not_contains'
    | 'regex'
    | string
  value: unknown
}

export interface Query {
  text?: string
  start_time?: string
  end_time?: string
  levels?: string[]
  hosts?: string[]
  sources?: string[]
  event_ids?: number[]
  field_filters?: FieldFilter[]
  sort_by?: string
  sort_order?: 'asc' | 'desc' | string
  offset?: number
  limit?: number
}

export interface QueryResult {
  total: number
  hits: LogEntry[]
  took: number
  has_more: boolean
  query_id?: string
}

export interface ContextResult {
  entries: LogEntry[]
  anchor_index: number
}

export interface QuerySnapshot {
  id: string
  project_id: string
  name: string
  description: string
  query: Query
  result_count: number
  created_at: string
}

export interface AggregateRequest {
  query: Query
  aggregate_by?: string[]
  top_n?: number
  time_bucket?: string
}

export interface TimeSeriesPoint {
  timestamp: string
  count: number
}

export interface DistributionItem {
  key: string
  count: number
  percentage: number
}

export interface TopItem {
  key: string
  count: number
}

export interface AggregateDimension {
  field: string
  items: TopItem[]
  total: number
}

export interface AggregateResult {
  time_series?: TimeSeriesPoint[]
  distribution?: DistributionItem[]
  top_items?: TopItem[]
  total: number
  took: number
}

export interface LogPattern {
  id: string
  template: string
  count: number
  percentage: number
  first_seen: string
  last_seen: string
  examples: string[]
  parameters: Record<string, string[]>
}

export interface PatternClusterResult {
  patterns: LogPattern[]
  total_count: number
  took: number
}

export interface AIAnalysisResult {
  timeline: TimelineEvent[]
  attack_phases: AttackPhase[]
  risk_level: string
  findings: string[]
  suggestions: string[]
  generated_at: string
}

export interface TimelineEvent {
  timestamp: string
  event: string
  source: string
  severity: string
  log_entry_ids: string[]
}

export interface AttackPhase {
  phase: string
  description: string
  evidence: string[]
}

export interface Investigation {
  id: string
  project_id: string
  title: string
  description: string
  clue_type: string
  clue_value: string
  status: 'open' | 'analyzing' | 'closed' | string
  related_log_count: number
  time_range?: TimeRange
  ai_analysis?: AIAnalysisResult
  evidence_ids: string[]
  tags?: string[]
  risk_score?: number
  created_at: string
  updated_at: string
  closed_at?: string
  assigned_to?: string
}

export interface EvidenceMark {
  id: string
  investigation_id: string
  doc_id: number
  log_entry_id: string
  marked_at: string
  note: string
}

export interface EvidenceItem {
  mark: EvidenceMark
  entry?: LogEntry
}

export interface CreateInvestigationRequest {
  project_id: string
  title: string
  description: string
  clue_type: string
  clue_value: string
  tags: string[]
  risk_score?: number
  time_range?: TimeRange
  assigned_to?: string
  doc_ids?: number[]
}

export interface InvestigationUpdates {
  title?: string
  description?: string
  status?: string
  clue_type?: string
  clue_value?: string
  time_range?: TimeRange
  clear_time_range?: boolean
  tags?: string[]
  risk_score?: number
  clear_risk_score?: boolean
  assigned_to?: string
}

export interface AIProviderInfo {
  provider: string
  name: string
  endpoint: string
  model: string
  enabled: boolean
  priority: number
  has_key: boolean
  configured: boolean
  updated_at: string
}

export interface SaveAIConfigRequest {
  provider: string
  endpoint: string
  model: string
  api_key?: string
  enabled: boolean
  priority: number
  parameters?: Record<string, string>
}

export interface AIModelsRequest {
  provider: string
  endpoint?: string
  api_key?: string
}

export interface AIModelTestRequest {
  provider: string
  endpoint?: string
  model: string
  api_key?: string
}

export interface LogAnalysisRequest {
  project_id: string
  doc_id: number
  provider: string
  context_before: number
  context_after: number
  question?: string
}

export interface LogAnalysisResult {
  summary: string
  risk_level: string
  findings: string[]
  suggestions: string[]
  related_doc_ids?: number[]
  analyzed: number
  provider: string
  raw_response?: string
  generated_at: string
}

export interface AnomalyDetectionRequest {
  project_id: string
  provider: string
  query: Query
  aggregate_by?: string[]
  max_logs: number
}

export interface Anomaly {
  timestamp: string
  level: string
  message: string
  reason: string
  score: number
  doc_ids?: number[]
}

export interface AnomalyDetectionResult {
  summary: string
  risk_level: string
  findings: string[]
  suggestions: string[]
  anomalies: Anomaly[]
  aggregates?: AggregateDimension[]
  analyzed: number
  provider: string
  raw_response?: string
  generated_at: string
}

export interface RootCauseRequest {
  project_id: string
  provider: string
  query: Query
  question: string
  max_logs: number
}

export interface RootCause {
  cause: string
  confidence: number
  evidence: string[]
  description: string
}

export interface RootCauseResult {
  summary: string
  root_causes: RootCause[]
  timeline: string[]
  suggestions: string[]
  analyzed: number
  provider: string
  raw_response?: string
  generated_at: string
}

export interface ReportRequest {
  project_id: string
  investigation_id?: string
  provider: string
  title: string
  query: Query
  aggregate_by?: string[]
  max_logs: number
  output_path?: string
}

export interface ReportResult {
  title: string
  content: string
  format: string
  output_path?: string
  provider: string
  generated_at: string
  aggregates?: AggregateDimension[]
}

export interface IOCItem {
  id: string
  type: string
  value: string
  threat_level: string
  description: string
  source: string
  created_at: string
}

export interface IOCImportResult {
  imported: number
  skipped: number
  errors: string[]
  items: IOCItem[]
  format: string
}

export interface IOCSearchRequest {
  project_id: string
  values: string[]
  query?: Query
}

export interface IOCMatch {
  ioc: IOCItem
  doc_id: number
  entry?: LogEntry
  matched_field: string
}

export interface IOCSearchResult {
  matches: IOCMatch[]
  total: number
  scanned: number
  took: number
}

export interface DetectionRule {
  id: string
  name: string
  description: string
  severity: string
  enabled: boolean
  tags?: string[]
  mitre_tactics?: string[]
  mitre_techniques?: string[]
  match_count: number
  last_matched_at?: string
  file_path?: string
  builtin: boolean
}

export interface RunDetectionRequest {
  project_id: string
  rule_ids: string[]
}

export interface DetectionMatch {
  rule_id: string
  rule_name: string
  severity: string
  doc_id: number
  timestamp: string
  message: string
  level: string
  source_host: string
  group_key?: string
  tags?: string[]
  risk_score: number
}

export interface GroupSummary {
  group_key: string
  count: number
  doc_ids?: number[]
}

export interface RuleRunResult {
  rule_id: string
  rule_name: string
  severity: string
  count: number
  matches: DetectionMatch[]
  error?: string
  summaries?: GroupSummary[]
}

export interface DetectionResult {
  task_id: string
  results: RuleRunResult[]
  total_matches: number
  scanned: number
  took: number
  started_at: string
  finished_at: string
}

export interface ForensicsRequest {
  project_id: string
  investigation_id?: string
  output_path: string
  doc_ids?: number[]
  ioc_values?: string[]
  query?: Query
  include_entries: boolean
  include_evidence: boolean
}

export interface ForensicsResult {
  path: string
  files: string[]
  entry_count: number
  bytes: number
  sha256: string
  generated_at: string
}

export interface GeneralConfig {
  version: string
  data_path: string
  temp_path: string
  log_path: string
}

export interface PerformanceConfig {
  max_goroutines: number
  parser_workers: number
  indexer_workers: number
  ai_task_workers: number
  max_memory_mb: number
  cache_size_mb: number
  lru_cache_items: number
  query_timeout_seconds: number
  query_cache_ttl_minutes: number
}

export interface CollectorConfig {
  ssh_timeout_seconds: number
  ssh_concurrency: number
  ssh_compression: string
  local_buffer_size_kb: number
  local_batch_size: number
}

export interface IndexerConfig {
  index_flush_interval_seconds: number
  index_batch_size: number
  compression_algorithm: string
  bloom_filter_size: number
  bloom_filter_hash_functions: number
}

export interface ParserConfig {
  multiline_timeout_ms: number
  multiline_max_lines: number
  default_time_format: string
}

export interface AIConfigSection {
  default_provider: string
  default_sample_size: number
  max_sample_size: number
  request_timeout_seconds: number
  max_logs_per_request: number
}

export interface UIConfig {
  theme: string
  language: string
  log_page_size: number
  virtual_scroll_overscan: number
}

export interface AppConfig {
  general: GeneralConfig
  performance: PerformanceConfig
  collector: CollectorConfig
  indexer: IndexerConfig
  parser: ParserConfig
  ai: AIConfigSection
  ui: UIConfig
}

export interface SystemInfo {
  os: string
  arch: string
  version: string
  data_path: string
  projects_path: string
  temp_path: string
  log_path: string
  db_path: string
  executable: string
}

export interface HostPort {
  protocol: string
  local_address: string
  local_port: number
  process?: string
  pid?: number
  exposed: boolean
}

export interface HostConnection {
  protocol: string
  local_address: string
  local_port: number
  remote_address: string
  remote_port: number
  state: string
  process?: string
  pid?: number
  external: boolean
}

export interface HostNetworkSnapshot {
  generated_at: string
  hostname: string
  platform: string
  arch: string
  ports: HostPort[]
  connections: HostConnection[]
  external_ips: string[]
  errors: string[]
  truncated: boolean
  total_ports: number
  total_connections: number
}

export type RouteName =
  | 'projects'
  | 'collect'
  | 'logs'
  | 'log-detail'
  | 'investigations'
  | 'threat'
  | 'settings'
