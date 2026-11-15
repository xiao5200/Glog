import { For, JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type {
  AggregateDimension,
  AIProviderInfo,
  Anomaly,
  AnomalyDetectionResult,
  LogAnalysisResult,
  LogEntry,
  LogSource,
  Query,
  TopItem,
} from '../../types'
import { api, errorText } from '../../api'
import { navigate, openLogDetail } from '../../stores/app'
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Icon,
  LevelBadge,
  Modal,
  PageBusy,
  Segmented,
  Select,
  TextArea,
} from '../ui'
import {
  formatInt,
  formatTime,
  friendlyLogMessage,
  severityTone,
  truncate,
} from '../../utils'
import {
  basename,
  localInputToIso,
  resolveTimeBounds,
  toLocalInput,
  type TimePreset,
} from './logCommon'

/* ------------------------------------------------------------------ */
/* 类型与常量                                                           */
/* ------------------------------------------------------------------ */

type AnalysisMode = 'aggregate' | 'entry'
type EntryScope = 'single' | 'context'

interface StoredParams {
  sourceId: string
  preset: TimePreset
  customStart: string
  customEnd: string
  maxLogs: number
  docId: number | null
  scope: EntryScope
  contextWindow: number
  aggregateBy: string[]
}

interface StoredAnalysis {
  mode: AnalysisMode
  result: AnomalyDetectionResult | LogAnalysisResult
  params: StoredParams
  savedAt: string
}

type AnalysisResultState =
  | { mode: 'aggregate'; data: AnomalyDetectionResult }
  | { mode: 'entry'; data: LogAnalysisResult }

interface ParsedStored {
  mode: AnalysisMode
  result: Record<string, unknown>
  params: StoredParams
  savedAt: string
}

const STORAGE_PREFIX = 'glog.logs.ai.v1.'
const DEFAULT_MAX_LOGS = 200
const DEFAULT_CONTEXT_WINDOW = 20
const CANDIDATE_LIMIT = 50
const MAX_FIELD_ROWS = 100

const ANALYSIS_MODE_OPTIONS = [
  { value: 'aggregate', label: '聚合分析' },
  { value: 'entry', label: '逐条分析' },
]

const ENTRY_SCOPE_OPTIONS = [
  { value: 'single', label: '仅分析该条' },
  { value: 'context', label: '结合上下文' },
]

const TIME_PRESET_OPTIONS: Array<{ value: TimePreset; label: string }> = [
  { value: '15m', label: '近15分钟' },
  { value: '1h', label: '1小时' },
  { value: '24h', label: '24小时' },
  { value: '7d', label: '7天' },
  { value: 'all', label: '全部' },
  { value: 'custom', label: '自定义' },
]

const PRESET_VALUES: TimePreset[] = ['15m', '1h', '24h', '7d', 'all', 'custom']

const MAX_LOG_OPTIONS = [50, 100, 200, 500]

const MAX_LOG_SELECT_OPTIONS = MAX_LOG_OPTIONS.map((value) => ({
  value: String(value),
  label: `${value} 条`,
}))

const CONTEXT_WINDOW_OPTIONS = [
  { value: '5', label: '±5 条' },
  { value: '10', label: '±10 条' },
  { value: '20', label: '±20 条' },
  { value: '50', label: '±50 条' },
  { value: '100', label: '±100 条' },
  { value: '200', label: '±200 条' },
]

const CONTEXT_WINDOW_VALUES = CONTEXT_WINDOW_OPTIONS.map((item) => Number(item.value))

interface AggregateDimensionOption {
  value: string
  label: string
  title: string
}

const AGGREGATE_DIMENSION_OPTIONS: AggregateDimensionOption[] = [
  {
    value: 'event_type',
    label: '事件类型',
    title: '事件类型（Windows 提供程序 / 日志文件）',
  },
  { value: 'source_ip', label: '来源 IP', title: '来源 IP' },
  { value: 'destination_ip', label: '目的 IP', title: '目的 IP' },
  { value: 'event_id', label: '事件 ID', title: '事件 ID' },
  { value: 'source_host', label: '主机', title: '主机（source_host）' },
  { value: 'level', label: '级别', title: '日志级别' },
]

const AGGREGATE_DIMENSION_VALUES = AGGREGATE_DIMENSION_OPTIONS.map((item) => item.value)

// 默认聚合维度保持固定顺序，便于持久化结果稳定展示。
const DEFAULT_AGGREGATE_DIMENSIONS = ['event_type', 'event_id']

function normalizeAggregateDimensions(values: unknown): string[] {
  const raw = stringList(values)
  const selected = raw
    .map((item) => item.trim().toLowerCase())
    .filter((item) => AGGREGATE_DIMENSION_VALUES.includes(item))
  const ordered = AGGREGATE_DIMENSION_VALUES.filter((item) => selected.includes(item))
  return ordered.length > 0 ? ordered : [...DEFAULT_AGGREGATE_DIMENSIONS]
}

function aggregateDimensionLabel(field: string): string {
  return (
    AGGREGATE_DIMENSION_OPTIONS.find((item) => item.value === field)?.label ||
    field ||
    '未知维度'
  )
}

function aggregatePercentage(count: number, total: number): string {
  if (!Number.isFinite(count) || !Number.isFinite(total) || total <= 0) return '—'
  const percent = (count / total) * 100
  return `${percent >= 10 ? percent.toFixed(1) : percent.toFixed(2)}%`
}

const rangeInputStyle: JSX.CSSProperties = {
  width: '190px',
  'min-height': '28px',
  padding: '3px 8px',
  'font-size': '10.5px',
  'font-family': 'var(--font-mono)',
}

/* ------------------------------------------------------------------ */
/* 小工具                                                               */
/* ------------------------------------------------------------------ */

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringList(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function numberList(value: unknown): number[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is number => typeof item === 'number' && Number.isFinite(item),
      )
    : []
}

function normalizeAggregates(value: unknown): AggregateDimension[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((item): AggregateDimension[] => {
    if (!isRecord(item)) return []
    const field = typeof item.field === 'string' ? item.field : ''
    if (!field) return []
    const total = Number(item.total)
    const items: TopItem[] = Array.isArray(item.items)
      ? item.items.flatMap((entry): TopItem[] => {
          if (!isRecord(entry)) return []
          const key = typeof entry.key === 'string' ? entry.key : ''
          const count = Number(entry.count)
          if (!key || !Number.isFinite(count)) return []
          return [{ key, count }]
        })
      : []
    return [
      {
        field,
        items,
        total: Number.isFinite(total) ? total : 0,
      },
    ]
  })
}

function normalizeAnomalyResult(value: Record<string, unknown>): AnomalyDetectionResult {
  const anomalies: Anomaly[] = Array.isArray(value.anomalies)
    ? value.anomalies.flatMap((item): Anomaly[] => {
        if (!isRecord(item)) return []
        const docIDs = numberList(item.doc_ids)
        return [
          {
            timestamp: typeof item.timestamp === 'string' ? item.timestamp : '',
            level: typeof item.level === 'string' ? item.level : 'UNKNOWN',
            message: typeof item.message === 'string' ? item.message : '',
            reason: typeof item.reason === 'string' ? item.reason : '',
            score: Number(item.score) || 0,
            ...(docIDs.length > 0 ? { doc_ids: docIDs } : {}),
          },
        ]
      })
    : []
  const aggregates = normalizeAggregates(value.aggregates)
  return {
    summary: typeof value.summary === 'string' ? value.summary : '',
    risk_level: typeof value.risk_level === 'string' ? value.risk_level : 'unknown',
    findings: stringList(value.findings),
    suggestions: stringList(value.suggestions),
    anomalies,
    ...(aggregates.length > 0 ? { aggregates } : {}),
    analyzed: Number(value.analyzed) || 0,
    provider: typeof value.provider === 'string' ? value.provider : '',
    ...(typeof value.raw_response === 'string'
      ? { raw_response: value.raw_response }
      : {}),
    generated_at: typeof value.generated_at === 'string' ? value.generated_at : '',
  }
}

function normalizeEntryResult(value: Record<string, unknown>): LogAnalysisResult {
  const related = numberList(value.related_doc_ids)
  return {
    summary: typeof value.summary === 'string' ? value.summary : '',
    risk_level: typeof value.risk_level === 'string' ? value.risk_level : 'unknown',
    findings: stringList(value.findings),
    suggestions: stringList(value.suggestions),
    ...(related.length > 0 ? { related_doc_ids: related } : {}),
    analyzed: Number(value.analyzed) || 0,
    provider: typeof value.provider === 'string' ? value.provider : '',
    ...(typeof value.raw_response === 'string'
      ? { raw_response: value.raw_response }
      : {}),
    generated_at: typeof value.generated_at === 'string' ? value.generated_at : '',
  }
}

function parseStoredAnalysis(raw: string): ParsedStored {
  const parsed: unknown = JSON.parse(raw)
  if (!isRecord(parsed)) throw new Error('记录格式无效')
  const mode = parsed.mode
  if (mode !== 'aggregate' && mode !== 'entry') throw new Error('分析模式无效')
  if (!isRecord(parsed.result)) throw new Error('分析结果无效')
  const paramsRaw = isRecord(parsed.params) ? parsed.params : {}

  const sourceId = typeof paramsRaw.sourceId === 'string' ? paramsRaw.sourceId : ''
  const presetRaw = paramsRaw.preset
  const preset: TimePreset =
    typeof presetRaw === 'string' && PRESET_VALUES.includes(presetRaw as TimePreset)
      ? (presetRaw as TimePreset)
      : '24h'
  const customStart =
    typeof paramsRaw.customStart === 'string' ? paramsRaw.customStart : ''
  const customEnd = typeof paramsRaw.customEnd === 'string' ? paramsRaw.customEnd : ''
  const maxLogsRaw = Number(paramsRaw.maxLogs)
  const maxLogs = MAX_LOG_OPTIONS.includes(maxLogsRaw)
    ? maxLogsRaw
    : DEFAULT_MAX_LOGS
  const docIdRaw = paramsRaw.docId
  const docId =
    typeof docIdRaw === 'number' && Number.isFinite(docIdRaw) && Number.isInteger(docIdRaw)
      ? docIdRaw
      : null
  const scope: EntryScope =
    paramsRaw.scope === 'single' || paramsRaw.scope === 'context'
      ? paramsRaw.scope
      : 'context'
  const contextRaw = Number(paramsRaw.contextWindow)
  const contextWindow = CONTEXT_WINDOW_VALUES.includes(contextRaw)
    ? contextRaw
    : DEFAULT_CONTEXT_WINDOW
  const aggregateBy = normalizeAggregateDimensions(paramsRaw.aggregateBy)
  const savedAt = typeof parsed.savedAt === 'string' ? parsed.savedAt : ''

  return {
    mode,
    result: parsed.result,
    params: {
      sourceId,
      preset,
      customStart,
      customEnd,
      maxLogs,
      docId,
      scope,
      contextWindow,
      aggregateBy,
    },
    savedAt,
  }
}

function storageKey(projectID: string): string {
  return `${STORAGE_PREFIX}${projectID}`
}

function sourceKindLabel(source: LogSource): string {
  const rawType = String(source.type ?? '').trim()
  const haystack = `${rawType} ${source.parse_mode ?? ''} ${source.path ?? ''}`.toLowerCase()
  if (/evtx|windows|winevt|eventlog/.test(haystack)) return 'Windows 事件日志'
  if (/nginx/.test(haystack)) return 'Nginx'
  if (/apache|httpd/.test(haystack)) return 'Apache'
  if (/ssh|secure|auth\.log/.test(haystack)) return 'SSH'
  if (/journal|systemd/.test(haystack)) return 'Systemd Journal'
  if (/syslog/.test(haystack)) return 'Syslog'
  if (/mysql|postgres|mariadb|database|\bsql\b/.test(haystack)) return '数据库日志'
  if (/docker|container|k8s|kubernetes/.test(haystack)) return '容器日志'
  return rawType || '日志源'
}

function sourceOptionLabel(source: LogSource): string {
  const kind = sourceKindLabel(source)
  if (kind === 'SSH' && source.host) return `${kind} · ${source.host}`
  const detail =
    basename(source.path) || source.template_id || source.host || source.id
  return `${kind} · ${detail}`
}

function anomalyScore(value: number | undefined | null): string {
  const n = Number(value ?? 0)
  return Number.isFinite(n) ? n.toFixed(2) : '—'
}

function BulletList(props: { items: string[]; empty: string }): JSX.Element {
  return (
    <Show
      when={props.items?.length > 0}
      fallback={<div class="mono-sm faint">{props.empty}</div>}
    >
      <ul
        style={{
          margin: 0,
          padding: '0 0 0 16px',
          'font-size': '11px',
          'line-height': '1.7',
          color: 'var(--text-muted)',
          'word-break': 'break-word',
        }}
      >
        <For each={props.items}>{(item) => <li>{item}</li>}</For>
      </ul>
    </Show>
  )
}

/* ------------------------------------------------------------------ */
/* 组件                                                                 */
/* ------------------------------------------------------------------ */

export interface LogAnalysisAIPanelProps {
  open: boolean
  projectID: string
  projectName?: string
  onClose: () => void
}

/**
 * 日志分析 AI 面板：
 * - 聚合分析：按日志类型与时间范围调用异常检测接口；
 * - 逐条分析：加载候选日志后选择一条，单独或结合上下文分析。
 * 最近一次成功分析会按项目持久化到 localStorage，打开时只恢复展示，绝不自动调用 AI。
 */
export function LogAnalysisAIPanel(props: LogAnalysisAIPanelProps): JSX.Element {
  const [sources, setSources] = createSignal<LogSource[]>([])
  const [sourcesLoading, setSourcesLoading] = createSignal(false)
  const [sourcesError, setSourcesError] = createSignal<string | null>(null)
  const [sourceId, setSourceId] = createSignal('')

  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [providersLoading, setProvidersLoading] = createSignal(false)
  const [providersError, setProvidersError] = createSignal<string | null>(null)
  const [provider, setProvider] = createSignal('')

  const [preset, setPreset] = createSignal<TimePreset>('24h')
  const [customStart, setCustomStart] = createSignal('')
  const [customEnd, setCustomEnd] = createSignal('')
  const [mode, setMode] = createSignal<AnalysisMode>('aggregate')
  const [maxLogs, setMaxLogs] = createSignal(DEFAULT_MAX_LOGS)
  const [aggregateDims, setAggregateDims] = createSignal<string[]>([
    ...DEFAULT_AGGREGATE_DIMENSIONS,
  ])

  const [candidates, setCandidates] = createSignal<LogEntry[]>([])
  const [candidateTotal, setCandidateTotal] = createSignal(0)
  const [candidatesLoading, setCandidatesLoading] = createSignal(false)
  const [candidatesError, setCandidatesError] = createSignal<string | null>(null)
  const [candidatesLoaded, setCandidatesLoaded] = createSignal(false)
  const [selectedDocID, setSelectedDocID] = createSignal<number | null>(null)
  const [scope, setScope] = createSignal<EntryScope>('context')
  const [contextWindow, setContextWindow] = createSignal(DEFAULT_CONTEXT_WINDOW)
  const [question, setQuestion] = createSignal('')

  const [running, setRunning] = createSignal(false)
  const [aggregateError, setAggregateError] = createSignal<string | null>(null)
  const [entryError, setEntryError] = createSignal<string | null>(null)

  const [result, setResult] = createSignal<AnalysisResultState | null>(null)
  const [savedAt, setSavedAt] = createSignal('')
  const [restored, setRestored] = createSignal(false)
  const [restoreError, setRestoreError] = createSignal<string | null>(null)

  let providerSeq = 0
  let sourceSeq = 0
  let candidateSeq = 0
  let runSeq = 0

  const configuredProviders = () => providers().filter((item) => item.configured)
  const selectedProvider = () =>
    configuredProviders().find((item) => item.provider === provider())

  const providerOptions = () => [
    {
      value: '',
      label: providersLoading()
        ? '加载 AI 服务…'
        : configuredProviders().length === 0
          ? '无可用 Provider'
          : '选择 Provider…',
    },
    ...configuredProviders().map((item) => ({
      value: item.provider,
      label: `${item.name || item.provider}${item.model ? ` · ${item.model}` : ''}${
        item.enabled ? '' : '（已停用）'
      }`,
    })),
  ]

  const sourceOptions = () => [
    { value: '', label: '全部日志' },
    ...sources().map((item) => ({ value: item.id, label: sourceOptionLabel(item) })),
  ]

  const customRangeError = () => {
    if (preset() !== 'custom') return ''
    const start = localInputToIso(customStart())
    const end = localInputToIso(customEnd())
    if (start && end && new Date(start).getTime() > new Date(end).getTime()) {
      return '自定义时间范围无效：开始时间晚于结束时间。'
    }
    return ''
  }

  const rangeSummary = () => {
    if (preset() === 'all') return '不限制时间范围'
    if (customRangeError()) return '时间范围无效'
    const bounds = resolveTimeBounds(
      preset(),
      customStart(),
      customEnd(),
      Date.now(),
    )
    if (!bounds.start && !bounds.end) return '未设置时间范围'
    return `${bounds.start ? formatTime(bounds.start) : '不限'} → ${
      bounds.end ? formatTime(bounds.end) : '不限'
    }`
  }

  const aggregateResult = () => {
    const current = result()
    return current && current.mode === 'aggregate' ? current.data : null
  }

  const entryResult = () => {
    const current = result()
    return current && current.mode === 'entry' ? current.data : null
  }

  const selectedCandidate = () => {
    const docID = selectedDocID()
    if (docID == null) return null
    return candidates().find((entry) => entry.doc_id === docID) ?? null
  }

  const otherModeHint = () => {
    const current = result()
    if (!current || current.mode === mode()) return ''
    return current.mode === 'aggregate'
      ? '最近一次分析为聚合分析结果，切换到“聚合分析”可查看。'
      : '最近一次分析为逐条分析结果，切换到“逐条分析”可查看。'
  }

  const canRunBase = () =>
    Boolean(props.projectID) &&
    Boolean(provider()) &&
    configuredProviders().length > 0 &&
    !providersLoading() &&
    !sourcesLoading() &&
    !running() &&
    !customRangeError()

  const canRunAggregate = () => canRunBase() && aggregateDims().length > 0

  const canRunEntry = () => canRunBase() && selectedDocID() != null

  function buildScopedQuery(limit: number): Query {
    const bounds = resolveTimeBounds(
      preset(),
      customStart(),
      customEnd(),
      Date.now(),
    )
    const query: Query = {
      limit,
      sort_by: 'timestamp',
      sort_order: 'desc',
    }
    const activeSource = sourceId()
    if (activeSource) query.sources = [activeSource]
    if (bounds.start) query.start_time = bounds.start
    if (bounds.end) query.end_time = bounds.end
    return query
  }

  function currentParams(docID: number | null): StoredParams {
    return {
      sourceId: sourceId(),
      preset: preset(),
      customStart: customStart(),
      customEnd: customEnd(),
      maxLogs: maxLogs(),
      docId: docID,
      scope: scope(),
      contextWindow: contextWindow(),
      aggregateBy: aggregateDims(),
    }
  }

  async function loadProviders() {
    const seq = ++providerSeq
    setProvidersLoading(true)
    setProvidersError(null)
    try {
      const list = await api.ai.providers()
      if (seq !== providerSeq) return
      const items = Array.isArray(list) ? list : []
      setProviders(items)
      const usable = items.filter((item) => item.configured)
      if (usable.length === 0) {
        setProvider('')
        return
      }
      const current = provider()
      if (!usable.some((item) => item.provider === current)) {
        const preferred = usable.find((item) => item.enabled) ?? usable[0]
        setProvider(preferred.provider)
      }
    } catch (err) {
      if (seq !== providerSeq) return
      setProviders([])
      setProvider('')
      setProvidersError(errorText(err))
    } finally {
      if (seq === providerSeq) setProvidersLoading(false)
    }
  }

  async function loadSources() {
    const projectID = props.projectID
    if (!projectID) return
    const seq = ++sourceSeq
    setSourcesLoading(true)
    setSourcesError(null)
    try {
      const list = await api.projects.sources(projectID)
      if (seq !== sourceSeq || props.projectID !== projectID) return
      const items = Array.isArray(list) ? list : []
      setSources(items)
      const current = sourceId()
      // 记录中引用的日志源可能已被删除，避免继续带着失效 ID 查询。
      if (current && !items.some((item) => item.id === current)) setSourceId('')
    } catch (err) {
      if (seq !== sourceSeq) return
      setSourcesError(errorText(err))
    } finally {
      if (seq === sourceSeq) setSourcesLoading(false)
    }
  }

  function loadPersisted(projectID: string) {
    let raw: string | null = null
    try {
      raw = window.localStorage.getItem(storageKey(projectID))
    } catch {
      raw = null
    }
    if (!raw) return
    try {
      const parsed = parseStoredAnalysis(raw)
      if (props.projectID !== projectID) return
      setMode(parsed.mode)
      setSourceId(parsed.params.sourceId)
      setPreset(parsed.params.preset)
      setCustomStart(parsed.params.customStart)
      setCustomEnd(parsed.params.customEnd)
      setMaxLogs(parsed.params.maxLogs)
      setSelectedDocID(parsed.params.docId)
      setScope(parsed.params.scope)
      setContextWindow(parsed.params.contextWindow)
      setAggregateDims([...parsed.params.aggregateBy])
      setResult(
        parsed.mode === 'aggregate'
          ? { mode: 'aggregate', data: normalizeAnomalyResult(parsed.result) }
          : { mode: 'entry', data: normalizeEntryResult(parsed.result) },
      )
      setSavedAt(parsed.savedAt)
      setRestored(true)
    } catch {
      if (props.projectID !== projectID) return
      setRestoreError('上次 AI 分析记录无法解析，已忽略。')
      try {
        window.localStorage.removeItem(storageKey(projectID))
      } catch {
        /* 忽略存储异常 */
      }
    }
  }

  function persistResult(
    projectID: string,
    analysisMode: AnalysisMode,
    data: AnomalyDetectionResult | LogAnalysisResult,
    params: StoredParams,
  ) {
    const stamp = new Date().toISOString()
    const record: StoredAnalysis = {
      mode: analysisMode,
      result: data,
      params,
      savedAt: stamp,
    }
    try {
      window.localStorage.setItem(storageKey(projectID), JSON.stringify(record))
    } catch {
      /* 存储不可用时仅保留内存结果 */
    }
    setSavedAt(stamp)
    setRestored(false)
    setRestoreError(null)
  }

  async function runAggregate() {
    const projectID = props.projectID
    const activeProvider = provider()
    const dims = aggregateDims()
    if (!projectID || !activeProvider || customRangeError()) return
    if (dims.length === 0) {
      setAggregateError('请至少选择一个聚合维度后再开始分析。')
      return
    }
    const params = currentParams(null)
    const seq = ++runSeq
    setRunning(true)
    setAggregateError(null)
    setResult(null)
    setRestored(false)
    try {
      const data = await api.ai.detectAnomaly({
        project_id: projectID,
        provider: activeProvider,
        query: buildScopedQuery(maxLogs()),
        max_logs: maxLogs(),
        aggregate_by: dims,
      })
      if (seq !== runSeq || !props.open || props.projectID !== projectID) return
      setResult({ mode: 'aggregate', data })
      persistResult(projectID, 'aggregate', data, params)
    } catch (err) {
      if (seq !== runSeq) return
      setAggregateError(errorText(err))
    } finally {
      if (seq === runSeq) setRunning(false)
    }
  }

  async function loadCandidates() {
    const projectID = props.projectID
    if (!projectID || customRangeError()) return
    const seq = ++candidateSeq
    setCandidatesLoading(true)
    setCandidatesError(null)
    setCandidatesLoaded(false)
    setCandidates([])
    setCandidateTotal(0)
    try {
      const data = await api.query.search(
        projectID,
        buildScopedQuery(CANDIDATE_LIMIT),
      )
      if (seq !== candidateSeq || !props.open || props.projectID !== projectID) return
      const hits = Array.isArray(data?.hits) ? data.hits : []
      setCandidates(hits)
      setCandidateTotal(Number(data?.total) || 0)
      setCandidatesLoaded(true)
      const current = selectedDocID()
      if (current != null && !hits.some((entry) => entry.doc_id === current)) {
        setSelectedDocID(null)
      }
    } catch (err) {
      if (seq !== candidateSeq) return
      setCandidatesError(errorText(err))
    } finally {
      if (seq === candidateSeq) setCandidatesLoading(false)
    }
  }

  async function runEntryAnalysis() {
    const projectID = props.projectID
    const activeProvider = provider()
    const docID = selectedDocID()
    if (!projectID || !activeProvider || docID == null || customRangeError()) return
    const contextSize = scope() === 'single' ? 0 : contextWindow()
    const params = currentParams(docID)
    const seq = ++runSeq
    setRunning(true)
    setEntryError(null)
    setResult(null)
    setRestored(false)
    try {
      const questionText = question().trim()
      const data = await api.ai.analyzeLogEntry({
        project_id: projectID,
        doc_id: docID,
        provider: activeProvider,
        context_before: contextSize,
        context_after: contextSize,
        ...(questionText ? { question: questionText } : {}),
      })
      if (seq !== runSeq || !props.open || props.projectID !== projectID) return
      setResult({ mode: 'entry', data })
      persistResult(projectID, 'entry', data, params)
    } catch (err) {
      if (seq !== runSeq) return
      setEntryError(errorText(err))
    } finally {
      if (seq === runSeq) setRunning(false)
    }
  }

  function openSettings() {
    props.onClose()
    navigate('settings')
  }

  function openDoc(docID: number) {
    props.onClose()
    openLogDetail(docID)
  }

  function handlePresetChange(value: string) {
    if (running()) return
    const next = value as TimePreset
    setPreset(next)
    if (next === 'custom' && !customStart() && !customEnd()) {
      const now = new Date()
      setCustomStart(
        toLocalInput(new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString()),
      )
      setCustomEnd(toLocalInput(now.toISOString()))
    }
  }

  function handleModeChange(value: string) {
    if (running()) return
    setMode(value as AnalysisMode)
  }

  function toggleAggregateDim(value: string) {
    if (running()) return
    setAggregateDims((current) => {
      if (current.includes(value)) {
        return current.filter((item) => item !== value)
      }
      return AGGREGATE_DIMENSION_VALUES.filter(
        (item) => current.includes(item) || item === value,
      )
    })
  }

  createEffect(
    on(
      () => [props.open, props.projectID] as const,
      ([isOpen, projectID]) => {
        // 关闭、切换项目时作废所有进行中的请求。
        providerSeq++
        sourceSeq++
        candidateSeq++
        runSeq++
        setRunning(false)
        setCandidatesLoading(false)
        setProvidersLoading(false)
        setSourcesLoading(false)
        setAggregateDims([...DEFAULT_AGGREGATE_DIMENSIONS])
        if (!isOpen) return

        setSources([])
        setSourcesError(null)
        setSourceId('')
        setProviders([])
        setProvidersError(null)
        setProvider('')
        setPreset('24h')
        setCustomStart('')
        setCustomEnd('')
        setMode('aggregate')
        setMaxLogs(DEFAULT_MAX_LOGS)
        setAggregateDims([...DEFAULT_AGGREGATE_DIMENSIONS])
        setCandidates([])
        setCandidateTotal(0)
        setCandidatesError(null)
        setCandidatesLoaded(false)
        setSelectedDocID(null)
        setScope('context')
        setContextWindow(DEFAULT_CONTEXT_WINDOW)
        setQuestion('')
        setAggregateError(null)
        setEntryError(null)
        setResult(null)
        setSavedAt('')
        setRestored(false)
        setRestoreError(null)

        if (!projectID) return
        void loadProviders()
        void loadSources()
        loadPersisted(projectID)
      },
    ),
  )

  const projectLabel = () => props.projectName || '当前项目'
  const hasConfiguredProviders = () => configuredProviders().length > 0

  return (
    <Modal
      open={props.open}
      title="AI 日志分析"
      subtitle={`${projectLabel()} · 聚合异常检测与逐条日志分析`}
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={openSettings}>
            AI 设置
          </Button>
          <span class="grow" />
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '14px' }}>
        <Show when={restoreError()}>
          <div class="detail-note">{restoreError()}</div>
        </Show>

        <div class="form-row">
          <Field
            label="日志类型"
            hint={
              sourcesLoading()
                ? '加载中…'
                : sources().length > 0
                  ? `${sources().length} 个来源`
                  : undefined
            }
          >
            <Select
              value={sourceId()}
              options={sourceOptions()}
              disabled={running() || sourcesLoading()}
              onInput={setSourceId}
            />
          </Field>

          <Field
            label="AI Provider"
            hint={
              providersLoading()
                ? '加载中…'
                : selectedProvider()?.model || undefined
            }
          >
            <div class="row" style={{ gap: '6px' }}>
              <div style={{ flex: 1, 'min-width': 0 }}>
                <Select
                  value={provider()}
                  options={providerOptions()}
                  disabled={
                    running() ||
                    providersLoading() ||
                    configuredProviders().length === 0
                  }
                  onInput={setProvider}
                />
              </div>
              <Button
                size="sm"
                variant="ghost"
                loading={providersLoading()}
                disabled={running()}
                onClick={() => void loadProviders()}
              >
                <Icon name="refresh" size={12} />
                刷新
              </Button>
            </div>
          </Field>
        </div>

        <PageBusy visible={sourcesLoading()} text="正在读取项目日志源…" />

        <Show when={sourcesError()}>
          <div class="danger-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span class="grow" style={{ 'line-height': '1.6' }}>
                日志源加载失败：{sourcesError()}
              </span>
              <Button size="sm" variant="ghost" onClick={() => void loadSources()}>
                重试
              </Button>
            </div>
          </div>
        </Show>

        <Show
          when={
            !sourcesLoading() &&
            !sourcesError() &&
            sources().length === 0 &&
            Boolean(props.projectID)
          }
        >
          <div class="detail-note">
            当前项目暂无可用的日志源，AI 分析将覆盖项目中的全部日志。
          </div>
        </Show>

        <Show when={providersError()}>
          <div class="danger-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span class="grow" style={{ 'line-height': '1.6' }}>
                AI 服务读取失败：{providersError()}
              </span>
              <div class="row" style={{ gap: '6px' }}>
                <Button size="sm" variant="ghost" onClick={() => void loadProviders()}>
                  重试
                </Button>
                <Button size="sm" variant="ghost" onClick={openSettings}>
                  前往设置
                </Button>
              </div>
            </div>
          </div>
        </Show>

        <Show
          when={
            !providersLoading() &&
            !providersError() &&
            !hasConfiguredProviders()
          }
        >
          <div class="detail-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span class="grow" style={{ 'line-height': '1.6' }}>
                尚未配置可用的 AI 服务。请前往设置 → AI 服务填写 Provider
                配置与 API Key 后再开始分析。
              </span>
              <Button size="sm" variant="primary" onClick={openSettings}>
                前往设置
              </Button>
            </div>
          </div>
        </Show>

        <Field label="时间范围" hint={rangeSummary()}>
          <Segmented
            options={TIME_PRESET_OPTIONS}
            value={preset()}
            onChange={handlePresetChange}
          />
          <Show when={preset() === 'custom'}>
            <div class="row wrap" style={{ gap: '6px', 'margin-top': '7px' }}>
              <input
                type="datetime-local"
                class="input"
                style={rangeInputStyle}
                value={customStart()}
                title="开始时间（本地时间）"
                aria-label="自定义开始时间"
                disabled={running()}
                onInput={(event) => setCustomStart(event.currentTarget.value)}
              />
              <span class="faint mono-sm">→</span>
              <input
                type="datetime-local"
                class="input"
                style={rangeInputStyle}
                value={customEnd()}
                title="结束时间（本地时间）"
                aria-label="自定义结束时间"
                disabled={running()}
                onInput={(event) => setCustomEnd(event.currentTarget.value)}
              />
            </div>
          </Show>
          <Show when={customRangeError()}>
            <div class="mono-sm ember" style={{ 'margin-top': '5px' }}>
              {customRangeError()}
            </div>
          </Show>
        </Field>

        <Field label="分析模式">
          <Segmented
            options={ANALYSIS_MODE_OPTIONS}
            value={mode()}
            onChange={handleModeChange}
          />
        </Field>

        <Show when={otherModeHint()}>
          <div class="detail-note">{otherModeHint()}</div>
        </Show>

        <Show when={mode() === 'aggregate'}>
          <div
            class="panel"
            style={{ padding: '11px 12px', 'box-shadow': 'none' }}
          >
            <div class="row between wrap" style={{ gap: '9px' }}>
              <Field label="最大分析日志数" hint="仅取时间倒序的前 N 条">
                <div style={{ width: '150px' }}>
                  <Select
                    value={String(maxLogs())}
                    options={MAX_LOG_SELECT_OPTIONS}
                    disabled={running()}
                    onInput={(value) => {
                      const next = Number(value)
                      if (MAX_LOG_OPTIONS.includes(next)) setMaxLogs(next)
                    }}
                  />
                </div>
              </Field>
              <Button
                size="sm"
                variant="primary"
                loading={running()}
                disabled={!canRunAggregate()}
                onClick={() => void runAggregate()}
              >
                <Icon name="zap" size={12} />
                开始聚合 AI 分析
              </Button>
            </div>

            <Field
              label="聚合维度"
              hint={
                aggregateDims().length === 0
                  ? '至少选择一项维度'
                  : `${aggregateDims().length} 项`
              }
            >
              <div class="row wrap" style={{ gap: '5px', 'margin-top': '4px' }}>
                <For each={AGGREGATE_DIMENSION_OPTIONS}>
                  {(item) => (
                    <button
                      type="button"
                      class={`chip ${aggregateDims().includes(item.value) ? 'selected' : ''}`}
                      title={item.title}
                      disabled={running()}
                      onClick={() => toggleAggregateDim(item.value)}
                    >
                      {item.label}
                    </button>
                  )}
                </For>
              </div>
            </Field>
            <Show when={aggregateDims().length === 0}>
              <div class="mono-sm ember" style={{ 'margin-top': '5px' }}>
                请至少选择一个聚合维度。
              </div>
            </Show>
            <div class="mono-sm faint" style={{ 'margin-top': '5px', 'line-height': '1.6' }}>
              聚合统计覆盖所选时间/日志类型范围的全部日志，AI 仅采样最多{' '}
              {formatInt(maxLogs())} 条；维度越多耗时越长。
            </div>

            <Show when={!running() && !aggregateResult() && !aggregateError()}>
              <div class="detail-note" style={{ 'margin-top': '9px' }}>
                按所选日志类型与时间范围提取日志，交由 AI
                识别异常模式、风险等级与处置建议。点击结果中的 DocID
                可直接打开对应日志。
              </div>
            </Show>

            <PageBusy
              visible={running()}
              text="AI 正在聚合分析日志，可能需要数十秒…"
            />

            <Show when={aggregateError()}>
              <div class="danger-note" style={{ 'margin-top': '9px' }}>
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <span class="grow" style={{ 'line-height': '1.6' }}>
                    {aggregateError()}
                  </span>
                  <Button size="sm" variant="ghost" onClick={openSettings}>
                    检查 AI 设置
                  </Button>
                </div>
              </div>
            </Show>

            <Show when={aggregateResult()}>
              {(resultAccessor) => {
                const data = resultAccessor()
                const anomalies = data.anomalies ?? []
                return (
                  <div class="stack" style={{ gap: '10px', 'margin-top': '10px' }}>
                    <div
                      class="row wrap"
                      style={{ gap: '6px', 'align-items': 'center' }}
                    >
                      <Badge tone={severityTone(data.risk_level)}>
                        风险 {data.risk_level || '未知'}
                      </Badge>
                      <Badge tone="muted">
                        {data.provider || provider() || 'AI'}
                      </Badge>
                      <span class="mono-sm faint">
                        已分析 {formatInt(data.analyzed)} 条 ·{' '}
                        {formatTime(data.generated_at)}
                      </span>
                      <Show when={restored()}>
                        <Badge tone="sky">已恢复上次结果</Badge>
                      </Show>
                    </div>

                    <div
                      class="success-note"
                      style={{ 'line-height': '1.7', 'white-space': 'pre-wrap' }}
                    >
                      {data.summary || 'AI 未返回摘要。'}
                    </div>

                    <div class="form-row">
                      <div>
                        <div class="section-label">发现</div>
                        <BulletList
                          items={data.findings ?? []}
                          empty="未返回发现项。"
                        />
                      </div>
                      <div>
                        <div class="section-label">处置建议</div>
                        <BulletList
                          items={data.suggestions ?? []}
                          empty="未返回处置建议。"
                        />
                      </div>
                    </div>

                    <Show when={(data.aggregates?.length ?? 0) > 0}>
                      <div>
                        <div class="section-label">聚合统计</div>
                        <div class="stack" style={{ gap: '8px' }}>
                          <For each={data.aggregates ?? []}>
                            {(dimension) => {
                              const total = () => Number(dimension.total) || 0
                              const items = () => dimension.items ?? []
                              return (
                                <div
                                  style={{
                                    border: '1px solid var(--border)',
                                    'border-radius': 'var(--r-md)',
                                    overflow: 'hidden',
                                  }}
                                >
                                  <div
                                    class="row between wrap"
                                    style={{
                                      gap: '8px',
                                      padding: '6px 9px',
                                      background: 'var(--surface-soft)',
                                      'border-bottom': '1px solid var(--divider)',
                                    }}
                                  >
                                    <span class="mono-sm">
                                      {aggregateDimensionLabel(dimension.field)}
                                    </span>
                                    <span class="mono-sm faint">
                                      总匹配 {formatInt(total())} 条
                                    </span>
                                  </div>
                                  <Show
                                    when={items().length > 0}
                                    fallback={
                                      <div
                                        class="mono-sm faint"
                                        style={{ padding: '6px 9px' }}
                                      >
                                        该维度没有可展示的聚合数据。
                                      </div>
                                    }
                                  >
                                    <div
                                      class="scrollable scroll-thin"
                                      style={{
                                        'max-height': '168px',
                                        overflow: 'auto',
                                      }}
                                    >
                                      <table
                                        class="data-table"
                                        style={{ 'font-size': '10.5px' }}
                                      >
                                        <thead>
                                          <tr>
                                            <th>取值</th>
                                            <th style={{ width: '72px' }}>数量</th>
                                            <th style={{ width: '72px' }}>占比</th>
                                          </tr>
                                        </thead>
                                        <tbody>
                                          <For each={items()}>
                                            {(item) => (
                                              <tr>
                                                <td
                                                  style={{ 'word-break': 'break-word' }}
                                                  title={item.key}
                                                >
                                                  {truncate(item.key, 200) || '(空)'}
                                                </td>
                                                <td class="mono-sm">
                                                  {formatInt(item.count)}
                                                </td>
                                                <td class="mono-sm faint">
                                                  {aggregatePercentage(
                                                    Number(item.count),
                                                    total(),
                                                  )}
                                                </td>
                                              </tr>
                                            )}
                                          </For>
                                        </tbody>
                                      </table>
                                    </div>
                                  </Show>
                                </div>
                              )
                            }}
                          </For>
                        </div>
                      </div>
                    </Show>

                    <div>
                      <div class="section-label">
                        异常条目（{anomalies.length}）
                      </div>
                      <Show
                        when={anomalies.length > 0}
                        fallback={
                          <div class="mono-sm faint">
                            未返回具体异常条目，请参考上方发现与建议。
                          </div>
                        }
                      >
                        <div
                          class="scrollable scroll-thin"
                          style={{
                            'max-height': '320px',
                            overflow: 'auto',
                            border: '1px solid var(--border)',
                            'border-radius': 'var(--r-md)',
                          }}
                        >
                          <table class="data-table" style={{ 'font-size': '10.5px' }}>
                            <thead>
                              <tr>
                                <th style={{ 'min-width': '112px' }}>时间</th>
                                <th style={{ 'min-width': '64px' }}>级别</th>
                                <th style={{ 'min-width': '180px' }}>消息</th>
                                <th style={{ 'min-width': '150px' }}>原因</th>
                                <th style={{ 'min-width': '52px' }}>评分</th>
                                <th style={{ 'min-width': '90px' }}>DocID</th>
                              </tr>
                            </thead>
                            <tbody>
                              <For each={anomalies}>
                                {(anomaly) => (
                                  <tr>
                                    <td
                                      class="mono-sm faint"
                                      style={{ 'white-space': 'nowrap' }}
                                    >
                                      {formatTime(anomaly.timestamp)}
                                    </td>
                                    <td>
                                      <LevelBadge level={anomaly.level} />
                                    </td>
                                    <td
                                      style={{ 'word-break': 'break-word' }}
                                      title={anomaly.message}
                                    >
                                      {truncate(anomaly.message, 260) || '(空消息)'}
                                    </td>
                                    <td
                                      class="mono-sm"
                                      style={{ 'word-break': 'break-word' }}
                                      title={anomaly.reason}
                                    >
                                      {truncate(anomaly.reason, 200) || '未说明'}
                                    </td>
                                    <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                                      {anomalyScore(anomaly.score)}
                                    </td>
                                    <td>
                                      <Show
                                        when={(anomaly.doc_ids?.length ?? 0) > 0}
                                        fallback={<span class="faint">—</span>}
                                      >
                                        <div class="row wrap" style={{ gap: '3px' }}>
                                          <For each={anomaly.doc_ids ?? []}>
                                            {(docID) => (
                                              <button
                                                type="button"
                                                class="chip"
                                                style={{
                                                  padding: '1px 6px',
                                                  'font-size': '9.5px',
                                                }}
                                                title={`打开日志 #${docID}`}
                                                onClick={() => openDoc(docID)}
                                              >
                                                #{docID}
                                              </button>
                                            )}
                                          </For>
                                        </div>
                                      </Show>
                                    </td>
                                  </tr>
                                )}
                              </For>
                            </tbody>
                          </table>
                        </div>
                      </Show>
                    </div>

                    <Show when={restored() && savedAt()}>
                      <div class="mono-sm faint">
                        结果保存于 {formatTime(savedAt())}，本次打开未重新调用 AI。
                      </div>
                    </Show>
                  </div>
                )
              }}
            </Show>
          </div>
        </Show>

        <Show when={mode() === 'entry'}>
          <div
            class="panel"
            style={{ padding: '11px 12px', 'box-shadow': 'none' }}
          >
            <div class="row between wrap" style={{ gap: '9px' }}>
              <span class="mono-sm faint" style={{ 'line-height': '1.6' }}>
                按当前日志类型与时间范围加载候选日志，点击列表中的一行进行选择。
              </span>
              <Button
                size="sm"
                variant="default"
                loading={candidatesLoading()}
                disabled={
                  running() ||
                  sourcesLoading() ||
                  Boolean(customRangeError())
                }
                onClick={() => void loadCandidates()}
              >
                <Icon name="list" size={12} />
                加载候选日志
              </Button>
            </div>

            <PageBusy
              visible={candidatesLoading()}
              text="正在加载候选日志…"
            />

            <Show when={candidatesError()}>
              <div class="danger-note" style={{ 'margin-top': '9px' }}>
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <span class="grow" style={{ 'line-height': '1.6' }}>
                    {candidatesError()}
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => void loadCandidates()}
                  >
                    重试
                  </Button>
                </div>
              </div>
            </Show>

            <Show
              when={candidates().length > 0}
              fallback={
                <Show
                  when={
                    candidatesLoaded() &&
                    !candidatesLoading() &&
                    !candidatesError()
                  }
                >
                  <EmptyState
                    icon="search"
                    title="没有候选日志"
                    text="当前日志类型与时间范围内没有命中日志，请调整时间范围或日志类型后重试。"
                  />
                </Show>
              }
            >
              <div
                class="scrollable scroll-thin"
                style={{
                  'max-height': '242px',
                  overflow: 'auto',
                  'margin-top': '9px',
                  border: '1px solid var(--border)',
                  'border-radius': 'var(--r-md)',
                }}
              >
                <For each={candidates()}>
                  {(entry) => {
                    const selected = () => selectedDocID() === entry.doc_id
                    return (
                      <button
                        type="button"
                        aria-pressed={selected()}
                        disabled={running()}
                        title={`选择日志 #${entry.doc_id}：${truncate(
                          friendlyLogMessage(entry.message),
                          500,
                        )}`}
                        style={{
                          display: 'flex',
                          width: '100%',
                          'align-items': 'center',
                          gap: '8px',
                          padding: '6px 9px',
                          border: 'none',
                          'border-bottom': '1px solid var(--divider)',
                          'border-left': selected()
                            ? '2px solid var(--accent)'
                            : '2px solid transparent',
                          background: selected()
                            ? 'var(--accent-ghost)'
                            : 'transparent',
                          color: 'var(--text)',
                          'text-align': 'left',
                          cursor: running() ? 'not-allowed' : 'pointer',
                          opacity: running() ? 0.72 : 1,
                          'font-family': 'var(--font-mono)',
                          'font-size': '10.5px',
                        }}
                        onClick={() => {
                          if (!running()) setSelectedDocID(entry.doc_id)
                        }}
                      >
                        <span
                          class="faint"
                          style={{ width: '118px', flex: 'none' }}
                        >
                          {formatTime(entry.timestamp)}
                        </span>
                        <span style={{ flex: 'none' }}>
                          <LevelBadge level={entry.level} />
                        </span>
                        <span
                          class="truncate"
                          style={{ width: '92px', flex: 'none' }}
                          title={entry.source_host || '未知主机'}
                        >
                          {entry.source_host || '—'}
                        </span>
                        <span
                          class="faint"
                          style={{ width: '64px', flex: 'none' }}
                        >
                          <Show when={entry.event_id != null}>
                            #{entry.event_id}
                          </Show>
                        </span>
                        <span class="truncate grow" style={{ 'min-width': 0 }}>
                          {truncate(friendlyLogMessage(entry.message), 180) ||
                            '(空消息)'}
                        </span>
                        <Show when={selected()}>
                          <span style={{ color: 'var(--accent)', flex: 'none' }}>
                            <Icon name="check" size={12} />
                          </span>
                        </Show>
                      </button>
                    )
                  }}
                </For>
              </div>
              <div class="mono-sm faint" style={{ 'margin-top': '5px' }}>
                共 {formatInt(candidateTotal() || candidates().length)} 条命中，已加载{' '}
                {formatInt(candidates().length)} 条（最多 {CANDIDATE_LIMIT} 条）
              </div>
            </Show>

            <Show when={selectedDocID() != null}>
              <div class="detail-note" style={{ 'margin-top': '9px' }}>
                已选择日志 #{selectedDocID()}
                <Show when={selectedCandidate()}>
                  {(entryAccessor) => {
                    const entry = entryAccessor()
                    return (
                      <>
                        {' '}
                        · {formatTime(entry.timestamp)} · {entry.level} ·{' '}
                        {entry.source_host || '未知主机'}
                      </>
                    )
                  }}
                </Show>
                <Show when={!selectedCandidate()}>
                  {' '}
                  （来自上次分析记录；重新加载候选日志可确认日志内容）
                </Show>
              </div>
            </Show>

            <div class="form-row" style={{ 'margin-top': '10px' }}>
              <Field label="分析范围">
                <Segmented
                  options={ENTRY_SCOPE_OPTIONS}
                  value={scope()}
                  onChange={(value) => {
                    if (running()) return
                    setScope(value as EntryScope)
                  }}
                />
              </Field>
              <Field
                label="上下文窗口"
                hint={
                  scope() === 'single'
                    ? '仅提交当前一条日志'
                    : `前后各 ${contextWindow()} 条（最多约 ${contextWindow() * 2 + 1} 条，超长时围绕目标日志自动截断）`
                }
              >
                <Select
                  value={String(contextWindow())}
                  options={CONTEXT_WINDOW_OPTIONS}
                  disabled={running() || scope() === 'single'}
                  onInput={(value) => {
                    const next = Number(value)
                    if (CONTEXT_WINDOW_VALUES.includes(next)) setContextWindow(next)
                  }}
                />
              </Field>
            </div>

            <Field label="补充问题" hint="可选，用于引导 AI 关注点">
              <TextArea
                value={question()}
                rows={3}
                placeholder="例如：这条日志是否代表一次暴力破解或横向移动尝试？"
                disabled={running()}
                onInput={setQuestion}
              />
            </Field>

            <div
              class="row between wrap"
              style={{ gap: '9px', 'margin-top': '4px' }}
            >
              <span class="mono-sm faint" style={{ 'line-height': '1.6' }}>
                逐条分析会把所选日志及上下文提交给 AI，并返回风险结论与建议。
              </span>
              <Button
                size="sm"
                variant="primary"
                loading={running()}
                disabled={!canRunEntry()}
                onClick={() => void runEntryAnalysis()}
              >
                <Icon name="wand" size={12} />
                开始 AI 分析
              </Button>
            </div>

            <PageBusy
              visible={running()}
              text="AI 正在分析所选日志，可能需要数十秒…"
            />

            <Show when={entryError()}>
              <div class="danger-note" style={{ 'margin-top': '9px' }}>
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <span class="grow" style={{ 'line-height': '1.6' }}>
                    {entryError()}
                  </span>
                  <Button size="sm" variant="ghost" onClick={openSettings}>
                    检查 AI 设置
                  </Button>
                </div>
              </div>
            </Show>

            <Show when={entryResult()}>
              {(resultAccessor) => {
                const data = resultAccessor()
                const related = data.related_doc_ids ?? []
                return (
                  <div class="stack" style={{ gap: '10px', 'margin-top': '10px' }}>
                    <div
                      class="row wrap"
                      style={{ gap: '6px', 'align-items': 'center' }}
                    >
                      <Badge tone={severityTone(data.risk_level)}>
                        风险 {data.risk_level || '未知'}
                      </Badge>
                      <Badge tone="muted">
                        {data.provider || provider() || 'AI'}
                      </Badge>
                      <span class="mono-sm faint">
                        已分析 {formatInt(data.analyzed)} 条 ·{' '}
                        {formatTime(data.generated_at)}
                      </span>
                      <Show when={restored()}>
                        <Badge tone="sky">已恢复上次结果</Badge>
                      </Show>
                    </div>

                    <div
                      class="success-note"
                      style={{ 'line-height': '1.7', 'white-space': 'pre-wrap' }}
                    >
                      {data.summary || 'AI 未返回摘要。'}
                    </div>

                    <div class="form-row">
                      <div>
                        <div class="section-label">发现</div>
                        <BulletList
                          items={data.findings ?? []}
                          empty="未返回发现项。"
                        />
                      </div>
                      <div>
                        <div class="section-label">处置建议</div>
                        <BulletList
                          items={data.suggestions ?? []}
                          empty="未返回处置建议。"
                        />
                      </div>
                    </div>

                    <div>
                      <div class="section-label">相关日志</div>
                      <Show
                        when={related.length > 0}
                        fallback={
                          <div class="mono-sm faint">未返回相关日志。</div>
                        }
                      >
                        <div class="row wrap" style={{ gap: '4px' }}>
                          <For each={related}>
                            {(docID) => (
                              <button
                                type="button"
                                class="chip"
                                title={`打开日志 #${docID}`}
                                onClick={() => openDoc(docID)}
                              >
                                <Icon name="file" size={10} />#{docID}
                              </button>
                            )}
                          </For>
                        </div>
                      </Show>
                    </div>

                    <Show when={restored() && savedAt()}>
                      <div class="mono-sm faint">
                        结果保存于 {formatTime(savedAt())}，本次打开未重新调用 AI。
                      </div>
                    </Show>
                  </div>
                )
              }}
            </Show>
          </div>
        </Show>
      </div>
    </Modal>
  )
}
