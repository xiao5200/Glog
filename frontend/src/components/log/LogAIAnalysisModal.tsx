import { For, JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type { AIProviderInfo, LogAnalysisResult, LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { navigate, openLogDetail } from '../../stores/app'
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
  Segmented,
  Select,
  TextArea,
} from '../ui'
import {
  formatTime,
  friendlyLogMessage,
  severityTone,
  truncate,
} from '../../utils'

export interface LogAIAnalysisModalProps {
  open: boolean
  projectID: string
  entry: LogEntry
  onClose: () => void
}

type AnalysisMode = 'single' | 'context'

const CONTEXT_WINDOW_OPTIONS = [
  { value: '5', label: '±5 条' },
  { value: '10', label: '±10 条' },
  { value: '20', label: '±20 条' },
  { value: '50', label: '±50 条' },
  { value: '100', label: '±100 条' },
  { value: '200', label: '±200 条' },
]

const DEFAULT_CONTEXT_WINDOW = 20

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

/** 单条日志 AI 分析弹窗：仅分析本条或结合上下文综合分析。 */
export function LogAIAnalysisModal(props: LogAIAnalysisModalProps): JSX.Element {
  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [providersLoading, setProvidersLoading] = createSignal(false)
  const [providersError, setProvidersError] = createSignal<string | null>(null)
  const [provider, setProvider] = createSignal('')
  const [mode, setMode] = createSignal<AnalysisMode>('context')
  const [contextWindow, setContextWindow] = createSignal(DEFAULT_CONTEXT_WINDOW)
  const [question, setQuestion] = createSignal('')
  const [running, setRunning] = createSignal(false)
  const [runError, setRunError] = createSignal<string | null>(null)
  const [result, setResult] = createSignal<LogAnalysisResult | null>(null)
  const [showRaw, setShowRaw] = createSignal(false)

  let providerSeq = 0
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

  const canRun = () =>
    Boolean(props.projectID) &&
    Boolean(provider()) &&
    !providersLoading() &&
    configuredProviders().length > 0

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
      const message = errorText(err)
      setProviders([])
      setProvider('')
      setProvidersError(message)
    } finally {
      if (seq === providerSeq) setProvidersLoading(false)
    }
  }

  createEffect(
    on(
      () => [props.open, props.entry.doc_id] as const,
      ([open]) => {
        // 关闭或切换日志时作废进行中的请求，避免旧结果写入新上下文。
        runSeq++
        setRunning(false)
        setResult(null)
        setRunError(null)
        setShowRaw(false)
        if (!open) {
          providerSeq++
          setProvidersLoading(false)
          return
        }
        setQuestion('')
        setMode('context')
        setContextWindow(DEFAULT_CONTEXT_WINDOW)
        void loadProviders()
      },
    ),
  )

  async function runAnalysis() {
    const entry = props.entry
    const activeProvider = provider()
    if (!props.projectID || !activeProvider) return
    const seq = ++runSeq
    const contextSize = mode() === 'single' ? 0 : contextWindow()
    const questionText = question().trim()
    setRunning(true)
    setRunError(null)
    setResult(null)
    setShowRaw(false)
    try {
      const data = await api.ai.analyzeLogEntry({
        project_id: props.projectID,
        doc_id: entry.doc_id,
        provider: activeProvider,
        context_before: contextSize,
        context_after: contextSize,
        ...(questionText ? { question: questionText } : {}),
      })
      if (seq !== runSeq) return
      if (!props.open || props.entry.doc_id !== entry.doc_id) return
      setResult(data)
    } catch (err) {
      if (seq !== runSeq) return
      setRunError(errorText(err))
    } finally {
      if (seq === runSeq) setRunning(false)
    }
  }

  const openSettings = () => {
    props.onClose()
    navigate('settings')
  }

  return (
    <Modal
      open={props.open}
      title="AI 日志分析"
      subtitle={`日志 #${props.entry.doc_id} · ${props.entry.source_host || '未知主机'}`}
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Button
            size="sm"
            variant="primary"
            loading={running()}
            disabled={!canRun()}
            onClick={() => void runAnalysis()}
          >
            <Icon name="wand" size={12} />
            开始 AI 分析
          </Button>
        </>
      }
    >
      <div class="stack" style={{ gap: '14px' }}>
        <div>
          <div class="section-label">目标日志</div>
          <div
            style={{
              padding: '10px 12px',
              border: '1px solid var(--border)',
              'border-radius': 'var(--r-md)',
              background: 'var(--surface-soft)',
            }}
          >
            <div class="row wrap" style={{ gap: '8px', 'align-items': 'center' }}>
              <LevelBadge level={props.entry.level} />
              <span class="mono-sm muted">{formatTime(props.entry.timestamp)}</span>
              <span class="mono-sm faint">{props.entry.source_host || '未知主机'}</span>
              <Show when={props.entry.event_id != null}>
                <Badge tone="sky">事件 ID {props.entry.event_id}</Badge>
              </Show>
              <Show when={props.entry.provider}>
                <Badge tone="muted">{props.entry.provider}</Badge>
              </Show>
            </div>
            <div
              class="mono-sm"
              style={{
                'margin-top': '6px',
                'line-height': '1.6',
                'word-break': 'break-word',
                color: 'var(--text-muted)',
              }}
            >
              {truncate(friendlyLogMessage(props.entry.message), 300) || '(空消息)'}
            </div>
          </div>
        </div>

        <div>
          <Field label="AI Provider" hint={selectedProvider()?.model || undefined}>
            <Select
              value={provider()}
              options={providerOptions()}
              disabled={providersLoading() || running()}
              onInput={setProvider}
            />
          </Field>
          <PageBusy visible={providersLoading()} text="正在读取 AI 服务配置…" />
          <Show when={providersError()}>
            <div class="danger-note" style={{ 'margin-top': '6px' }}>
              <div class="row between wrap" style={{ gap: '8px' }}>
                <span class="grow" style={{ 'line-height': '1.6' }}>
                  {providersError()}
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
              configuredProviders().length === 0
            }
          >
            <div class="detail-note" style={{ 'margin-top': '6px' }}>
              <div class="row between wrap" style={{ gap: '8px' }}>
                <span class="grow" style={{ 'line-height': '1.6' }}>
                  尚未配置可用的 AI 服务。请前往设置 → AI 服务填写 Provider 配置与 API Key。
                </span>
                <Button size="sm" variant="primary" onClick={openSettings}>
                  前往设置
                </Button>
              </div>
            </div>
          </Show>
        </div>

        <Field label="分析范围">
          <Segmented
            options={[
              { value: 'single', label: '仅分析本条' },
              { value: 'context', label: '上下文综合分析' },
            ]}
            value={mode()}
            onChange={(value) => setMode(value as AnalysisMode)}
          />
        </Field>

        <Show when={mode() === 'context'}>
          <Field
            label="上下文窗口"
            hint={
              running()
                ? '分析进行中'
                : `前后各 ${contextWindow()} 条（最多约 ${contextWindow() * 2 + 1} 条，超长时围绕目标日志自动截断）`
            }
          >
            <Select
              value={String(contextWindow())}
              options={CONTEXT_WINDOW_OPTIONS}
              disabled={running()}
              onInput={(value) =>
                setContextWindow(Number(value) || DEFAULT_CONTEXT_WINDOW)
              }
            />
          </Field>
        </Show>

        <Field label="补充问题" hint="可选，用于引导 AI 关注点">
          <TextArea
            value={question()}
            rows={3}
            placeholder="例如：这条日志是否代表一次暴力破解或横向移动尝试？"
            disabled={running()}
            onInput={setQuestion}
          />
        </Field>

        <PageBusy visible={running()} text="正在调用 AI 分析日志，可能需要数十秒…" />

        <Show when={runError()}>
          <div class="danger-note" style={{ 'line-height': '1.6' }}>
            {runError()}
          </div>
        </Show>

        <Show when={result()}>
          {(resultAccessor) => {
            const data = resultAccessor()
            return (
              <div class="stack" style={{ gap: '10px' }}>
                <div class="row wrap" style={{ gap: '6px', 'align-items': 'center' }}>
                  <Badge tone={severityTone(data.risk_level)}>
                    风险 {data.risk_level || '未知'}
                  </Badge>
                  <Badge tone="muted">{data.provider || provider() || 'AI'}</Badge>
                  <span class="mono-sm faint">
                    已分析 {data.analyzed ?? 0} 条 · {formatTime(data.generated_at)}
                  </span>
                </div>

                <div class="success-note" style={{ 'line-height': '1.7', 'white-space': 'pre-wrap' }}>
                  {data.summary || 'AI 未返回摘要。'}
                </div>

                <div>
                  <div class="mono-sm faint" style={{ 'margin-bottom': '3px' }}>
                    关键发现
                  </div>
                  <BulletList items={data.findings ?? []} empty="未返回发现项。" />
                </div>

                <div>
                  <div class="mono-sm faint" style={{ 'margin-bottom': '3px' }}>
                    处置建议
                  </div>
                  <BulletList items={data.suggestions ?? []} empty="未返回建议。" />
                </div>

                <Show when={(data.related_doc_ids?.length ?? 0) > 0}>
                  <div>
                    <div class="mono-sm faint" style={{ 'margin-bottom': '4px' }}>
                      相关日志
                    </div>
                    <div class="row wrap" style={{ gap: '4px' }}>
                      <For each={data.related_doc_ids ?? []}>
                        {(docID) => (
                          <Button
                            size="sm"
                            variant="ghost"
                            title={`打开文档 #${docID}`}
                            onClick={() => {
                              props.onClose()
                              openLogDetail(docID)
                            }}
                          >
                            <Icon name="file" size={11} />#{docID}
                          </Button>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>

                <Show when={data.raw_response}>
                  <div>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setShowRaw((value) => !value)}
                    >
                      <Icon name={showRaw() ? 'chevron-up' : 'chevron-down'} size={12} />
                      {showRaw() ? '隐藏原始响应' : '显示原始响应'}
                    </Button>
                    <Show when={showRaw()}>
                      <div style={{ 'margin-top': '6px' }}>
                        <CodeBlock text={data.raw_response ?? ''} maxHeight="240px" />
                      </div>
                    </Show>
                  </div>
                </Show>
              </div>
            )
          }}
        </Show>

        <Show when={!running() && !runError() && !result() && configuredProviders().length > 0}>
          <EmptyState
            icon="wand"
            title="准备就绪"
            text="选择分析范围与 Provider 后，点击“开始 AI 分析”查看风险结论、发现与建议。"
          />
        </Show>
      </div>
    </Modal>
  )
}
