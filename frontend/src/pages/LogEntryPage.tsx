import { For, JSX, Show, createEffect, createSignal, onCleanup } from 'solid-js'
import type { ContextResult, LogEntry } from '../types'
import { api, errorText } from '../api'
import {
  backToLogs,
  consumeLogAIAutoOpen,
  currentProject,
  logAIAutoOpenDocID,
  notify,
  openLogDetail,
  selectedLogDocID,
} from '../stores/app'
import {
  Badge,
  Button,
  CodeBlock,
  EmptyState,
  Icon,
  LevelBadge,
  PageBusy,
  Tabs,
} from '../components/ui'
import { LogAIAnalysisModal } from '../components/log/LogAIAnalysisModal'
import {
  copyText,
  fieldLabel,
  formatTime,
  formatTimeShort,
  friendlyLogMessage,
  riskTone,
  safeJson,
  truncate,
} from '../utils'

type DetailTab = 'parsed' | 'raw' | 'fields'

/** 单条日志独立查看页：完整解析结果、原始日志、字段与相邻日志。 */
export function LogEntryPage(): JSX.Element {
  const [entry, setEntry] = createSignal<LogEntry | null>(null)
  const [context, setContext] = createSignal<ContextResult | null>(null)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal('')
  const [tab, setTab] = createSignal<DetailTab>('parsed')
  const [aiOpen, setAiOpen] = createSignal(false)

  let loadSeq = 0

  const jsonText = () => {
    const value = entry()
    if (!value) return ''
    try {
      return JSON.stringify(value, null, 2)
    } catch {
      return safeJson(value, 50000)
    }
  }

  async function load(docID: number | null) {
    const project = currentProject()
    if (!project || docID == null) {
      setEntry(null)
      setContext(null)
      setError(project ? '未选择日志' : '请先打开项目')
      return
    }
    const seq = ++loadSeq
    setLoading(true)
    setError('')
    setEntry(null)
    setContext(null)
    try {
      const [item, around] = await Promise.all([
        api.query.getEntry(project.id, docID),
        api.query.context(project.id, docID, 3, 3).catch(() => null),
      ])
      if (seq !== loadSeq) return
      setEntry(item)
      setContext(around)
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setError(message)
      notify('error', '加载日志失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    void load(selectedLogDocID())
  })

  // 从日志列表/详情抽屉发起的一次性请求：等待目标日志加载完成后自动打开 AI 分析。
  createEffect(() => {
    const pending = logAIAutoOpenDocID()
    const item = entry()
    if (pending == null || !item || item.doc_id !== pending) return
    if (consumeLogAIAutoOpen(pending)) setAiOpen(true)
  })

  onCleanup(() => {
    loadSeq++
  })

  const anchorIndex = () => {
    const ctx = context()
    const item = entry()
    if (!ctx || !item) return -1
    return ctx.entries.findIndex((candidate) => candidate.doc_id === item.doc_id)
  }

  const previous = () => {
    const ctx = context()
    const idx = anchorIndex()
    return ctx && idx > 0 ? ctx.entries[idx - 1] : null
  }

  const next = () => {
    const ctx = context()
    const idx = anchorIndex()
    return ctx && idx >= 0 && idx < ctx.entries.length - 1 ? ctx.entries[idx + 1] : null
  }

  const copy = async (text: string, label: string) => {
    try {
      await copyText(text)
      notify('success', '已复制', label)
    } catch {
      notify('error', '复制失败', '无法写入剪贴板。')
    }
  }

  const title = () => {
    const item = entry()
    return item ? `日志 #${item.doc_id}` : '单条日志'
  }

  return (
    <div class="page wide log-entry-page">
      <header class="log-entry-header">
        <div class="row" style={{ gap: '12px', 'align-items': 'center', 'min-width': 0 }}>
          <Button size="sm" variant="ghost" onClick={backToLogs}>
            <Icon name="arrow-left" size={14} />
            返回日志列表
          </Button>
          <div class="log-entry-heading">
            <div class="eyebrow" style={{ 'margin-bottom': '2px' }}>
              日志分析 / 单条日志
            </div>
            <h1 class="page-title" style={{ 'font-size': '22px' }}>
              {title()}
            </h1>
          </div>
        </div>
        <div class="page-actions">
          <Button size="sm" variant="ghost" disabled={!previous()} onClick={() => previous() && openLogDetail(previous()!.doc_id)}>
            <Icon name="chevron-left" size={13} />
            上一条
          </Button>
          <Button size="sm" variant="ghost" disabled={!next()} onClick={() => next() && openLogDetail(next()!.doc_id)}>
            下一条
            <Icon name="chevron-right" size={13} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={!entry()}
            onClick={() => entry() && void copy(jsonText(), '完整解析 JSON')}
          >
            <Icon name="copy" size={13} />
            复制 JSON
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={!entry()}
            onClick={() => entry() && void copy(entry()!.raw_log || entry()!.message || '', '原始日志')}
          >
            <Icon name="file" size={13} />
            复制原始日志
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!entry()}
            onClick={() => setAiOpen(true)}
          >
            <Icon name="wand" size={13} />
            AI 分析
          </Button>
        </div>
      </header>

      <Show
        when={currentProject()}
        fallback={
          <div class="panel">
            <div class="panel-body">
              <EmptyState
                icon="folder"
                title="尚未打开项目"
                text="单条日志查看需要项目上下文，请先返回项目列表打开一个项目。"
                action={
                  <Button variant="primary" onClick={backToLogs}>
                    返回日志列表
                  </Button>
                }
              />
            </div>
          </div>
        }
      >
        <Show when={!loading() && !error() && !entry()}>
          <div class="panel">
            <div class="panel-body">
              <EmptyState icon="file" title="未选择日志" text="从日志工作台选择一条日志后即可查看完整内容。" />
            </div>
          </div>
        </Show>

        <Show when={error()}>
          <div class="panel">
            <div class="panel-body">
              <EmptyState
                icon="alert"
                title="加载失败"
                text={error()}
                action={
                  <div class="row">
                    <Button variant="primary" onClick={() => void load(selectedLogDocID())}>
                      重新加载
                    </Button>
                    <Button variant="ghost" onClick={backToLogs}>
                      返回列表
                    </Button>
                  </div>
                }
              />
            </div>
          </div>
        </Show>

        <Show when={entry()}>
          {(item) => (
            <div class="log-entry-layout">
              <section class="panel log-entry-main">
                <div class="panel-head">
                  <div class="row wrap" style={{ gap: '9px', 'min-width': 0 }}>
                    <LevelBadge level={item().level} />
                    <span class="mono-sm muted">{formatTime(item().timestamp)}</span>
                    <span class="mono-sm faint truncate" style={{ 'max-width': '380px' }}>
                      {item().source_host || '未知主机'}
                      {item().source_file ? ` · ${item().source_file}` : ''}
                    </span>
                  </div>
                  <div class="row wrap" style={{ gap: '6px' }}>
                    <Show when={item().event_id != null}>
                      <Badge tone="sky">事件 ID {item().event_id}</Badge>
                    </Show>
                    <Show when={item().provider}>
                      <Badge tone="muted">{item().provider}</Badge>
                    </Show>
                    <Show when={item().risk_score != null}>
                      <Badge tone={riskTone(item().risk_score)}>风险 {item().risk_score}</Badge>
                    </Show>
                    <Show when={item().is_anomaly}>
                      <Badge tone="ember">异常</Badge>
                    </Show>
                    <Show when={item().is_evidence}>
                      <Badge tone="mint">证据</Badge>
                    </Show>
                  </div>
                </div>

                <div style={{ padding: '0 16px' }}>
                  <Tabs
                    items={[
                      { key: 'parsed', label: '解析结果 JSON' },
                      { key: 'raw', label: '原始日志' },
                      { key: 'fields', label: `字段 (${Object.keys(item().fields ?? {}).length})` },
                    ]}
                    active={tab()}
                    onChange={(key) => setTab(key as DetailTab)}
                  />
                </div>

                <div class="log-entry-content">
                  <Show when={tab() === 'parsed'}>
                    <div class="stack" style={{ gap: '8px' }}>
                      <div class="log-entry-hint">
                        完整 LogEntry：包含来源元数据、动态字段、原始日志与分析标记，内容不截断。
                      </div>
                      <CodeBlock text={jsonText()} maxHeight="calc(100vh - 360px)" />
                    </div>
                  </Show>

                  <Show when={tab() === 'raw'}>
                    <div class="stack" style={{ gap: '8px' }}>
                      <div class="log-entry-hint">采集器读取并交给解析器的原始内容，完整展示。</div>
                      <CodeBlock text={item().raw_log || item().message || '(空日志)'} maxHeight="calc(100vh - 360px)" />
                    </div>
                  </Show>

                  <Show when={tab() === 'fields'}>
                    <Show
                      when={Object.keys(item().fields ?? {}).length > 0}
                      fallback={<div class="mono-sm faint">该日志没有结构化字段。</div>}
                    >
                      <div class="scrollable" style={{ 'max-height': 'calc(100vh - 380px)', overflow: 'auto' }}>
                        <table class="data-table">
                          <thead>
                            <tr>
                              <th style={{ width: '30%' }}>字段</th>
                              <th>值</th>
                            </tr>
                          </thead>
                          <tbody>
                            <For each={Object.entries(item().fields ?? {})}>
                              {([key, value]) => (
                                <tr>
                                  <td style={{ 'word-break': 'break-all' }}>
                                    <div class="mono-sm" style={{ color: 'var(--text)' }}>
                                      {fieldLabel(key)}
                                    </div>
                                    <Show when={fieldLabel(key) !== key}>
                                      <div
                                        class="mono-sm faint"
                                        style={{ 'font-size': '9.5px', 'line-height': '1.4' }}
                                      >
                                        {key}
                                      </div>
                                    </Show>
                                  </td>
                                  <td class="mono-sm" style={{ 'word-break': 'break-all', 'white-space': 'pre-wrap' }}>
                                    {typeof value === 'object' ? safeJson(value, 20000) : String(value)}
                                  </td>
                                </tr>
                              )}
                            </For>
                          </tbody>
                        </table>
                      </div>
                    </Show>
                  </Show>
                </div>
              </section>

              <aside class="stack" style={{ gap: '14px' }}>
                <section class="panel">
                  <div class="panel-head">
                    <div class="panel-title">
                      <span class="tick" />
                      元数据
                    </div>
                  </div>
                  <div class="panel-body">
                    <dl class="kv">
                      <dt>时间</dt>
                      <dd>{formatTime(item().timestamp)}</dd>
                      <dt>级别</dt>
                      <dd><LevelBadge level={item().level} /></dd>
                      <dt>主机</dt>
                      <dd>{item().source_host || '—'}</dd>
                      <dt>源文件</dt>
                      <dd style={{ 'word-break': 'break-all' }}>{item().source_file || '—'}</dd>
                      <dt>文件偏移</dt>
                      <dd>{item().source_offset ?? '—'}</dd>
                      <dt>日志源</dt>
                      <dd style={{ 'word-break': 'break-all' }}>{item().source || '—'}</dd>
                      <dt>事件 ID</dt>
                      <dd>{item().event_id ?? '—'}</dd>
                      <dt>提供程序</dt>
                      <dd>{item().provider || '—'}</dd>
                      <dt>计算机</dt>
                      <dd>{item().computer || '—'}</dd>
                      <dt>用户</dt>
                      <dd>{item().user_id || '—'}</dd>
                      <dt>文档 ID</dt>
                      <dd>{item().doc_id}</dd>
                      <Show when={(item().tags?.length ?? 0) > 0}>
                        <dt>标签</dt>
                        <dd class="row wrap" style={{ gap: '4px' }}>
                          <For each={item().tags ?? []}>
                            {(tag) => <span class="tag">{tag}</span>}
                          </For>
                        </dd>
                      </Show>
                    </dl>
                  </div>
                </section>

                <section class="panel">
                  <div class="panel-head">
                    <div class="panel-title">
                      <span class="tick" />
                      相邻日志
                    </div>
                    <span class="mono-sm faint">{context()?.entries?.length ?? 0} 条</span>
                  </div>
                  <div class="panel-body" style={{ padding: '8px' }}>
                    <Show
                      when={(context()?.entries?.length ?? 0) > 1}
                      fallback={<div class="mono-sm faint" style={{ padding: '8px' }}>没有可用的相邻日志。</div>}
                    >
                      <div class="log-neighbor-list">
                        <For each={context()?.entries ?? []}>
                          {(candidate) => (
                            <button
                              type="button"
                              class={`log-neighbor ${candidate.doc_id === item().doc_id ? 'active' : ''}`}
                              onClick={() => openLogDetail(candidate.doc_id)}
                            >
                              <span class="log-neighbor-time">{formatTimeShort(candidate.timestamp)}</span>
                              <LevelBadge level={candidate.level} />
                              <span class="log-neighbor-message">
                                {truncate(friendlyLogMessage(candidate.message), 80) ||
                                  '(空消息)'}
                              </span>
                            </button>
                          )}
                        </For>
                      </div>
                    </Show>
                  </div>
                </section>

                <section class="panel">
                  <div class="panel-body" style={{ display: 'grid', gap: '8px' }}>
                    <Button variant="primary" onClick={backToLogs}>
                      <Icon name="search" size={14} />
                      返回日志工作台
                    </Button>
                    <div class="mono-sm faint">
                      提示：按上一条/下一条可快速浏览相邻日志；完整解析 JSON 可用于复核与上报。
                    </div>
                  </div>
                </section>
              </aside>
            </div>
          )}
        </Show>

        <Show when={loading()}>
          <div class="panel">
            <div class="panel-body">
              <PageBusy visible text="正在读取完整日志…" />
            </div>
          </div>
        </Show>
      </Show>

      <Show when={entry()}>
        {(item) => (
          <LogAIAnalysisModal
            open={aiOpen()}
            projectID={currentProject()?.id ?? ''}
            entry={item()}
            onClose={() => setAiOpen(false)}
          />
        )}
      </Show>
    </div>
  )
}
