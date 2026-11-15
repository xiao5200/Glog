import { For, JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type { AnomalyDetectionResult, Investigation, LogEntry, Query } from '../../types'
import { api, errorText } from '../../api'
import { notify, openLogDetailWithAI } from '../../stores/app'
import {
  Badge,
  Button,
  CodeBlock,
  Drawer,
  EmptyState,
  Field,
  Icon,
  LevelBadge,
  PageBusy,
  TextArea,
  TextInput,
} from '../ui'
import {
  copyText,
  fieldLabel,
  formatTime,
  parseTime,
  riskTone,
  severityTone,
  truncate,
} from '../../utils'

export interface LogDetailDrawerProps {
  open: boolean
  projectID: string
  entry: LogEntry | null
  onClose: () => void
  onOpenContext: (entry: LogEntry) => void
  onOpenFullLog: (entry: LogEntry) => void
  onEntryUpdated: (entry: LogEntry) => void
}

function FieldValue(props: { value: unknown }): JSX.Element {
  const complex = () => props.value != null && typeof props.value === 'object'
  const text = () => {
    const value = props.value
    if (value == null) return value === null ? 'null' : 'undefined'
    if (typeof value === 'string') return value
    if (typeof value === 'number' || typeof value === 'boolean') return String(value)
    try {
      return JSON.stringify(value, null, 2)
    } catch {
      return String(value)
    }
  }
  return (
    <Show
      when={complex()}
      fallback={
        <span class="mono-sm" style={{ 'word-break': 'break-word' }}>
          {text()}
        </span>
      }
    >
      <pre
        class="code-block"
        style={{
          margin: 0,
          padding: '6px 8px',
          'max-height': '180px',
          'font-size': '10px',
          'line-height': '1.55',
        }}
      >
        {text()}
      </pre>
    </Show>
  )
}

function BulletList(props: { items: string[]; empty?: string }): JSX.Element {
  return (
    <Show
      when={props.items?.length > 0}
      fallback={<div class="mono-sm faint">{props.empty ?? '无'}</div>}
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
          {(item) => (
            <li>
              {item}
            </li>
          )}
        </For>
      </ul>
    </Show>
  )
}

export function LogDetailDrawer(props: LogDetailDrawerProps): JSX.Element {
  const [investigations, setInvestigations] = createSignal<Investigation[]>([])
  const [investigationsLoading, setInvestigationsLoading] = createSignal(false)
  const [investigationsError, setInvestigationsError] = createSignal<string | null>(null)
  const [selectedInvestigation, setSelectedInvestigation] = createSignal('')
  const [evidenceNote, setEvidenceNote] = createSignal('')
  const [markingEvidence, setMarkingEvidence] = createSignal(false)
  const [showNewInvestigation, setShowNewInvestigation] = createSignal(false)
  const [newTitle, setNewTitle] = createSignal('')
  const [newDescription, setNewDescription] = createSignal('')
  const [creatingInvestigation, setCreatingInvestigation] = createSignal(false)

  const [aiLoading, setAiLoading] = createSignal(false)
  const [aiError, setAiError] = createSignal<string | null>(null)
  const [aiResult, setAiResult] = createSignal<AnomalyDetectionResult | null>(null)

  let investigationSeq = 0
  let aiSeq = 0

  const defaultInvestigationTitle = (entry: LogEntry) =>
    entry.event_id != null ? `事件 ${entry.event_id} 调查` : `日志 #${entry.doc_id} 调查`

  async function loadInvestigations(selectID?: string) {
    const projectID = props.projectID
    if (!projectID) return
    const seq = ++investigationSeq
    setInvestigationsLoading(true)
    setInvestigationsError(null)
    try {
      const list = await api.investigations.list(projectID)
      if (seq !== investigationSeq) return
      const items = Array.isArray(list) ? list : []
      setInvestigations(items)
      if (selectID && items.some((item) => item.id === selectID)) {
        setSelectedInvestigation(selectID)
        return
      }
      const current = selectedInvestigation()
      if (current && !items.some((item) => item.id === current)) {
        setSelectedInvestigation('')
      }
      if (!selectedInvestigation() && items.length > 0) {
        const open = items.find((item) => item.status === 'open') ?? items[0]
        setSelectedInvestigation(open.id)
      }
    } catch (err) {
      if (seq !== investigationSeq) return
      const text = errorText(err)
      setInvestigationsError(text)
      setInvestigations([])
      notify('error', '调查工单加载失败', text, 6000)
    } finally {
      if (seq === investigationSeq) setInvestigationsLoading(false)
    }
  }

  createEffect(
    on(
      () => [props.open, props.entry?.doc_id] as const,
      ([open]) => {
        aiSeq++
        setAiLoading(false)
        if (!open) return
        const entry = props.entry
        if (!entry) return
        setAiError(null)
        setAiResult(null)
        setAiLoading(false)
        setEvidenceNote('')
        setShowNewInvestigation(false)
        setNewTitle(defaultInvestigationTitle(entry))
        setNewDescription(truncate(entry.message, 500))
        void loadInvestigations()
      },
    ),
  )

  async function copyRawLog() {
    const entry = props.entry
    if (!entry) return
    try {
      await copyText(entry.raw_log || entry.message || '')
      notify('success', '已复制原始日志', `日志 #${entry.doc_id}`)
    } catch (err) {
      notify('error', '复制失败', errorText(err))
    }
  }

  async function markAsEvidence() {
    const entry = props.entry
    if (!entry) return
    const investigationID = selectedInvestigation()
    if (!investigationID) {
      notify('warn', '请选择调查工单', '请先选择一个工单，或新建调查工单。')
      return
    }
    setMarkingEvidence(true)
    try {
      await api.investigations.markEvidence(
        investigationID,
        entry.doc_id,
        evidenceNote().trim() || `来自日志详情：doc #${entry.doc_id}`,
      )
      notify('success', '已标记为证据', `日志 #${entry.doc_id} 已加入调查工单。`)
      if (props.entry?.doc_id === entry.doc_id) {
        setEvidenceNote('')
        props.onEntryUpdated({ ...entry, is_evidence: true })
      }
    } catch (err) {
      notify('error', '标记证据失败', errorText(err), 7000)
    } finally {
      setMarkingEvidence(false)
    }
  }

  async function createInvestigation() {
    const entry = props.entry
    if (!entry) return
    const title = newTitle().trim()
    if (!title) {
      notify('warn', '请输入工单标题', '创建调查工单需要填写标题。')
      return
    }
    setCreatingInvestigation(true)
    try {
      const investigation = await api.investigations.create({
        project_id: props.projectID,
        title,
        description: newDescription().trim() || entry.message,
        clue_type: 'log_entry',
        clue_value: String(entry.id || entry.doc_id),
        tags: [String(entry.level ?? ''), entry.source_host]
          .filter((value): value is string => Boolean(value)),
        risk_score: entry.risk_score,
        time_range: entry.timestamp
          ? { start: entry.timestamp, end: entry.timestamp }
          : undefined,
        doc_ids: [entry.doc_id],
      })
      notify('success', '调查工单已创建', investigation?.title || title)
      if (props.entry?.doc_id !== entry.doc_id) {
        await loadInvestigations(investigation?.id)
        return
      }
      props.onEntryUpdated({ ...entry, is_evidence: true })
      setShowNewInvestigation(false)
      await loadInvestigations(investigation?.id)
    } catch (err) {
      notify('error', '创建工单失败', errorText(err), 7000)
    } finally {
      setCreatingInvestigation(false)
    }
  }

  async function explainWithAI() {
    const entry = props.entry
    if (!entry) return
    const seq = ++aiSeq
    const stillCurrent = () =>
      seq === aiSeq && props.open && props.entry?.doc_id === entry.doc_id
    setAiLoading(true)
    setAiError(null)
    setAiResult(null)
    try {
      const providers = await api.ai.providers()
      if (!stillCurrent()) return
      const configured = (Array.isArray(providers) ? providers : []).filter(
        (provider) => provider.configured,
      )
      if (configured.length === 0) {
        const message = '尚未配置 AI 服务。请前往系统设置 → AI 服务完成配置。'
        setAiError(message)
        notify('warn', 'AI 未配置', '请先在系统设置中配置 AI Provider。', 6000)
        return
      }
      const chosen = configured.find((provider) => provider.enabled) ?? configured[0]
      const query: Query = { limit: 1 }
      const time = parseTime(entry.timestamp)
      if (time) {
        query.start_time = new Date(time.getTime() - 2000).toISOString()
        query.end_time = new Date(time.getTime() + 2000).toISOString()
      }
      if (entry.level) query.levels = [String(entry.level)]
      const result = await api.ai.detectAnomaly({
        project_id: props.projectID,
        provider: chosen.provider,
        query,
        max_logs: 1,
      })
      if (!stillCurrent()) return
      setAiResult(result)
    } catch (err) {
      if (!stillCurrent()) return
      const text = errorText(err)
      setAiError(text)
      notify('error', 'AI 解释失败', text, 7000)
    } finally {
      if (seq === aiSeq) setAiLoading(false)
    }
  }

  const title = () => (props.entry ? `日志 #${props.entry.doc_id}` : '日志详情')
  const subtitle = () => {
    const entry = props.entry
    if (!entry) return undefined
    return `${formatTime(entry.timestamp)} · ${entry.source_host || '未知主机'}`
  }

  return (
    <Drawer
      open={props.open && Boolean(props.entry)}
      title={title()}
      subtitle={subtitle()}
      wide
      onClose={props.onClose}
      footer={
        <>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              const entry = props.entry
              if (!entry) return
              props.onOpenFullLog(entry)
              props.onClose()
            }}
          >
            <Icon name="file" size={12} />
            打开独立日志页
          </Button>
          <Button size="sm" variant="ghost" onClick={() => void copyRawLog()}>
            <Icon name="copy" size={12} />
            复制原始日志
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              const entry = props.entry
              if (entry) props.onOpenContext(entry)
            }}
          >
            <Icon name="layers" size={12} />
            查看上下文
          </Button>
          <Button size="sm" variant="primary" onClick={props.onClose}>
            关闭
          </Button>
        </>
      }
    >
      <Show
        keyed
        when={props.entry}
        fallback={<EmptyState icon="file" title="未选择日志" text="关闭后重新从列表选择一条日志。" />}
      >
        {(entry) => (
          <div class="stack" style={{ gap: '14px' }}>
            <div>
              <div class="section-label">元数据</div>
              <dl class="kv">
                <dt>时间</dt>
                <dd title={entry.timestamp}>{formatTime(entry.timestamp) || '—'}</dd>
                <dt>级别</dt>
                <dd>
                  <LevelBadge level={entry.level} />
                </dd>
                <dt>来源主机</dt>
                <dd>{entry.source_host || '—'}</dd>
                <dt>源文件</dt>
                <dd class="mono-sm" style={{ 'word-break': 'break-all' }}>
                  {entry.source_file || '—'}
                </dd>
                <dt>文件偏移</dt>
                <dd>{entry.source_offset ?? '—'}</dd>
                <dt>日志源</dt>
                <dd>{entry.source || '—'}</dd>
                <dt>事件 ID</dt>
                <dd>{entry.event_id ?? '—'}</dd>
                <dt>提供程序</dt>
                <dd>{entry.provider || '—'}</dd>
                <dt>计算机</dt>
                <dd>{entry.computer || '—'}</dd>
                <dt>用户</dt>
                <dd>{entry.user_id || '—'}</dd>
                <dt>文档 ID</dt>
                <dd>{entry.doc_id}</dd>
                <dt>日志 ID</dt>
                <dd class="mono-sm" style={{ 'word-break': 'break-all' }}>
                  {entry.id || '—'}
                </dd>
                <dt>标签</dt>
                <dd>
                  <Show
                    when={(entry.tags?.length ?? 0) > 0}
                    fallback={<span class="faint">—</span>}
                  >
                    <div class="row wrap" style={{ gap: '4px' }}>
                      <For each={entry.tags ?? []}>
                        {(tag) => <span class="tag">{tag}</span>}
                      </For>
                    </div>
                  </Show>
                </dd>
                <dt>风险评分</dt>
                <dd>
                  <div class="row wrap" style={{ gap: '5px' }}>
                    <Show
                      when={entry.risk_score != null}
                      fallback={<span class="faint">未评分</span>}
                    >
                      <Badge tone={riskTone(entry.risk_score)}>
                        风险 {entry.risk_score}
                      </Badge>
                    </Show>
                    <Show when={entry.is_anomaly}>
                      <Badge tone="ember">异常</Badge>
                    </Show>
                    <Show when={entry.is_evidence}>
                      <Badge tone="mint">证据</Badge>
                    </Show>
                  </div>
                </dd>
              </dl>
            </div>

            <div>
              <div class="section-label">字段</div>
              <Show
                when={Object.keys(entry.fields ?? {}).length > 0}
                fallback={<div class="mono-sm faint">无结构化字段。</div>}
              >
                <div class="scrollable" style={{ 'max-height': '320px', 'overflow': 'auto' }}>
                  <table class="data-table">
                    <thead>
                      <tr>
                        <th style={{ width: '34%' }}>字段</th>
                        <th>值</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={Object.entries(entry.fields ?? {})}>
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
                            <td>
                              <FieldValue value={value} />
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </div>

            <div>
              <div class="section-label">原始日志</div>
              <CodeBlock text={entry.raw_log || entry.message || '(空日志)'} maxHeight="280px" />
            </div>

            <div>
              <div class="section-label">证据与调查工单</div>
              <Show when={entry.is_evidence}>
                <div class="success-note" style={{ 'margin-bottom': '8px' }}>
                  该日志已标记为证据。
                </div>
              </Show>

              <PageBusy visible={investigationsLoading()} text="加载调查工单…" />

              <Show when={investigationsError()}>
                <div class="danger-note" style={{ 'margin-bottom': '8px' }}>
                  <div class="row between" style={{ gap: '8px' }}>
                    <span class="grow">{investigationsError()}</span>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => void loadInvestigations()}
                    >
                      重试
                    </Button>
                  </div>
                </div>
              </Show>

              <Show
                when={!investigationsLoading() && !investigationsError() && investigations().length === 0}
              >
                <div class="detail-note" style={{ 'margin-bottom': '8px' }}>
                  当前项目还没有调查工单，可在下方新建并自动关联此日志。
                </div>
              </Show>

              <div class="row wrap" style={{ gap: '6px', 'margin-bottom': '7px' }}>
                <select
                  class="select"
                  style={{
                    flex: '1 1 230px',
                    'min-width': '180px',
                    'min-height': '28px',
                    padding: '3px 24px 3px 8px',
                    'font-size': '10.5px',
                  }}
                  value={selectedInvestigation()}
                  aria-label="选择调查工单"
                  onChange={(event) => setSelectedInvestigation(event.currentTarget.value)}
                >
                  <option value="">选择调查工单…</option>
                  <For each={investigations()}>
                    {(investigation) => (
                      <option value={investigation.id}>
                        {investigation.title} · {investigation.status}
                      </option>
                    )}
                  </For>
                </select>
                <input
                  class="input"
                  style={{ flex: '1 1 160px', 'min-width': '140px', 'min-height': '28px', padding: '3px 8px' }}
                  value={evidenceNote()}
                  placeholder="证据备注（可选）"
                  aria-label="证据备注"
                  onInput={(event) => setEvidenceNote(event.currentTarget.value)}
                />
              </div>

              <div class="row wrap" style={{ gap: '6px' }}>
                <Button
                  size="sm"
                  variant="primary"
                  loading={markingEvidence()}
                  disabled={!selectedInvestigation()}
                  onClick={() => void markAsEvidence()}
                >
                  <Icon name="shield-check" size={12} />
                  标记为证据
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setShowNewInvestigation((value) => !value)}
                >
                  {showNewInvestigation() ? '取消新建' : '新建调查工单'}
                </Button>
              </div>

              <Show when={showNewInvestigation()}>
                <div
                  class="panel"
                  style={{ padding: '10px', 'margin-top': '8px', 'box-shadow': 'none' }}
                >
                  <div class="form-stack" style={{ gap: '9px' }}>
                    <Field label="工单标题">
                      <TextInput
                        value={newTitle()}
                        placeholder="例如：可疑登录事件调查"
                        onInput={setNewTitle}
                      />
                    </Field>
                    <Field label="描述" hint="默认使用当前日志消息">
                      <TextArea
                        value={newDescription()}
                        rows={3}
                        onInput={setNewDescription}
                      />
                    </Field>
                    <div class="row between" style={{ gap: '8px' }}>
                      <span class="mono-sm faint">关联日志 doc #{entry.doc_id}</span>
                      <div class="row" style={{ gap: '6px' }}>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setShowNewInvestigation(false)}
                        >
                          取消
                        </Button>
                        <Button
                          size="sm"
                          variant="primary"
                          loading={creatingInvestigation()}
                          onClick={() => void createInvestigation()}
                        >
                          创建并关联
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              </Show>
            </div>

            <div>
              <div class="section-label">AI 辅助</div>
              <div class="row between" style={{ gap: '8px', 'flex-wrap': 'wrap' }}>
                <span class="mono-sm faint" style={{ 'line-height': '1.6' }}>
                  使用异常检测接口分析该日志时间点附近 1 条日志。
                </span>
                <div class="row" style={{ gap: '6px' }}>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={aiLoading()}
                    onClick={() => void explainWithAI()}
                  >
                    <Icon name="wand" size={12} />
                    AI 解释该日志
                  </Button>
                  <Button
                    size="sm"
                    variant="primary"
                    onClick={() => {
                      openLogDetailWithAI(entry.doc_id)
                      props.onClose()
                    }}
                  >
                    <Icon name="wand" size={12} />
                    AI 分析
                  </Button>
                </div>
              </div>

              <Show when={aiError()}>
                <div class="detail-note" style={{ 'margin-top': '8px' }}>
                  {aiError()}
                </div>
              </Show>

              <Show when={aiResult()}>
                {(resultAccessor) => {
                  const result = resultAccessor()
                  return (
                    <div class="stack" style={{ gap: '8px', 'margin-top': '8px' }}>
                      <div class="row wrap" style={{ gap: '6px' }}>
                        <Badge tone={severityTone(result.risk_level)}>
                          风险 {result.risk_level || '未知'}
                        </Badge>
                        <Badge tone="muted">{result.provider || 'AI'}</Badge>
                        <span class="mono-sm faint">
                          已分析 {result.analyzed} 条 · {formatTime(result.generated_at)}
                        </span>
                      </div>
                      <div class="success-note" style={{ 'line-height': '1.7' }}>
                        {result.summary || 'AI 未返回摘要。'}
                      </div>
                      <div>
                        <div class="mono-sm faint" style={{ 'margin-bottom': '3px' }}>
                          发现
                        </div>
                        <BulletList items={result.findings ?? []} empty="未返回发现项。" />
                      </div>
                      <div>
                        <div class="mono-sm faint" style={{ 'margin-bottom': '3px' }}>
                          建议
                        </div>
                        <BulletList items={result.suggestions ?? []} empty="未返回建议。" />
                      </div>
                    </div>
                  )
                }}
              </Show>
            </div>
          </div>
        )}
      </Show>
    </Drawer>
  )
}
