import { For, Show, createMemo, createSignal, onMount } from 'solid-js'
import type { AIProviderInfo, Investigation, LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { copyText, formatRange, formatTime, riskTone, shortId } from '../../utils'
import { Badge, Button, ConfirmDialog, Icon, PageBusy, Select } from '../ui'
import { AIAnalysisPanel } from './AIAnalysisPanel'
import { EditInvestigationModal } from './EditInvestigationModal'
import { EvidenceList } from './EvidenceList'
import { LogEntryDrawer } from './LogEntryDrawer'
import {
  clueTypeLabel,
  investigationStatusLabel,
  investigationStatusTone,
} from './meta'
import { RelatedLogsPanel } from './RelatedLogsPanel'
import { TimelineView } from './TimelineView'

export interface InvestigationDetailProps {
  investigation: Investigation
  projectId: string
  onChanged: (investigation: Investigation) => void
  onDeleted?: (id: string) => void
}

export function InvestigationDetail(props: InvestigationDetailProps) {
  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [providersLoading, setProvidersLoading] = createSignal(false)
  const [providerError, setProviderError] = createSignal('')
  const [provider, setProvider] = createSignal('')

  const [analyzing, setAnalyzing] = createSignal(false)
  const [analyzeError, setAnalyzeError] = createSignal('')

  const [closeOpen, setCloseOpen] = createSignal(false)
  const [closing, setClosing] = createSignal(false)

  const [deleteOpen, setDeleteOpen] = createSignal(false)
  const [deleting, setDeleting] = createSignal(false)

  const [exporting, setExporting] = createSignal(false)
  const [exportError, setExportError] = createSignal('')
  const [exportedPath, setExportedPath] = createSignal('')

  const [editOpen, setEditOpen] = createSignal(false)

  const [logOpen, setLogOpen] = createSignal(false)
  const [logEntry, setLogEntry] = createSignal<LogEntry | null>(null)
  const [logLoading, setLogLoading] = createSignal(false)
  const [logError, setLogError] = createSignal('')
  const [logNote, setLogNote] = createSignal('')
  let logSeq = 0
  let lastLogDocId = -1

  const [relatedReloadKey, setRelatedReloadKey] = createSignal(0)
  const [evidenceReloadKey, setEvidenceReloadKey] = createSignal(0)
  const [timelineReloadKey, setTimelineReloadKey] = createSignal(0)
  const bumpRelatedReload = () => setRelatedReloadKey((value) => value + 1)
  const bumpEvidenceReload = () => setEvidenceReloadKey((value) => value + 1)
  const bumpTimelineReload = () => setTimelineReloadKey((value) => value + 1)

  const loadProviders = async () => {
    setProvidersLoading(true)
    setProviderError('')
    try {
      const list = await api.ai.providers()
      const safe = Array.isArray(list) ? list : []
      setProviders(safe)
      const configured = safe.filter((item) => item.configured)
      const currentValid = configured.some((item) => item.provider === provider())
      if (!currentValid) {
        const preferred = configured.find((item) => item.enabled) ?? configured[0]
        setProvider(preferred?.provider ?? '')
      }
    } catch (err) {
      const message = errorText(err)
      setProviderError(message)
      notify('error', '加载 AI 服务商失败', message)
    } finally {
      setProvidersLoading(false)
    }
  }

  onMount(() => {
    void loadProviders()
  })

  // 只暴露已配置完成的 AI 服务商，避免把未配置的服务商发送给后端。
  const configuredProviders = createMemo(() => providers().filter((item) => item.configured))

  const providerOptions = createMemo(() =>
    configuredProviders().map((item) => ({
      value: item.provider,
      label: `${item.name || item.provider}${item.model ? ` · ${item.model}` : ''}`,
    })),
  )

  const analyze = async () => {
    const selected = provider()
    const selectedInfo = configuredProviders().find((item) => item.provider === selected)
    if (!selected || !selectedInfo) {
      notify(
        'warn',
        '暂无可用的 AI 服务商',
        '请先在“设置 → AI 服务商”中完成服务商配置并填写 API Key。',
      )
      return
    }
    setAnalyzing(true)
    setAnalyzeError('')
    try {
      await api.investigations.analyze(props.investigation.id, selected)
      const fresh = await api.investigations.get(props.investigation.id)
      props.onChanged(fresh)
      const risk = fresh.ai_analysis?.risk_level
      notify('success', 'AI 分析完成', risk ? `研判风险等级：${risk}` : '工单研判结果已更新')
    } catch (err) {
      const message = errorText(err)
      setAnalyzeError(message)
      notify('error', 'AI 分析失败', message)
    } finally {
      setAnalyzing(false)
    }
  }

  const confirmClose = async () => {
    setClosing(true)
    try {
      await api.investigations.close(props.investigation.id)
      const fresh = await api.investigations.get(props.investigation.id)
      props.onChanged(fresh)
      setCloseOpen(false)
      notify('success', '工单已结案', fresh.title || props.investigation.title)
    } catch (err) {
      notify('error', '结案失败', errorText(err))
    } finally {
      setClosing(false)
    }
  }

  const exportEvidence = async () => {
    try {
      const path = await api.system.selectSaveFile(
        `evidence-${props.investigation.id}.zip`,
        '证据包',
        '*.zip',
      )
      if (!path) return
      setExporting(true)
      setExportError('')
      setExportedPath('')
      const output = await api.investigations.exportEvidence(props.investigation.id, path)
      setExportedPath(output)
      notify('success', '证据包已导出', output)
    } catch (err) {
      const message = errorText(err)
      setExportError(message)
      notify('error', '导出证据包失败', message)
    } finally {
      setExporting(false)
    }
  }

  const copyExportedPath = async () => {
    const path = exportedPath()
    if (!path) return
    try {
      await copyText(path)
      notify('success', '路径已复制', path)
    } catch (err) {
      notify('error', '复制路径失败', errorText(err))
    }
  }

  const refreshInvestigation = async () => {
    try {
      const fresh = await api.investigations.get(props.investigation.id)
      props.onChanged(fresh)
    } catch (err) {
      notify('error', '刷新工单失败', errorText(err))
    }
  }

  const openLog = async (docID: number, note?: string) => {
    if (!Number.isSafeInteger(docID) || docID < 0) {
      notify('warn', 'DocID 无效', '无法打开该条关联日志，请刷新后重试。')
      return
    }
    if (!props.projectId) {
      notify('error', '无法打开日志', '缺少项目上下文，请重新进入该工单。')
      return
    }
    const seq = ++logSeq
    lastLogDocId = docID
    setLogOpen(true)
    setLogEntry(null)
    setLogLoading(true)
    setLogError('')
    setLogNote(note ?? '')
    try {
      const entry = await api.query.getEntry(props.projectId, docID)
      if (seq !== logSeq) return
      setLogEntry(entry)
    } catch (err) {
      if (seq !== logSeq) return
      const message = errorText(err)
      setLogError(message)
      notify('error', '加载关联日志失败', message)
    } finally {
      if (seq === logSeq) setLogLoading(false)
    }
  }

  const closeLog = () => {
    logSeq++
    setLogOpen(false)
  }

  // EvidenceList 自己会重载证据；此处只需刷新关联日志、时间线与工单元数据。
  const handleEvidenceChanged = () => {
    bumpRelatedReload()
    bumpTimelineReload()
    void refreshInvestigation()
  }

  // 关联日志面板标记证据后，证据链与时间线也需要同步刷新。
  const handleRelatedChanged = () => {
    bumpRelatedReload()
    bumpEvidenceReload()
    bumpTimelineReload()
    void refreshInvestigation()
  }

  // 编辑线索类型/值或时间范围后，关联日志与时间线的检索条件会变化。
  const handleInvestigationUpdated = (fresh: Investigation) => {
    props.onChanged(fresh)
    bumpRelatedReload()
    bumpTimelineReload()
  }

  const confirmDelete = async () => {
    setDeleting(true)
    try {
      await api.investigations.remove(props.investigation.id)
      setDeleteOpen(false)
      if (props.onDeleted) {
        // 页面负责移除列表项、选择下一工单并提示成功。
        props.onDeleted(props.investigation.id)
      } else {
        notify('success', '工单已删除', props.investigation.title || props.investigation.id)
      }
    } catch (err) {
      notify('error', '删除工单失败', errorText(err))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div class="stack">
      <div class="panel">
        <div class="panel-head">
          <div class="panel-title">
            <span class="tick" />
            工单详情
            <Badge tone={investigationStatusTone(props.investigation.status)}>
              {investigationStatusLabel(props.investigation.status)}
            </Badge>
          </div>
          <span class="mono-sm faint">ID {shortId(props.investigation.id, 14)}</span>
        </div>

        <div class="panel-body stack">
          <div>
            <div
              style={{
                'font-family': 'var(--font-display)',
                'font-size': '23px',
                'font-weight': 600,
                'letter-spacing': '0.02em',
                'line-height': '1.2',
                color: 'var(--text)',
              }}
            >
              {props.investigation.title || '未命名工单'}
            </div>

            <div class="row wrap" style={{ 'margin-top': '9px', gap: '6px' }}>
              <Badge tone={riskTone(props.investigation.risk_score)}>
                风险 {props.investigation.risk_score != null ? props.investigation.risk_score : '—'}
              </Badge>
              <span class="mono-sm faint">
                证据 {props.investigation.evidence_ids?.length ?? 0} 件
              </span>
              <Show when={props.investigation.assigned_to}>
                <span class="mono-sm faint">处理人 {props.investigation.assigned_to}</span>
              </Show>
              <For each={props.investigation.tags ?? []}>
                {(tag) => <span class="tag">#{tag}</span>}
              </For>
            </div>
          </div>

          <p class="muted" style={{ 'font-size': '12px', 'line-height': '1.7' }}>
            {props.investigation.description || '暂无工单描述。'}
          </p>

          <dl class="kv">
            <dt>线索</dt>
            <dd>
              {clueTypeLabel(props.investigation.clue_type)} ·{' '}
              <span class="mono-sm" style={{ color: 'var(--text)' }}>
                {props.investigation.clue_value || '未填写'}
              </span>
            </dd>
            <dt>时间范围</dt>
            <dd>
              {formatRange(
                props.investigation.time_range?.start,
                props.investigation.time_range?.end,
              )}
            </dd>
            <dt>处理人</dt>
            <dd>{props.investigation.assigned_to || '未指派'}</dd>
            <dt>创建时间</dt>
            <dd>{formatTime(props.investigation.created_at)}</dd>
            <dt>更新时间</dt>
            <dd>{formatTime(props.investigation.updated_at)}</dd>
            <Show when={props.investigation.closed_at}>
              <dt>结案时间</dt>
              <dd>{formatTime(props.investigation.closed_at)}</dd>
            </Show>
            <dt>工单编号</dt>
            <dd style={{ 'word-break': 'break-all' }}>{props.investigation.id}</dd>
          </dl>

          <div class="divider" style={{ margin: '2px 0' }} />

          <div class="row between wrap" style={{ 'align-items': 'flex-end', gap: '10px' }}>
            <div class="row wrap" style={{ 'align-items': 'flex-end', gap: '8px' }}>
              <div style={{ width: '240px' }}>
                <div class="label">AI 服务商</div>
                <Select
                  value={provider()}
                  options={providerOptions()}
                  disabled={
                    providersLoading() || analyzing() || configuredProviders().length === 0
                  }
                  onInput={setProvider}
                />
              </div>
              <Button
                variant="primary"
                size="sm"
                loading={analyzing()}
                disabled={!provider() || configuredProviders().length === 0}
                onClick={() => void analyze()}
              >
                <Icon name="wand" size={12} />
                AI 分析
              </Button>
              <Button size="sm" onClick={() => setEditOpen(true)}>
                <Icon name="save" size={12} />
                编辑
              </Button>
              <Button
                size="sm"
                loading={exporting()}
                onClick={() => void exportEvidence()}
              >
                <Icon name="download" size={12} />
                导出证据包
              </Button>
              <Show when={String(props.investigation.status).toLowerCase() !== 'closed'}>
                <Button size="sm" variant="danger" onClick={() => setCloseOpen(true)}>
                  <Icon name="lock" size={12} />
                  结案
                </Button>
              </Show>
              <Button size="sm" variant="danger" onClick={() => setDeleteOpen(true)}>
                <Icon name="trash" size={12} />
                删除工单
              </Button>
            </div>

            <div class="row wrap" style={{ gap: '6px' }}>
              <Show when={providersLoading()}>
                <span class="mono-sm faint">加载服务商…</span>
              </Show>
              <Show when={!providersLoading() && providerError()}>
                <span class="mono-sm ember" title={providerError()}>
                  服务商加载失败
                </span>
                <Button size="sm" variant="ghost" onClick={() => void loadProviders()}>
                  重试
                </Button>
              </Show>
              <Show
                when={
                  !providersLoading() && !providerError() && configuredProviders().length === 0
                }
              >
                <span class="mono-sm faint">
                  暂无可用的 AI 服务商，请先在“设置 → AI 服务商”中完成配置并填写 API Key。
                </span>
                <Button size="sm" variant="ghost" onClick={() => void loadProviders()}>
                  重试
                </Button>
              </Show>
              <Show
                when={
                  !providersLoading() && !providerError() && configuredProviders().length > 0
                }
              >
                <span class="mono-sm faint">可用服务商 {configuredProviders().length} 个</span>
              </Show>
            </div>
          </div>

          <Show when={analyzeError()}>
            <div class="danger-note">AI 分析失败：{analyzeError()}</div>
          </Show>

          <Show when={analyzing()}>
            <PageBusy visible text="AI 正在分析工单证据，请稍候…" />
          </Show>

          <Show when={exportError()}>
            <div class="danger-note">证据包导出失败：{exportError()}</div>
          </Show>

          <Show when={exportedPath()}>
            <div class="success-note">
              <div class="row between wrap" style={{ gap: '8px' }}>
                <div class="grow" style={{ 'min-width': 0 }}>
                  <div class="section-label" style={{ 'margin-bottom': '3px' }}>
                    证据包路径
                  </div>
                  <div class="mono-sm" style={{ 'word-break': 'break-all' }}>
                    {exportedPath()}
                  </div>
                </div>
                <Button size="sm" variant="ghost" onClick={() => void copyExportedPath()}>
                  <Icon name="copy" size={12} />
                  复制路径
                </Button>
              </div>
            </div>
          </Show>
        </div>
      </div>

      <EvidenceList
        investigationId={props.investigation.id}
        projectId={props.projectId}
        reloadKey={evidenceReloadKey()}
        onChanged={handleEvidenceChanged}
      />

      <RelatedLogsPanel
        investigationId={props.investigation.id}
        projectId={props.projectId}
        evidenceIds={props.investigation.evidence_ids ?? []}
        reloadKey={relatedReloadKey()}
        onOpenLog={(docID) => void openLog(docID)}
        onChanged={handleRelatedChanged}
      />

      <div
        style={{
          display: 'grid',
          'grid-template-columns': 'repeat(auto-fit, minmax(430px, 1fr))',
          gap: '14px',
          'align-items': 'start',
        }}
      >
        <TimelineView
          investigationId={props.investigation.id}
          reloadKey={timelineReloadKey()}
          onOpenLog={(docID) => void openLog(docID)}
        />
        <Show when={props.investigation.ai_analysis}>
          {(analysis) => (
            <AIAnalysisPanel
              analysis={analysis()}
              onOpenLog={(docID) => void openLog(docID)}
            />
          )}
        </Show>
      </div>

      <EditInvestigationModal
        open={editOpen()}
        investigation={props.investigation}
        onClose={() => setEditOpen(false)}
        onChanged={handleInvestigationUpdated}
      />

      <ConfirmDialog
        open={closeOpen()}
        danger
        title="结案确认"
        message={`确定关闭工单“${props.investigation.title || '未命名工单'}”？结案后仍可查看证据、时间线与 AI 研判结果。`}
        confirmText="确认结案"
        loading={closing()}
        onConfirm={() => void confirmClose()}
        onCancel={() => {
          if (!closing()) setCloseOpen(false)
        }}
      />

      <ConfirmDialog
        open={deleteOpen()}
        danger
        title="删除工单确认"
        message={`确定删除工单“${props.investigation.title || '未命名工单'}”？该操作不可撤销，工单中的证据标记与 AI 研判结果将一并删除，但不会删除原始日志。`}
        confirmText="确认删除"
        loading={deleting()}
        onConfirm={() => void confirmDelete()}
        onCancel={() => {
          if (!deleting()) setDeleteOpen(false)
        }}
      />

      <LogEntryDrawer
        open={logOpen()}
        entry={logEntry()}
        loading={logLoading()}
        error={logError()}
        note={logNote()}
        title="关联日志详情"
        onClose={closeLog}
        onRetry={() => {
          if (lastLogDocId >= 0) void openLog(lastLogDocId, logNote())
        }}
      />
    </div>
  )
}
