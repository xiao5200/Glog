import { For, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { AIProviderInfo, Anomaly, AnomalyDetectionResult, Query } from '../../types'
import { api, errorText } from '../../api'
import { appConfig, navigate, notify } from '../../stores/app'
import {
  formatInt,
  formatTime,
  riskTone,
  severityTone,
  truncate,
} from '../../utils'
import {
  Badge,
  Button,
  Field,
  Icon,
  LevelBadge,
  PageBusy,
  Select,
  TextInput,
} from '../ui'

export interface ThreatAIPanelProps {
  projectId: string
  onOpenLog: (docId: number) => void
}

type RangePreset = '15m' | '1h' | '24h' | '7d' | 'all' | 'custom'
type PresetKey = Exclude<RangePreset, 'custom'>

const PRESET_ITEMS: Array<{ value: PresetKey; label: string; ms?: number }> = [
  { value: '15m', label: '近 15 分钟', ms: 15 * 60 * 1000 },
  { value: '1h', label: '近 1 小时', ms: 60 * 60 * 1000 },
  { value: '24h', label: '近 24 小时', ms: 24 * 60 * 60 * 1000 },
  { value: '7d', label: '近 7 天', ms: 7 * 24 * 60 * 60 * 1000 },
  { value: 'all', label: '全部' },
]

const MAX_LOG_OPTIONS = [50, 100, 200, 500].map((value) => ({
  value: String(value),
  label: `${value} 条`,
}))

const DEFAULT_MAX_LOGS = 200

const DETECTION_STORAGE_PREFIX = 'glog.threat.detection.v1.'

interface SavedDetectionState {
  result?: AnomalyDetectionResult
  provider?: string
  preset?: RangePreset
  customStart?: string
  customEnd?: string
  maxLogs?: number
  savedAt?: string
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

/** 将 Date 转为 datetime-local 输入的本地时间字符串。 */
function toLocalInput(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`
}

function riskToneOf(value: string | undefined): 'ember' | 'amber' | 'sky' | 'mint' {
  const text = String(value ?? '').trim()
  const numeric = Number(text)
  if (text && Number.isFinite(numeric)) return riskTone(numeric)
  return severityTone(text)
}

function scoreText(score: number | undefined): string {
  const value = Number(score)
  if (!Number.isFinite(value)) return '—'
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function isRangePreset(value: unknown): value is RangePreset {
  return (
    value === '15m' ||
    value === '1h' ||
    value === '24h' ||
    value === '7d' ||
    value === 'all' ||
    value === 'custom'
  )
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string')
}

function isDetectionResult(value: unknown): value is AnomalyDetectionResult {
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  if (typeof record.summary !== 'string') return false
  if (!isStringArray(record.findings) || !isStringArray(record.suggestions)) return false
  if (!Array.isArray(record.anomalies)) return false
  return record.anomalies.every((item: unknown) => {
    if (!item || typeof item !== 'object') return false
    const anomaly = item as Record<string, unknown>
    return anomaly.doc_ids === undefined || Array.isArray(anomaly.doc_ids)
  })
}

function detectionStorageKey(projectId: string): string {
  return `${DETECTION_STORAGE_PREFIX}${projectId}`
}

function readSavedDetection(projectId: string): SavedDetectionState | null {
  if (!projectId) return null
  try {
    const raw = localStorage.getItem(detectionStorageKey(projectId))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return null
    const record = parsed as Record<string, unknown>
    const maxLogs = Number(record.maxLogs)
    return {
      result: isDetectionResult(record.result) ? record.result : undefined,
      provider: typeof record.provider === 'string' ? record.provider : undefined,
      preset: isRangePreset(record.preset) ? record.preset : undefined,
      customStart:
        typeof record.customStart === 'string' ? record.customStart : undefined,
      customEnd: typeof record.customEnd === 'string' ? record.customEnd : undefined,
      maxLogs: Number.isFinite(maxLogs) && maxLogs > 0 ? maxLogs : undefined,
      savedAt: typeof record.savedAt === 'string' ? record.savedAt : undefined,
    }
  } catch {
    // 损坏或不可访问的本地存储视为无历史检测，忽略即可。
    return null
  }
}

function writeSavedDetection(projectId: string, state: SavedDetectionState): void {
  if (!projectId) return
  try {
    localStorage.setItem(detectionStorageKey(projectId), JSON.stringify(state))
  } catch {
    // 存储配额或隐私模式限制不影响检测结果展示。
  }
}

interface ResolvedRange {
  start?: string
  end?: string
  error?: string
}

export function ThreatAIPanel(props: ThreatAIPanelProps) {
  const initialEnd = new Date()
  const initialStart = new Date(initialEnd.getTime() - 24 * 60 * 60 * 1000)

  const [preset, setPreset] = createSignal<RangePreset>('24h')
  const [customStart, setCustomStart] = createSignal(toLocalInput(initialStart))
  const [customEnd, setCustomEnd] = createSignal(toLocalInput(initialEnd))
  const [rangeError, setRangeError] = createSignal('')

  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [providersLoading, setProvidersLoading] = createSignal(false)
  const [providersError, setProvidersError] = createSignal('')
  const [provider, setProvider] = createSignal('')

  const [maxLogs, setMaxLogs] = createSignal(DEFAULT_MAX_LOGS)

  const [busy, setBusy] = createSignal(false)
  const [error, setError] = createSignal('')
  const [result, setResult] = createSignal<AnomalyDetectionResult | null>(null)

  let providerSeq = 0
  let runSeq = 0
  let pendingProvider = ''

  const loadProviders = async () => {
    const seq = ++providerSeq
    setProvidersLoading(true)
    setProvidersError('')
    try {
      const list = await api.ai.providers()
      if (seq !== providerSeq) return
      const safe = (Array.isArray(list) ? list : []).filter(
        (item) => item && typeof item.provider === 'string' && item.provider,
      )
      setProviders(safe)
      const configured = safe.filter((item) => item.configured)
      const savedProvider = pendingProvider
      pendingProvider = ''
      setProvider((prev) => {
        if (savedProvider && configured.some((item) => item.provider === savedProvider)) {
          return savedProvider
        }
        if (prev && configured.some((item) => item.provider === prev)) return prev
        const preferred = configured.find((item) => item.enabled) ?? configured[0]
        return preferred?.provider ?? ''
      })
    } catch (err) {
      if (seq !== providerSeq) return
      const message = errorText(err)
      setProvidersError(message)
      notify('error', '加载 AI 提供商失败', message)
    } finally {
      if (seq === providerSeq) setProvidersLoading(false)
    }
  }

  const configuredProviders = createMemo(() =>
    providers()
      .filter((item) => item.configured)
      .slice()
      .sort((a, b) => {
        if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
        return (Number(b.priority) || 0) - (Number(a.priority) || 0)
      }),
  )

  const providerOptions = () => {
    const list = configuredProviders()
    if (list.length === 0) {
      return [
        {
          value: '',
          label: providersLoading() ? '加载 AI 提供商…' : '未配置可用提供商',
        },
      ]
    }
    return list.map((item) => ({
      value: item.provider,
      label: `${item.name || item.provider}${item.model ? ` · ${item.model}` : ''}${
        item.enabled ? '' : '（未启用）'
      }`,
    }))
  }

  const providerLabel = (key: string | undefined): string => {
    const value = String(key ?? '')
    if (!value) return '—'
    return providers().find((item) => item.provider === value)?.name || value
  }

  const resetRange = () => {
    const end = new Date()
    setPreset('24h')
    setCustomStart(toLocalInput(new Date(end.getTime() - 24 * 60 * 60 * 1000)))
    setCustomEnd(toLocalInput(end))
    setRangeError('')
  }

  const restoreSavedInputs = (saved: SavedDetectionState | null) => {
    if (saved?.result) setResult(saved.result)

    if (saved?.maxLogs) {
      const validMaxLogs = MAX_LOG_OPTIONS.some(
        (option) => Number(option.value) === saved.maxLogs,
      )
      if (validMaxLogs) setMaxLogs(saved.maxLogs)
    }

    if (!saved?.preset) {
      resetRange()
      return
    }

    if (saved.preset === 'all') {
      setPreset('all')
      setCustomStart('')
      setCustomEnd('')
      setRangeError('')
      return
    }

    const start = saved.customStart ?? ''
    const end = saved.customEnd ?? ''
    const validTime = (value: string) =>
      !value.trim() || !Number.isNaN(new Date(value).getTime())
    if (!validTime(start) || !validTime(end)) {
      resetRange()
      return
    }
    if (saved.preset !== 'custom' && (!start.trim() || !end.trim())) {
      resetRange()
      return
    }
    if (start && end && new Date(start).getTime() > new Date(end).getTime()) {
      resetRange()
      return
    }
    setPreset(saved.preset)
    setCustomStart(start)
    setCustomEnd(end)
    setRangeError('')
  }

  createEffect(() => {
    const projectId = props.projectId
    runSeq++
    setBusy(false)
    setError('')
    setResult(null)
    setProvider('')
    setMaxLogs(DEFAULT_MAX_LOGS)
    resetRange()
    pendingProvider = ''
    if (!projectId) return
    const saved = readSavedDetection(projectId)
    restoreSavedInputs(saved)
    pendingProvider = saved?.provider ?? ''
    void loadProviders()
  })

  const applyPreset = (value: PresetKey) => {
    setPreset(value)
    setRangeError('')
    if (value === 'all') {
      setCustomStart('')
      setCustomEnd('')
      return
    }
    const item = PRESET_ITEMS.find((entry) => entry.value === value)
    const now = Date.now()
    setCustomStart(toLocalInput(new Date(now - (item?.ms ?? 0))))
    setCustomEnd(toLocalInput(new Date(now)))
  }

  const updateStart = (value: string) => {
    setPreset('custom')
    setRangeError('')
    setCustomStart(value)
  }

  const updateEnd = (value: string) => {
    setPreset('custom')
    setRangeError('')
    setCustomEnd(value)
  }

  const resolveRange = (): ResolvedRange => {
    if (preset() === 'all') return {}
    const startRaw = customStart().trim()
    const endRaw = customEnd().trim()
    let start: string | undefined
    let end: string | undefined
    if (startRaw) {
      const parsed = new Date(startRaw)
      if (Number.isNaN(parsed.getTime())) return { error: '起始时间格式无效，请重新选择。' }
      start = parsed.toISOString()
    }
    if (endRaw) {
      const parsed = new Date(endRaw)
      if (Number.isNaN(parsed.getTime())) return { error: '结束时间格式无效，请重新选择。' }
      end = parsed.toISOString()
    }
    if (start && end && new Date(start).getTime() > new Date(end).getTime()) {
      return { error: '起始时间不能晚于结束时间。' }
    }
    return { start, end }
  }

  const rangeSummary = () => {
    if (preset() === 'all') return '全部时间范围（不限起止）'
    const range = resolveRange()
    if (range.error) return range.error
    return `${range.start ? formatTime(range.start) : '不限'} → ${
      range.end ? formatTime(range.end) : '不限'
    }`
  }

  const backendCap = () => {
    const value = Number(appConfig()?.ai?.max_logs_per_request ?? 0)
    return Number.isFinite(value) && value > 0 ? value : 0
  }

  const maxLogsHint = () => {
    const cap = backendCap()
    return cap > 0 ? `后端配置上限 ${cap}` : '后端可能按配置上限截断'
  }

  const maxLogsNote = () => {
    const cap = backendCap()
    return cap > 0
      ? `（后端配置上限 ${cap}，超出部分会被截断）`
      : '（后端可能按配置上限截断）'
  }

  const runAnalysis = async () => {
    if (busy()) return
    const projectId = props.projectId
    if (!projectId) return
    const runProvider = provider()
    if (!runProvider) {
      notify('warn', '请选择 AI 提供商', '请先在设置中配置并启用 AI 提供商。')
      return
    }
    const range = resolveRange()
    if (range.error) {
      setRangeError(range.error)
      notify('warn', '时间范围无效', range.error)
      return
    }

    const runPreset = preset()
    const runStart = customStart()
    const runEnd = customEnd()
    const runMaxLogs = maxLogs()

    setRangeError('')
    setError('')
    setBusy(true)
    const seq = ++runSeq
    try {
      const query: Query = {
        limit: runMaxLogs,
        sort_by: 'timestamp',
        sort_order: 'desc',
      }
      if (range.start) query.start_time = range.start
      if (range.end) query.end_time = range.end

      const response = await api.ai.detectAnomaly({
        project_id: projectId,
        provider: runProvider,
        query,
        max_logs: runMaxLogs,
      })
      if (seq !== runSeq) return
      setResult(response)
      writeSavedDetection(projectId, {
        result: response,
        provider: runProvider,
        preset: runPreset,
        customStart: runStart,
        customEnd: runEnd,
        maxLogs: runMaxLogs,
        savedAt: new Date().toISOString(),
      })
      const count = Array.isArray(response?.anomalies) ? response.anomalies.length : 0
      notify(
        'success',
        'AI 检测完成',
        `已分析 ${formatInt(response?.analyzed)} 条日志，发现 ${formatInt(count)} 个异常。`,
      )
    } catch (err) {
      if (seq !== runSeq) return
      const message = errorText(err)
      setError(message)
      notify('error', 'AI 检测失败', message)
    } finally {
      if (seq === runSeq) setBusy(false)
    }
  }

  const findings = () => result()?.findings ?? []
  const suggestions = () => result()?.suggestions ?? []
  const anomalies = () => result()?.anomalies ?? []

  return (
    <div class="stack">
      <div class="panel">
        <div class="panel-head">
          <div class="panel-title">
            <span class="tick" />
            AI 检测
            <Show when={configuredProviders().length > 0}>
              <Badge tone="muted">{configuredProviders().length} 个可用提供商</Badge>
            </Show>
          </div>
          <span class="mono-sm faint">按时间范围采样日志，由 AI 完成异常检测</span>
        </div>

        <div class="panel-body stack" style={{ gap: '14px' }}>
          <Show when={providersError()}>
            <div class="danger-note">
              <div class="row between wrap" style={{ gap: '8px' }}>
                <span>加载 AI 提供商失败：{providersError()}</span>
                <Button size="sm" variant="ghost" onClick={() => void loadProviders()}>
                  重试
                </Button>
              </div>
            </div>
          </Show>

          <Show
            when={
              !providersLoading() &&
              !providersError() &&
              configuredProviders().length === 0
            }
          >
            <div class="detail-note">
              <div
                class="row between wrap"
                style={{ gap: '8px', 'align-items': 'center' }}
              >
                <div class="row" style={{ gap: '7px', 'align-items': 'flex-start' }}>
                  <Icon name="alert" size={14} />
                  <span>
                    未检测到已配置的 AI 提供商。请前往“设置 → AI 提供商”填写
                    Endpoint、模型与密钥并启用后，再执行 AI 检测。
                  </span>
                </div>
                <Button size="sm" variant="ghost" onClick={() => navigate('settings')}>
                  <Icon name="settings" size={12} />
                  前往设置
                </Button>
              </div>
            </div>
          </Show>

          <div class="form-row">
            <Field label="AI 提供商" hint="仅显示已配置项，优先启用">
              <Select
                value={provider()}
                options={providerOptions()}
                disabled={providersLoading() || configuredProviders().length === 0}
                onInput={(value) => setProvider(value)}
              />
            </Field>
            <Field label="最大日志数" hint={maxLogsHint()}>
              <Select
                value={String(maxLogs())}
                options={MAX_LOG_OPTIONS}
                disabled={busy()}
                onInput={(value) => setMaxLogs(Number(value) || DEFAULT_MAX_LOGS)}
              />
            </Field>
          </div>

          <div>
            <div
              class="row between wrap"
              style={{ gap: '8px', 'align-items': 'center' }}
            >
              <div class="section-label" style={{ margin: 0 }}>
                时间范围
              </div>
              <span class="mono-sm faint">{rangeSummary()}</span>
            </div>

            <div class="row wrap" style={{ gap: '6px', 'margin-top': '9px' }}>
              <For each={PRESET_ITEMS}>
                {(item) => (
                  <button
                    type="button"
                    class={`chip ${preset() === item.value ? 'selected' : ''}`}
                    onClick={() => applyPreset(item.value)}
                  >
                    {item.label}
                  </button>
                )}
              </For>
              <Show when={preset() === 'custom'}>
                <span class="chip selected">自定义</span>
              </Show>
            </div>

            <div class="form-row" style={{ 'margin-top': '10px' }}>
              <Field label="起始时间" hint="datetime-local · 可留空">
                <TextInput
                  type="datetime-local"
                  value={customStart()}
                  disabled={busy()}
                  onInput={updateStart}
                />
              </Field>
              <Field label="结束时间" hint="datetime-local · 可留空">
                <TextInput
                  type="datetime-local"
                  value={customEnd()}
                  disabled={busy()}
                  onInput={updateEnd}
                />
              </Field>
            </div>

            <Show when={rangeError()}>
              <div class="mono-sm ember" style={{ 'margin-top': '4px' }}>
                {rangeError()}
              </div>
            </Show>
          </div>

          <div
            class="row between wrap"
            style={{ gap: '10px', 'align-items': 'center' }}
          >
            <span class="mono-sm faint">
              将按时间倒序读取最多 {maxLogs()} 条日志{maxLogsNote()}
            </span>
            <Button
              variant="primary"
              loading={busy()}
              disabled={!provider() || busy() || providersLoading()}
              onClick={() => void runAnalysis()}
            >
              <Icon name="wand" size={12} />
              {result() ? '重新检测' : '开始检测'}
            </Button>
          </div>
        </div>

        <Show when={busy()}>
          <div class="panel-body" style={{ 'padding-top': 0 }}>
            <PageBusy visible text="正在调用 AI 提供商检测日志，请稍候…" />
          </div>
        </Show>

        <Show when={error()}>
          <div class="panel-body" style={{ 'padding-top': 0 }}>
            <div class="danger-note">AI 检测失败：{error()}</div>
          </div>
        </Show>
      </div>

      <Show when={result()}>
        {(data) => (
          <div class="panel">
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                检测结果
                <Badge tone={riskToneOf(data().risk_level)}>
                  风险 {data().risk_level || 'unknown'}
                </Badge>
              </div>
              <span class="mono-sm faint">
                生成于 {formatTime(data().generated_at)}
              </span>
            </div>

            <div
              class="row wrap"
              style={{
                gap: '18px',
                padding: '10px 14px',
                'border-top': '1px solid var(--border)',
                'border-bottom': '1px solid var(--border)',
                background: 'var(--surface-soft)',
              }}
            >
              <span class="metric-inline">
                提供商 <b>{providerLabel(data().provider)}</b>
              </span>
              <span class="metric-inline">
                <b>{formatInt(data().analyzed)}</b> 条日志已分析
              </span>
              <span class="metric-inline">
                <b>{formatInt(anomalies().length)}</b> 个异常
              </span>
              <span class="metric-inline">
                关键发现 <b>{formatInt(findings().length)}</b>
              </span>
            </div>

            <div class="panel-body stack" style={{ gap: '16px' }}>
              <div>
                <div class="section-label">检测摘要</div>
                <div
                  style={{
                    'font-size': '12px',
                    'line-height': '1.75',
                    color: 'var(--text-muted)',
                    'white-space': 'pre-wrap',
                  }}
                >
                  {data().summary || '模型未返回检测摘要。'}
                </div>
              </div>

              <Show
                when={findings().length > 0}
                fallback={
                  <div class="faint" style={{ 'font-size': '11px' }}>
                    模型未返回关键发现。
                  </div>
                }
              >
                <div>
                  <div class="section-label">关键发现</div>
                  <ul
                    style={{
                      margin: 0,
                      padding: '0 0 0 16px',
                      display: 'grid',
                      gap: '5px',
                    }}
                  >
                    <For each={findings()}>
                      {(item) => (
                        <li
                          style={{
                            'font-size': '11.5px',
                            color: 'var(--text-muted)',
                            'line-height': '1.6',
                          }}
                        >
                          {item}
                        </li>
                      )}
                    </For>
                  </ul>
                </div>
              </Show>

              <Show
                when={suggestions().length > 0}
                fallback={
                  <div class="faint" style={{ 'font-size': '11px' }}>
                    模型未返回处置建议。
                  </div>
                }
              >
                <div>
                  <div class="section-label">处置建议</div>
                  <ul
                    style={{
                      margin: 0,
                      padding: '0 0 0 16px',
                      display: 'grid',
                      gap: '5px',
                    }}
                  >
                    <For each={suggestions()}>
                      {(item) => (
                        <li
                          style={{
                            'font-size': '11.5px',
                            color: 'var(--text-muted)',
                            'line-height': '1.6',
                          }}
                        >
                          {item}
                        </li>
                      )}
                    </For>
                  </ul>
                </div>
              </Show>

              <div>
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <div class="section-label" style={{ margin: 0 }}>
                    异常条目
                  </div>
                  <span class="mono-sm faint">点击 DocID 查看对应日志详情</span>
                </div>

                <Show
                  when={anomalies().length > 0}
                  fallback={
                    <div
                      class="faint"
                      style={{ 'font-size': '11px', padding: '10px 0' }}
                    >
                      本次检测未返回异常条目
                      {data().analyzed
                        ? `（已分析 ${formatInt(data().analyzed)} 条日志）。`
                        : '。'}
                    </div>
                  }
                >
                  <div
                    class="scrollable"
                    style={{ 'max-height': '480px', 'overflow-x': 'auto', 'margin-top': '8px' }}
                  >
                    <table class="data-table" style={{ 'min-width': '940px' }}>
                      <thead>
                        <tr>
                          <th style={{ width: '150px' }}>时间</th>
                          <th style={{ width: '86px' }}>级别</th>
                          <th>消息</th>
                          <th>原因</th>
                          <th style={{ width: '80px' }}>评分</th>
                          <th style={{ width: '150px' }}>DocID</th>
                        </tr>
                      </thead>
                      <tbody>
                        <For each={anomalies()}>
                          {(item: Anomaly) => (
                            <tr>
                              <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                                {formatTime(item.timestamp)}
                              </td>
                              <td>
                                <LevelBadge level={item.level} />
                              </td>
                              <td title={item.message}>
                                {item.message
                                  ? truncate(item.message, 160)
                                  : '—'}
                              </td>
                              <td class="muted" title={item.reason}>
                                {item.reason
                                  ? truncate(item.reason, 140)
                                  : '—'}
                              </td>
                              <td>
                                <Badge tone={riskTone(item.score)}>
                                  {scoreText(item.score)}
                                </Badge>
                              </td>
                              <td>
                                <Show
                                  when={(item.doc_ids ?? []).length > 0}
                                  fallback={<span class="mono-sm faint">—</span>}
                                >
                                  <div class="row wrap" style={{ gap: '4px' }}>
                                    <For each={item.doc_ids ?? []}>
                                      {(docId) => (
                                        <button
                                          type="button"
                                          class="chip mono-sm"
                                          title={`打开日志 #${docId}`}
                                          onClick={() => props.onOpenLog(docId)}
                                        >
                                          #{docId}
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
            </div>
          </div>
        )}
      </Show>
    </div>
  )
}
