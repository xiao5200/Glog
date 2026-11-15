import { For, JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type {
  AIProviderInfo,
  AnomalyDetectionResult,
  LogSource,
  Query,
  ReportResult,
  RootCauseResult,
} from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import {
  Badge,
  Button,
  CodeBlock,
  EmptyState,
  Field,
  Icon,
  LevelBadge,
  Modal,
  PageBusy,
  Select,
  TextArea,
  TextInput,
} from '../ui'
import {
  copyText,
  formatInt,
  formatTime,
  severityTone,
  todayStamp,
  truncate,
} from '../../utils'
import {
  basename,
  localInputToIso,
  normalizedFraction,
  parseEventIDs,
  parseMaxLogs,
  resolveTimeBounds,
  toLocalInput,
  toggleValue,
} from './logCommon'

/* ------------------------------------------------------------------ */
/* 公共组件与工具                                                       */
/* ------------------------------------------------------------------ */

function createProviderState(open: () => boolean) {
  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [provider, setProvider] = createSignal('')

  let requestSeq = 0

  async function reload() {
    const seq = ++requestSeq
    setLoading(true)
    setError(null)
    try {
      const list = await api.ai.providers()
      if (seq !== requestSeq) return
      const items = Array.isArray(list) ? list : []
      setProviders(items)
      const usable = items.filter((item) => item.configured)
      if (usable.length === 0) {
        setProvider('')
        setError('尚未配置可用的 AI 服务。请前往 设置 → AI 服务 填写 Provider 配置与 API Key。')
        return
      }
      const current = provider()
      if (!usable.some((item) => item.provider === current)) {
        const preferred = usable.find((item) => item.enabled) ?? usable[0]
        setProvider(preferred.provider)
      }
    } catch (err) {
      if (seq !== requestSeq) return
      const text = errorText(err)
      setProviders([])
      setProvider('')
      setError(text)
      notify('error', 'AI 服务读取失败', text, 6000)
    } finally {
      if (seq === requestSeq) setLoading(false)
    }
  }

  createEffect(
    on(open, (isOpen) => {
      if (isOpen) void reload()
    }),
  )

  return { providers, loading, error, provider, setProvider, reload }
}

function AIProviderField(props: {
  providers: AIProviderInfo[]
  loading: boolean
  error: string | null
  value: string
  disabled?: boolean
  onSelect: (provider: string) => void
  onReload: () => void
  onOpenSettings: () => void
}): JSX.Element {
  const configured = () => props.providers.filter((item) => item.configured)
  const selected = () => configured().find((item) => item.provider === props.value)

  return (
    <div>
      <Field
        label="AI Provider"
        hint={props.loading ? '加载中…' : selected()?.model || undefined}
      >
        <div class="row" style={{ gap: '6px' }}>
          <select
            class="select"
            style={{ flex: 1, 'min-width': 0 }}
            value={props.value}
            disabled={props.disabled || props.loading || configured().length === 0}
            aria-label="选择 AI Provider"
            onChange={(event) => props.onSelect(event.currentTarget.value)}
          >
            <option value="">
              {props.loading
                ? '加载 AI 服务…'
                : configured().length === 0
                  ? '无可用 Provider'
                  : '选择 Provider…'}
            </option>
            <For each={configured()}>
              {(item) => (
                <option value={item.provider}>
                  {item.name || item.provider}
                  {item.model ? ` · ${item.model}` : ''}
                  {item.enabled ? '' : '（已停用）'}
                </option>
              )}
            </For>
          </select>
          <Button
            size="sm"
            variant="ghost"
            loading={props.loading}
            disabled={props.disabled}
            onClick={props.onReload}
          >
            刷新
          </Button>
        </div>
      </Field>

      <Show when={props.error}>
        <div class="detail-note" style={{ 'margin-top': '6px' }}>
          <div class="row between" style={{ gap: '8px' }}>
            <span class="grow" style={{ 'line-height': '1.6' }}>
              {props.error}
            </span>
            <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
              前往设置
            </Button>
          </div>
        </div>
      </Show>
    </div>
  )
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
        <For each={props.items}>
          {(item) => <li>{item}</li>}
        </For>
      </ul>
    </Show>
  )
}

function QueryTags(props: { query: Query }): JSX.Element {
  const parts = () => {
    const query = props.query
    const list: string[] = []
    if (query.text?.trim()) list.push(`文本：${truncate(query.text.trim(), 70)}`)
    if (query.levels?.length) list.push(`级别：${query.levels.join(' / ')}`)
    if (query.hosts?.length) list.push(`主机：${query.hosts.length} 个`)
    if (query.sources?.length) list.push(`日志源：${query.sources.length} 个`)
    if (query.event_ids?.length) list.push(`事件 ID：${query.event_ids.join(', ')}`)
    if (query.field_filters?.length) list.push(`字段条件：${query.field_filters.length} 个`)
    if (query.start_time || query.end_time) {
      list.push(`时间：${formatTime(query.start_time, false)} ~ ${formatTime(query.end_time, false)}`)
    }
    return list
  }

  return (
    <Show
      when={parts().length > 0}
      fallback={<span class="mono-sm faint">查询范围：全部日志</span>}
    >
      <div class="row wrap" style={{ gap: '4px' }}>
        <For each={parts()}>
          {(part) => <span class="tag">{part}</span>}
        </For>
      </div>
    </Show>
  )
}

function confidenceText(value: number | undefined | null): string {
  return `${Math.round(normalizedFraction(value) * 100)}%`
}

function anomalyScore(value: number | undefined | null): string {
  const n = Number(value ?? 0)
  return Number.isFinite(n) ? n.toFixed(2) : '—'
}

/* ------------------------------------------------------------------ */
/* 异常检测                                                             */
/* ------------------------------------------------------------------ */

export interface AnomalyModalProps {
  open: boolean
  projectID: string
  query: Query
  onClose: () => void
  onOpenSettings: () => void
  onLocate: (docID: number) => void
}

export function AnomalyModal(props: AnomalyModalProps): JSX.Element {
  const providers = createProviderState(() => props.open)
  const [maxLogs, setMaxLogs] = createSignal('100')
  const [running, setRunning] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [result, setResult] = createSignal<AnomalyDetectionResult | null>(null)

  let runSeq = 0

  createEffect(
    on(
      () => props.open,
      (open) => {
        runSeq++
        setRunning(false)
        if (!open) return
        setMaxLogs('100')
        setError(null)
        setResult(null)
      },
    ),
  )

  async function run() {
    const provider = providers.provider()
    if (!props.projectID) {
      notify('warn', '未选择项目', '请先在项目页打开一个项目。')
      return
    }
    if (!provider) {
      notify('warn', 'AI 服务未配置', '请先在设置中配置并选择 AI Provider。')
      return
    }
    const seq = ++runSeq
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const data = await api.ai.detectAnomaly({
        project_id: props.projectID,
        provider,
        query: props.query,
        max_logs: parseMaxLogs(maxLogs(), 100),
      })
      if (seq !== runSeq) return
      setResult(data)
      notify('success', '异常检测完成', `已分析 ${formatInt(data?.analyzed ?? 0)} 条日志。`)
    } catch (err) {
      if (seq !== runSeq) return
      const text = errorText(err)
      setError(text)
      notify('error', '异常检测失败', text, 7000)
    } finally {
      if (seq === runSeq) setRunning(false)
    }
  }

  return (
    <Modal
      open={props.open}
      title="AI 异常检测"
      subtitle="分析当前查询命中的日志，识别异常模式并给出处置建议"
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
            AI 设置
          </Button>
          <span class="grow" />
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Button
            size="sm"
            variant="primary"
            loading={running()}
            disabled={!providers.provider() || providers.loading()}
            onClick={() => void run()}
          >
            开始检测
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '12px' }}>
        <QueryTags query={props.query} />

        <div class="form-row">
          <AIProviderField
            providers={providers.providers()}
            loading={providers.loading()}
            error={providers.error()}
            value={providers.provider()}
            disabled={running()}
            onSelect={providers.setProvider}
            onReload={() => void providers.reload()}
            onOpenSettings={props.onOpenSettings}
          />
          <Field label="最大分析日志数" hint="1 – 1000">
            <TextInput
              type="number"
              value={maxLogs()}
              disabled={running()}
              onInput={setMaxLogs}
            />
          </Field>
        </div>

        <Show when={error()}>
          <div class="danger-note">
            <div class="row between" style={{ gap: '8px' }}>
              <span class="grow">{error()}</span>
              <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
                检查 AI 设置
              </Button>
            </div>
          </div>
        </Show>

        <PageBusy visible={running()} text="AI 正在分析日志，请稍候…" />

        <Show when={!running() && !result()}>
          <div class="detail-note">
            选择可用的 AI Provider 与最大日志数后开始检测。分析结果中的异常条目可通过 doc ID 直接定位到日志详情。
          </div>
        </Show>

        <Show when={result()}>
          {(resultAccessor) => {
            const data = resultAccessor()
            return (
              <div class="stack" style={{ gap: '12px' }}>
                <div class="row wrap" style={{ gap: '6px' }}>
                  <Badge tone={severityTone(data.risk_level)}>
                    风险 {data.risk_level || '未知'}
                  </Badge>
                  <Badge tone="muted">{data.provider || 'AI'}</Badge>
                  <span class="mono-sm faint">
                    已分析 {formatInt(data.analyzed)} 条 · {formatTime(data.generated_at)}
                  </span>
                </div>

                <div class="success-note" style={{ 'line-height': '1.7' }}>
                  {data.summary || 'AI 未返回摘要。'}
                </div>

                <div class="form-row">
                  <div>
                    <div class="section-label">发现</div>
                    <BulletList items={data.findings ?? []} empty="未返回发现项。" />
                  </div>
                  <div>
                    <div class="section-label">建议</div>
                    <BulletList items={data.suggestions ?? []} empty="未返回处置建议。" />
                  </div>
                </div>

                <div>
                  <div class="section-label">
                    异常条目（{data.anomalies?.length ?? 0}）
                  </div>
                  <Show
                    when={(data.anomalies?.length ?? 0) > 0}
                    fallback={<div class="mono-sm faint">未返回具体异常条目。</div>}
                  >
                    <div
                      class="scrollable scroll-thin"
                      style={{
                        'max-height': '320px',
                        overflow: 'auto',
                        display: 'grid',
                        gap: '7px',
                      }}
                    >
                      <For each={data.anomalies ?? []}>
                        {(anomaly) => (
                          <div
                            class="panel"
                            style={{ padding: '8px 10px', 'box-shadow': 'none' }}
                          >
                            <div class="row between wrap" style={{ gap: '6px' }}>
                              <div class="row" style={{ gap: '6px' }}>
                                <LevelBadge level={anomaly.level} />
                                <span class="mono-sm faint">
                                  {formatTime(anomaly.timestamp)}
                                </span>
                              </div>
                              <Badge tone="ember">
                                评分 {anomalyScore(anomaly.score)}
                              </Badge>
                            </div>
                            <div
                              style={{
                                'font-size': '11px',
                                color: 'var(--text)',
                                'line-height': '1.6',
                                'margin-top': '5px',
                                'word-break': 'break-word',
                              }}
                            >
                              {anomaly.message || '(空消息)'}
                            </div>
                            <div class="detail-note" style={{ 'margin-top': '5px' }}>
                              原因：{anomaly.reason || '未说明'}
                            </div>
                            <Show when={(anomaly.doc_ids?.length ?? 0) > 0}>
                              <div
                                class="row wrap"
                                style={{ gap: '4px', 'margin-top': '6px' }}
                              >
                                <span class="mono-sm faint" style={{ 'margin-right': '2px' }}>
                                  定位
                                </span>
                                <For each={anomaly.doc_ids ?? []}>
                                  {(docID) => (
                                    <button
                                      type="button"
                                      class="chip"
                                      title={`打开日志 #${docID} 的详情`}
                                      onClick={() => props.onLocate(docID)}
                                    >
                                      #{docID}
                                    </button>
                                  )}
                                </For>
                              </div>
                            </Show>
                          </div>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>
              </div>
            )
          }}
        </Show>
      </div>
    </Modal>
  )
}

/* ------------------------------------------------------------------ */
/* 自然语言转查询                                                       */
/* ------------------------------------------------------------------ */

const NL_EXAMPLES = [
  '最近一小时有哪些登录失败事件？',
  '查找来源主机 web-01 的 ERROR 日志',
  '筛选 event_id 4625 且包含 admin 的日志',
  '列出最近 24 小时风险最高的日志',
]

export interface NLQueryModalProps {
  open: boolean
  projectID: string
  onClose: () => void
  onOpenSettings: () => void
  onApply: (query: Query) => void
}

export function NLQueryModal(props: NLQueryModalProps): JSX.Element {
  const [question, setQuestion] = createSignal('')
  const [generated, setGenerated] = createSignal<Query | null>(null)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)

  let requestSeq = 0

  createEffect(
    on(
      () => props.open,
      (open) => {
        requestSeq++
        setLoading(false)
        if (!open) return
        setQuestion('')
        setGenerated(null)
        setError(null)
      },
    ),
  )

  async function generate() {
    const text = question().trim()
    if (!text) {
      notify('warn', '请输入问题', '用自然语言描述你希望检索的日志。')
      return
    }
    if (!props.projectID) {
      notify('warn', '未选择项目', '请先在项目页打开一个项目。')
      return
    }
    const seq = ++requestSeq
    setLoading(true)
    setError(null)
    setGenerated(null)
    try {
      const query = await api.ai.nlQuery(props.projectID, text)
      if (seq !== requestSeq) return
      setGenerated(query)
      notify('success', '查询已生成', '请检查生成的查询条件，再应用到搜索。')
    } catch (err) {
      if (seq !== requestSeq) return
      const message = errorText(err)
      setError(message)
      notify('error', '自然语言查询失败', message, 7000)
    } finally {
      if (seq === requestSeq) setLoading(false)
    }
  }

  function apply() {
    const query = generated()
    if (query) props.onApply(query)
  }

  return (
    <Modal
      open={props.open}
      title="自然语言查询"
      subtitle="将中文问题转换为结构化日志查询"
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
            AI 设置
          </Button>
          <span class="grow" />
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Button
            size="sm"
            variant="default"
            loading={loading()}
            onClick={() => void generate()}
          >
            生成查询
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!generated() || loading()}
            onClick={apply}
          >
            应用到搜索
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '12px' }}>
        <Field label="用自然语言描述查询" hint="AI 会生成等价的结构化 Query">
          <TextArea
            value={question()}
            rows={4}
            disabled={loading()}
            placeholder="例如：查找最近 24 小时内来自 web-01 的 ERROR 或 CRITICAL 日志，并按时间倒序显示。"
            onInput={setQuestion}
          />
        </Field>

        <div class="row wrap" style={{ gap: '4px' }}>
          <span class="mono-sm faint" style={{ 'margin-right': '2px' }}>
            示例
          </span>
          <For each={NL_EXAMPLES}>
            {(example) => (
              <button
                type="button"
                class="chip"
                title={example}
                disabled={loading()}
                onClick={() => setQuestion(example)}
              >
                {truncate(example, 26)}
              </button>
            )}
          </For>
        </div>

        <Show when={error()}>
          <div class="danger-note">
            <div class="row between" style={{ gap: '8px' }}>
              <span class="grow">{error()}</span>
              <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
                检查 AI 设置
              </Button>
            </div>
          </div>
        </Show>

        <PageBusy visible={loading()} text="正在进行自然语言转换…" />

        <Show
          when={generated()}
          fallback={
            <Show when={!loading()}>
              <div class="detail-note">
                生成后可先检查文本、时间、级别与字段条件，再应用到当前搜索。执行时会替换现有查询条件。
              </div>
            </Show>
          }
        >
          {(queryAccessor) => {
            const query = queryAccessor()
            return (
              <div class="stack" style={{ gap: '8px' }}>
                <div class="section-label">生成的查询</div>
                <QueryTags query={query} />
                <CodeBlock text={JSON.stringify(query, null, 2)} maxHeight="280px" />
                <div class="success-note">
                  点击“应用到搜索”会同步更新工具栏筛选条件并立即执行查询。
                </div>
              </div>
            )
          }}
        </Show>
      </div>
    </Modal>
  )
}

/* ------------------------------------------------------------------ */
/* 根因分析                                                             */
/* ------------------------------------------------------------------ */

export interface RootCauseModalProps {
  open: boolean
  projectID: string
  query: Query
  onClose: () => void
  onOpenSettings: () => void
}

export function RootCauseModal(props: RootCauseModalProps): JSX.Element {
  const providers = createProviderState(() => props.open)
  const [question, setQuestion] = createSignal('')
  const [maxLogs, setMaxLogs] = createSignal('200')
  const [running, setRunning] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [result, setResult] = createSignal<RootCauseResult | null>(null)

  let runSeq = 0

  createEffect(
    on(
      () => props.open,
      (open) => {
        runSeq++
        setRunning(false)
        if (!open) return
        setQuestion('')
        setMaxLogs('200')
        setError(null)
        setResult(null)
      },
    ),
  )

  async function run() {
    const provider = providers.provider()
    if (!props.projectID) {
      notify('warn', '未选择项目', '请先在项目页打开一个项目。')
      return
    }
    if (!provider) {
      notify('warn', 'AI 服务未配置', '请先在设置中配置并选择 AI Provider。')
      return
    }
    const seq = ++runSeq
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const data = await api.ai.rootCause({
        project_id: props.projectID,
        provider,
        query: props.query,
        question:
          question().trim() || '请分析当前查询日志中的异常根因、时间线与处置建议。',
        max_logs: parseMaxLogs(maxLogs(), 200),
      })
      if (seq !== runSeq) return
      setResult(data)
      notify('success', '根因分析完成', `已分析 ${formatInt(data?.analyzed ?? 0)} 条日志。`)
    } catch (err) {
      if (seq !== runSeq) return
      const text = errorText(err)
      setError(text)
      notify('error', '根因分析失败', text, 7000)
    } finally {
      if (seq === runSeq) setRunning(false)
    }
  }

  return (
    <Modal
      open={props.open}
      title="AI 根因分析"
      subtitle="结合当前查询日志，推断根因、梳理时间线并生成处置建议"
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
            AI 设置
          </Button>
          <span class="grow" />
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Button
            size="sm"
            variant="primary"
            loading={running()}
            disabled={!providers.provider() || providers.loading()}
            onClick={() => void run()}
          >
            开始分析
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '12px' }}>
        <QueryTags query={props.query} />

        <div class="form-row">
          <AIProviderField
            providers={providers.providers()}
            loading={providers.loading()}
            error={providers.error()}
            value={providers.provider()}
            disabled={running()}
            onSelect={providers.setProvider}
            onReload={() => void providers.reload()}
            onOpenSettings={props.onOpenSettings}
          />
          <Field label="最大分析日志数" hint="1 – 1000">
            <TextInput
              type="number"
              value={maxLogs()}
              disabled={running()}
              onInput={setMaxLogs}
            />
          </Field>
        </div>

        <Field label="分析问题" hint="可选，默认分析异常根因">
          <TextArea
            value={question()}
            rows={3}
            disabled={running()}
            placeholder="例如：为什么 web-01 在 14:00 后出现大量 502？可能与哪些变更有关？"
            onInput={setQuestion}
          />
        </Field>

        <Show when={error()}>
          <div class="danger-note">
            <div class="row between" style={{ gap: '8px' }}>
              <span class="grow">{error()}</span>
              <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
                检查 AI 设置
              </Button>
            </div>
          </div>
        </Show>

        <PageBusy visible={running()} text="AI 正在梳理根因与时间线…" />

        <Show when={!running() && !result()}>
          <div class="detail-note">
            根因分析将读取当前查询命中的日志，输出可疑根因、置信度、证据、时间线与处置建议。
          </div>
        </Show>

        <Show when={result()}>
          {(resultAccessor) => {
            const data = resultAccessor()
            return (
              <div class="stack" style={{ gap: '12px' }}>
                <div class="row wrap" style={{ gap: '6px' }}>
                  <Badge tone="mint">根因分析完成</Badge>
                  <Badge tone="muted">{data.provider || 'AI'}</Badge>
                  <span class="mono-sm faint">
                    已分析 {formatInt(data.analyzed)} 条 · {formatTime(data.generated_at)}
                  </span>
                </div>

                <div class="success-note" style={{ 'line-height': '1.7' }}>
                  {data.summary || 'AI 未返回摘要。'}
                </div>

                <div>
                  <div class="section-label">
                    可能的根因（{data.root_causes?.length ?? 0}）
                  </div>
                  <Show
                    when={(data.root_causes?.length ?? 0) > 0}
                    fallback={<div class="mono-sm faint">未返回根因条目。</div>}
                  >
                    <div class="stack" style={{ gap: '7px' }}>
                      <For each={data.root_causes ?? []}>
                        {(cause) => (
                          <div
                            class="panel"
                            style={{ padding: '9px 10px', 'box-shadow': 'none' }}
                          >
                            <div class="row between wrap" style={{ gap: '6px' }}>
                              <span
                                class="grow"
                                style={{
                                  'font-size': '11.5px',
                                  'font-weight': 500,
                                  color: 'var(--text)',
                                  'min-width': '180px',
                                }}
                              >
                                {cause.cause}
                              </span>
                              <Badge tone="mint">
                                置信度 {confidenceText(cause.confidence)}
                              </Badge>
                            </div>
                            <div
                              style={{
                                'font-size': '11px',
                                'line-height': '1.65',
                                color: 'var(--text-muted)',
                                'margin-top': '4px',
                                'word-break': 'break-word',
                              }}
                            >
                              {cause.description || '无描述'}
                            </div>
                            <Show when={(cause.evidence?.length ?? 0) > 0}>
                              <div style={{ 'margin-top': '6px' }}>
                                <div class="mono-sm faint" style={{ 'margin-bottom': '2px' }}>
                                  证据
                                </div>
                                <BulletList items={cause.evidence ?? []} empty="无证据" />
                              </div>
                            </Show>
                          </div>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>

                <div>
                  <div class="section-label">时间线</div>
                  <Show
                    when={(data.timeline?.length ?? 0) > 0}
                    fallback={<div class="mono-sm faint">未返回时间线。</div>}
                  >
                    <div class="stack" style={{ gap: '4px' }}>
                      <For each={data.timeline ?? []}>
                        {(line) => (
                          <div class="row" style={{ gap: '7px', 'align-items': 'flex-start' }}>
                            <span
                              style={{
                                color: 'var(--accent-dim)',
                                'font-family': 'var(--font-mono)',
                                'font-size': '10px',
                                'line-height': '1.6',
                              }}
                            >
                              ›
                            </span>
                            <span
                              class="mono-sm muted"
                              style={{ 'line-height': '1.65', 'word-break': 'break-word' }}
                            >
                              {line}
                            </span>
                          </div>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>

                <div>
                  <div class="section-label">处置建议</div>
                  <BulletList items={data.suggestions ?? []} empty="未返回处置建议。" />
                </div>
              </div>
            )
          }}
        </Show>
      </div>
    </Modal>
  )
}

/* ------------------------------------------------------------------ */
/* 分析报告                                                             */
/* ------------------------------------------------------------------ */

type ReportTimePreset = 'current' | '15m' | '1h' | '24h' | '7d' | 'all' | 'custom'

const REPORT_TIME_PRESETS: ReadonlyArray<{
  value: ReportTimePreset
  label: string
  title: string
}> = [
  { value: 'current', label: '沿用当前查询', title: '使用日志页当前查询的时间范围' },
  { value: '15m', label: '15m', title: '最近 15 分钟' },
  { value: '1h', label: '1h', title: '最近 1 小时' },
  { value: '24h', label: '24h', title: '最近 24 小时' },
  { value: '7d', label: '7d', title: '最近 7 天' },
  { value: 'all', label: '全部', title: '不限制时间范围' },
  { value: 'custom', label: '自定义', title: '自定义本地时间范围' },
]

const REPORT_DIMENSIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'event_type', label: '事件类型' },
  { value: 'source_ip', label: '来源 IP' },
  { value: 'destination_ip', label: '目的 IP' },
  { value: 'event_id', label: '事件 ID' },
  { value: 'source_host', label: '主机' },
  { value: 'level', label: '级别' },
]

const DEFAULT_REPORT_DIMENSIONS = ['event_type', 'source_ip', 'destination_ip', 'event_id']

const REPORT_AGGREGATE_ITEM_LIMIT = 12

const reportRangeInputStyle: JSX.CSSProperties = {
  width: '190px',
  'min-height': '28px',
  padding: '3px 8px',
  'font-size': '10.5px',
  'font-family': 'var(--font-mono)',
}

function defaultReportCustomRange(): { start: string; end: string } {
  const now = new Date()
  return {
    start: toLocalInput(new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString()),
    end: toLocalInput(now.toISOString()),
  }
}

function reportSourceLabel(source: LogSource): string {
  const type = String(source.type ?? '').trim() || '日志源'
  const detail =
    basename(source.path) || source.host || source.template_id || source.id
  return detail ? `${type} · ${detail}` : type
}

/** 拆分 IP 输入：支持逗号、空格、分号、顿号分隔，并去重。 */
function splitScopeTokens(text: string): string[] {
  const seen = new Set<string>()
  const tokens: string[] = []
  for (const raw of text.split(/[\s,;，、；]+/)) {
    const token = raw.trim()
    if (!token || seen.has(token)) continue
    seen.add(token)
    tokens.push(token)
  }
  return tokens
}

/** 将字段名归一化为 source_ip / destination_ip，便于覆盖当前查询中的同类条件。 */
function canonicalScopeField(field: string): string {
  const normalized = String(field ?? '').trim().toLowerCase()
  if (/^(src|source)[\s._-]*ip$/.test(normalized)) return 'source_ip'
  if (/^(dst|dest|destination)[\s._-]*ip$/.test(normalized)) return 'destination_ip'
  return normalized.replace(/[\s.-]+/g, '_')
}

function reportDimensionLabel(field: string): string {
  return REPORT_DIMENSIONS.find((item) => item.value === field)?.label ?? field
}

function aggregatePercent(count: number, total: number): string | null {
  const n = Number(count)
  const t = Number(total)
  if (!Number.isFinite(n) || !Number.isFinite(t) || t <= 0) return null
  return `${((n / t) * 100).toFixed(1)}%`
}

export interface ReportModalProps {
  open: boolean
  projectID: string
  query: Query
  onClose: () => void
  onOpenSettings: () => void
}

export function ReportModal(props: ReportModalProps): JSX.Element {
  const providers = createProviderState(() => props.open)
  const initialCustomRange = defaultReportCustomRange()
  const [title, setTitle] = createSignal('')
  const [question, setQuestion] = createSignal('')
  const [maxLogs, setMaxLogs] = createSignal('200')
  const [outputPath, setOutputPath] = createSignal('')
  const [choosingPath, setChoosingPath] = createSignal(false)
  const [generating, setGenerating] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [result, setResult] = createSignal<ReportResult | null>(null)

  // 报告范围：时间 / 日志类型 / IP / 事件 ID / 聚合维度。
  const [timePreset, setTimePreset] = createSignal<ReportTimePreset>('current')
  const [customStart, setCustomStart] = createSignal(initialCustomRange.start)
  const [customEnd, setCustomEnd] = createSignal(initialCustomRange.end)
  const [sources, setSources] = createSignal<LogSource[]>([])
  const [sourcesLoading, setSourcesLoading] = createSignal(false)
  const [sourcesError, setSourcesError] = createSignal<string | null>(null)
  const [sourceID, setSourceID] = createSignal('')
  const [sourceIPText, setSourceIPText] = createSignal('')
  const [destIPText, setDestIPText] = createSignal('')
  const [eventIDText, setEventIDText] = createSignal('')
  const [dimensions, setDimensions] = createSignal<string[]>([
    ...DEFAULT_REPORT_DIMENSIONS,
  ])

  let requestSeq = 0
  let sourceSeq = 0

  async function loadSources() {
    const projectID = props.projectID
    const seq = ++sourceSeq
    setSourcesLoading(true)
    setSourcesError(null)
    if (!projectID) {
      setSources([])
      setSourcesLoading(false)
      return
    }
    try {
      const list = await api.projects.sources(projectID)
      if (seq !== sourceSeq) return
      const items = Array.isArray(list) ? list : []
      setSources(items)
      const current = sourceID()
      if (current && !items.some((source) => source.id === current)) {
        setSourceID('')
      }
    } catch (err) {
      if (seq !== sourceSeq) return
      // 日志源加载失败不阻塞报告生成：保留“沿用当前查询 / 全部”选项。
      setSources([])
      setSourceID('')
      setSourcesError(errorText(err))
    } finally {
      if (seq === sourceSeq) setSourcesLoading(false)
    }
  }

  createEffect(
    on(
      () => props.open,
      (open) => {
        requestSeq++
        sourceSeq++
        setGenerating(false)
        setSourcesLoading(false)
        if (!open) return
        const range = defaultReportCustomRange()
        setTitle(`日志分析报告 · ${todayStamp()}`)
        setQuestion('')
        setMaxLogs('200')
        setOutputPath('')
        setError(null)
        setResult(null)
        setTimePreset('current')
        setCustomStart(range.start)
        setCustomEnd(range.end)
        setSources([])
        setSourcesError(null)
        setSourceID('')
        setSourceIPText('')
        setDestIPText('')
        setEventIDText('')
        setDimensions([...DEFAULT_REPORT_DIMENSIONS])
        if (props.projectID) void loadSources()
      },
    ),
  )

  createEffect(
    on(
      () => props.projectID,
      () => {
        sourceSeq++
        setSources([])
        setSourcesError(null)
        setSourcesLoading(false)
        setSourceID('')
        if (props.open && props.projectID) void loadSources()
      },
      { defer: true },
    ),
  )

  async function chooseOutputPath() {
    setChoosingPath(true)
    try {
      const selected = await api.system.selectSaveFile(
        `glog-report-${todayStamp()}.md`,
        'Markdown 分析报告',
        '*.md',
      )
      if (selected) setOutputPath(selected)
    } catch (err) {
      notify('error', '选择保存路径失败', errorText(err), 6000)
    } finally {
      setChoosingPath(false)
    }
  }

  async function generate() {
    const provider = providers.provider()
    const titleValue = title().trim()
    if (!props.projectID) {
      notify('warn', '未选择项目', '请先在项目页打开一个项目。')
      return
    }
    if (!titleValue) {
      notify('warn', '请输入报告标题', '报告标题不能为空。')
      return
    }
    if (!provider) {
      notify('warn', 'AI 服务未配置', '请先在设置中配置并选择 AI Provider。')
      return
    }
    const invalidScope = scopeValidation()
    if (invalidScope) {
      notify('warn', '报告范围无效', invalidScope, 6000)
      return
    }
    const seq = ++requestSeq
    setGenerating(true)
    setError(null)
    setResult(null)
    try {
      const focus = question().trim()
      const composedTitle = focus ? `${titleValue}（分析关注：${focus}）` : titleValue
      const path = outputPath().trim()
      const data = await api.ai.report({
        project_id: props.projectID,
        provider,
        title: composedTitle,
        query: scopedQuery(),
        aggregate_by: [...dimensions()],
        max_logs: parseMaxLogs(maxLogs(), 200),
        output_path: path || undefined,
      })
      if (seq !== requestSeq) return
      setResult(data)
      if (data.output_path) setOutputPath(data.output_path)
      notify(
        'success',
        '报告已生成',
        data.output_path ? `已写入 ${data.output_path}` : '可在弹窗中查看并复制 Markdown。',
        6000,
      )
    } catch (err) {
      if (seq !== requestSeq) return
      const text = errorText(err)
      setError(text)
      notify('error', '报告生成失败', text, 7000)
    } finally {
      if (seq === requestSeq) setGenerating(false)
    }
  }

  async function copyReport(content: string) {
    try {
      await copyText(content)
      notify('success', '报告已复制', 'Markdown 内容已写入剪贴板。')
    } catch (err) {
      notify('error', '复制失败', errorText(err))
    }
  }

  async function showInExplorer(path: string) {
    try {
      await api.system.showInExplorer(path)
    } catch (err) {
      notify('error', '打开文件位置失败', errorText(err), 6000)
    }
  }

  const sourceOptions = () => [
    { value: '', label: '沿用当前查询 / 全部' },
    ...sources().map((source) => ({ value: source.id, label: reportSourceLabel(source) })),
  ]

  /**
   * 基于当前日志页查询构建报告范围。始终浅拷贝 props.query（含数组与字段条件），
   * 并根据时间 / 日志类型 / IP / 事件 ID 覆盖对应条件，不修改页面查询对象。
   */
  function scopedQuery(): Query {
    const base = props.query ?? {}
    const query: Query = { ...base }
    if (base.levels) query.levels = [...base.levels]
    if (base.hosts) query.hosts = [...base.hosts]
    if (base.sources) query.sources = [...base.sources]
    if (base.event_ids) query.event_ids = [...base.event_ids]
    if (base.field_filters) {
      query.field_filters = base.field_filters.map((filter) => ({ ...filter }))
    }
    delete query.offset
    delete query.limit

    const preset = timePreset()
    if (preset !== 'current') {
      const bounds = resolveTimeBounds(preset, customStart(), customEnd(), Date.now())
      if (bounds.start) query.start_time = bounds.start
      else delete query.start_time
      if (bounds.end) query.end_time = bounds.end
      else delete query.end_time
    }

    const activeSource = sourceID()
    if (activeSource) {
      query.sources = [activeSource]
    } else if (query.sources && query.sources.length === 0) {
      delete query.sources
    }

    const applyIPFilter = (
      field: 'source_ip' | 'destination_ip',
      text: string,
    ) => {
      // 输入框留空时保留当前查询中的同名字段条件；只有显式填写才覆盖。
      if (!text.trim()) return
      const tokens = splitScopeTokens(text)
      const current = query.field_filters ?? []
      const remaining = current.filter(
        (filter) => canonicalScopeField(filter.field) !== field,
      )
      if (tokens.length > 0) {
        remaining.push({ field, operator: 'in', value: tokens })
      }
      if (remaining.length > 0) query.field_filters = remaining
      else delete query.field_filters
    }
    applyIPFilter('source_ip', sourceIPText())
    applyIPFilter('destination_ip', destIPText())

    // 事件 ID 留空时沿用当前查询；填写但无可识别数字时由范围校验阻止生成。
    if (eventIDText().trim()) {
      const ids = parseEventIDs(eventIDText())
      if (ids.length > 0) query.event_ids = ids
    }

    return query
  }

  function scopeValidation(): string {
    if (timePreset() === 'custom') {
      const start = localInputToIso(customStart())
      const end = localInputToIso(customEnd())
      if (!start || !end) return '自定义时间范围无效：请填写完整的开始时间与结束时间。'
      if (new Date(start).getTime() > new Date(end).getTime()) {
        return '自定义时间范围无效：开始时间不能晚于结束时间。'
      }
    }
    if (eventIDText().trim() && parseEventIDs(eventIDText()).length === 0) {
      return '事件 ID 输入无效：请填写以逗号、空格或分号分隔的数字。'
    }
    if (dimensions().length === 0) return '请至少选择一个聚合维度，否则报告缺少统计维度。'
    return ''
  }

  function timeRangeSummary(): string {
    const query = scopedQuery()
    if (!query.start_time && !query.end_time) return '不限制时间范围'
    return `${formatTime(query.start_time)} → ${formatTime(query.end_time)}`
  }

  function hasScopeFilters(): boolean {
    return (
      splitScopeTokens(sourceIPText()).length > 0 ||
      splitScopeTokens(destIPText()).length > 0 ||
      parseEventIDs(eventIDText()).length > 0
    )
  }

  function eventIDHint(): string {
    const text = eventIDText().trim()
    if (!text) return '可选'
    const ids = parseEventIDs(text)
    return ids.length > 0 ? `已识别 ${ids.length} 个` : '未识别到有效数字'
  }

  return (
    <Modal
      open={props.open}
      title="生成分析报告"
      subtitle="按时间、日志类型、IP 与事件 ID 范围生成 Markdown 调查报告"
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
            AI 设置
          </Button>
          <span class="grow" />
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Button
            size="sm"
            variant="primary"
            loading={generating()}
            disabled={
              !providers.provider() ||
              providers.loading() ||
              !title().trim() ||
              Boolean(scopeValidation())
            }
            onClick={() => void generate()}
          >
            {result() ? '重新生成' : '生成报告'}
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '12px' }}>
        <div>
          <div class="section-label">当前查询</div>
          <QueryTags query={props.query} />
        </div>

        <div>
          <div class="section-label">报告范围</div>
          <div class="stack" style={{ gap: '10px' }}>
            <Field label="时间范围" hint={timeRangeSummary()}>
              <div class="row wrap" style={{ gap: '4px' }}>
                <For each={REPORT_TIME_PRESETS}>
                  {(item) => (
                    <button
                      type="button"
                      class={`chip ${timePreset() === item.value ? 'selected' : ''}`}
                      title={item.title}
                      aria-pressed={timePreset() === item.value}
                      disabled={generating()}
                      onClick={() => setTimePreset(item.value)}
                    >
                      {item.label}
                    </button>
                  )}
                </For>
              </div>
              <Show when={timePreset() === 'custom'}>
                <div class="row wrap" style={{ gap: '6px', 'margin-top': '7px' }}>
                  <input
                    type="datetime-local"
                    class="input"
                    style={reportRangeInputStyle}
                    value={customStart()}
                    title="开始时间（本地时间）"
                    aria-label="报告开始时间"
                    disabled={generating()}
                    onInput={(event) => setCustomStart(event.currentTarget.value)}
                  />
                  <span class="faint mono-sm">→</span>
                  <input
                    type="datetime-local"
                    class="input"
                    style={reportRangeInputStyle}
                    value={customEnd()}
                    title="结束时间（本地时间）"
                    aria-label="报告结束时间"
                    disabled={generating()}
                    onInput={(event) => setCustomEnd(event.currentTarget.value)}
                  />
                </div>
              </Show>
            </Field>

            <div class="form-row">
              <Field
                label="日志类型"
                hint={
                  sourcesLoading()
                    ? '加载中…'
                    : sources().length > 0
                      ? `${sources().length} 个日志源`
                      : '可选'
                }
              >
                <Select
                  value={sourceID()}
                  options={sourceOptions()}
                  disabled={generating() || sourcesLoading()}
                  onInput={setSourceID}
                />
              </Field>
              <Field label="事件 ID" hint={eventIDHint()}>
                <TextInput
                  value={eventIDText()}
                  disabled={generating()}
                  placeholder="443 等事件 ID，逗号分隔"
                  onInput={setEventIDText}
                />
              </Field>
            </div>

            <Show when={sourcesError()}>
              <div class="detail-note">
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <span class="grow" style={{ 'line-height': '1.6' }}>
                    日志源加载失败：{sourcesError()}。可继续按当前查询范围生成。
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={sourcesLoading()}
                    onClick={() => void loadSources()}
                  >
                    重试
                  </Button>
                </div>
              </div>
            </Show>

            <div class="form-row">
              <Field label="来源 IP" hint="可选，多个值用逗号或空格分隔">
                <TextInput
                  value={sourceIPText()}
                  disabled={generating()}
                  placeholder="10.0.0.1, 10.0.0.2"
                  onInput={setSourceIPText}
                />
              </Field>
              <Field label="目的 IP" hint="可选，多个值用逗号或空格分隔">
                <TextInput
                  value={destIPText()}
                  disabled={generating()}
                  placeholder="10.0.0.1, 10.0.0.2"
                  onInput={setDestIPText}
                />
              </Field>
            </div>

            <Field label="聚合维度" hint="报告中按所选维度统计日志范围分布">
              <div class="row wrap" style={{ gap: '4px' }}>
                <For each={REPORT_DIMENSIONS}>
                  {(dimension) => (
                    <button
                      type="button"
                      class={`chip ${
                        dimensions().includes(dimension.value) ? 'selected' : ''
                      }`}
                      aria-pressed={dimensions().includes(dimension.value)}
                      disabled={generating()}
                      onClick={() =>
                        setDimensions(toggleValue(dimensions(), dimension.value))
                      }
                    >
                      {dimension.label}
                    </button>
                  )}
                </For>
              </div>
            </Field>

            <Show when={scopeValidation()}>
              <div class="mono-sm ember">{scopeValidation()}</div>
            </Show>

            <div>
              <div class="mono-sm faint" style={{ 'margin-bottom': '4px' }}>
                最终报告范围
              </div>
              <QueryTags query={scopedQuery()} />
            </div>

            <Show when={hasScopeFilters()}>
              <div class="detail-note">
                IP 与事件 ID 条件会覆盖当前查询中的同名字段；多个值可用逗号、空格、分号或顿号分隔。
              </div>
            </Show>
          </div>
        </div>

        <Field label="报告标题">
          <TextInput
            value={title()}
            disabled={generating()}
            placeholder="例如：web-01 异常登录调查报告"
            onInput={setTitle}
          />
        </Field>

        <div class="form-row">
          <Field label="分析问题 / 关注点" hint="可选">
            <TextInput
              value={question()}
              disabled={generating()}
              placeholder="例如：重点分析 502 激增的原因与影响范围"
              onInput={setQuestion}
            />
          </Field>
          <Field
            label="采样上限（条）"
            hint="仅限制提交给 AI 的日志条数；统计范围由上方时间/类型/IP/事件 ID 决定"
          >
            <TextInput
              type="number"
              value={maxLogs()}
              disabled={generating()}
              onInput={setMaxLogs}
            />
          </Field>
        </div>

        <div class="detail-note">
          当前报告请求没有独立的 question 字段；填写的问题会附加到报告标题中，作为 AI 的分析关注点一并提交。
        </div>

        <div class="form-row">
          <AIProviderField
            providers={providers.providers()}
            loading={providers.loading()}
            error={providers.error()}
            value={providers.provider()}
            disabled={generating()}
            onSelect={providers.setProvider}
            onReload={() => void providers.reload()}
            onOpenSettings={props.onOpenSettings}
          />
          <Field label="保存到文件" hint="可选，留空由后端决定">
            <div class="row" style={{ gap: '6px' }}>
              <input
                class="input mono"
                style={{ flex: 1, 'min-width': 0, 'font-size': '10px' }}
                value={outputPath()}
                disabled={generating()}
                placeholder="选择或输入 .md 保存路径"
                aria-label="报告保存路径"
                onInput={(event) => setOutputPath(event.currentTarget.value)}
              />
              <Button
                size="sm"
                variant="ghost"
                loading={choosingPath()}
                disabled={generating()}
                onClick={() => void chooseOutputPath()}
              >
                选择…
              </Button>
            </div>
          </Field>
        </div>

        <Show when={error()}>
          <div class="danger-note">
            <div class="row between" style={{ gap: '8px' }}>
              <span class="grow">{error()}</span>
              <Button size="sm" variant="ghost" onClick={props.onOpenSettings}>
                检查 AI 设置
              </Button>
            </div>
          </div>
        </Show>

        <PageBusy visible={generating()} text="AI 正在生成分析报告…" />

        <Show when={result()}>
          {(resultAccessor) => {
            const data = resultAccessor()
            return (
              <div class="stack" style={{ gap: '9px' }}>
                <div class="row between wrap" style={{ gap: '6px' }}>
                  <div class="row wrap" style={{ gap: '6px' }}>
                    <Badge tone="mint">报告已生成</Badge>
                    <Badge tone="muted">{data.provider || 'AI'}</Badge>
                    <span class="mono-sm faint">
                      {formatTime(data.generated_at)} · {data.format || 'markdown'}
                    </span>
                  </div>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => void copyReport(data.content ?? '')}
                  >
                    <Icon name="copy" size={12} />
                    复制 Markdown
                  </Button>
                </div>

                <Show when={(data.aggregates?.length ?? 0) > 0}>
                  <div>
                    <div class="section-label">分析范围聚合</div>
                    <div class="stack" style={{ gap: '6px' }}>
                      <For each={data.aggregates ?? []}>
                        {(aggregate) => (
                          <div
                            class="panel"
                            style={{ padding: '7px 9px', 'box-shadow': 'none' }}
                          >
                            <div class="row between wrap" style={{ gap: '6px' }}>
                              <span
                                style={{
                                  'font-size': '11px',
                                  'font-weight': 500,
                                  color: 'var(--text)',
                                }}
                              >
                                {reportDimensionLabel(aggregate.field)}
                              </span>
                              <span class="mono-sm faint">
                                合计 {formatInt(aggregate.total)} 条
                              </span>
                            </div>
                            <Show
                              when={(aggregate.items?.length ?? 0) > 0}
                              fallback={
                                <div
                                  class="mono-sm faint"
                                  style={{ 'margin-top': '4px' }}
                                >
                                  无统计项。
                                </div>
                              }
                            >
                              <div
                                class="row wrap"
                                style={{ gap: '4px', 'margin-top': '5px' }}
                              >
                                <For
                                  each={(aggregate.items ?? []).slice(
                                    0,
                                    REPORT_AGGREGATE_ITEM_LIMIT,
                                  )}
                                >
                                  {(item) => {
                                    const percent = aggregatePercent(
                                      item.count,
                                      aggregate.total,
                                    )
                                    return (
                                      <span
                                        class="tag"
                                        title={`${item.key || '(空值)'} · ${formatInt(
                                          item.count,
                                        )} 条${percent ? ` · ${percent}` : ''}`}
                                      >
                                        {truncate(item.key, 36) || '(空值)'} ·{' '}
                                        {formatInt(item.count)}
                                        <Show when={percent}>{` · ${percent}`}</Show>
                                      </span>
                                    )
                                  }}
                                </For>
                                <Show
                                  when={
                                    (aggregate.items?.length ?? 0) >
                                    REPORT_AGGREGATE_ITEM_LIMIT
                                  }
                                >
                                  <span class="mono-sm faint">
                                    另有{' '}
                                    {(aggregate.items?.length ?? 0) -
                                      REPORT_AGGREGATE_ITEM_LIMIT}{' '}
                                    项
                                  </span>
                                </Show>
                              </div>
                            </Show>
                          </div>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>

                <Show
                  when={data.output_path}
                  fallback={
                    <Show when={outputPath().trim()}>
                      <div class="detail-note">
                        已请求写入 {outputPath()}，但后端未返回写入路径，请到目标位置确认文件是否生成。
                      </div>
                    </Show>
                  }
                >
                  {(pathAccessor) => (
                    <div class="success-note">
                      <div class="row between wrap" style={{ gap: '8px' }}>
                        <span class="grow" style={{ 'word-break': 'break-all' }}>
                          报告已写入：<code>{pathAccessor()}</code>
                        </span>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => void showInExplorer(pathAccessor())}
                        >
                          在资源管理器中显示
                        </Button>
                      </div>
                    </div>
                  )}
                </Show>

                <Show
                  when={data.content}
                  fallback={
                    <EmptyState
                      icon="book"
                      title="报告内容为空"
                      text="后端返回了空报告，请调整查询条件或更换 AI Provider 后重试。"
                    />
                  }
                >
                  <div
                    class="scrollable scroll-thin"
                    style={{
                      'max-height': '44vh',
                      overflow: 'auto',
                      border: '1px solid var(--border)',
                      'border-radius': '3px',
                    }}
                  >
                    <pre
                      style={{
                        margin: 0,
                        padding: '10px 12px',
                        'font-family': 'var(--font-mono)',
                        'font-size': '10.5px',
                        'line-height': '1.65',
                        'white-space': 'pre-wrap',
                        'word-break': 'break-word',
                        color: 'var(--text)',
                      }}
                    >
                      {data.content}
                    </pre>
                  </div>
                </Show>
              </div>
            )
          }}
        </Show>
      </div>
    </Modal>
  )
}
