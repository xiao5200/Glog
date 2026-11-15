import { For, Show, createEffect, createSignal } from 'solid-js'
import type { EvidenceItem, LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatTime, formatTimeShort, friendlyLogMessage, truncate } from '../../utils'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Icon,
  IconButton,
  LevelBadge,
  PageBusy,
  TextInput,
} from '../ui'
import { LogEntryDrawer } from './LogEntryDrawer'

export interface EvidenceListProps {
  investigationId: string
  projectId: string
  reloadKey?: number
  onChanged?: () => void
}

export function EvidenceList(props: EvidenceListProps) {
  const [evidence, setEvidence] = createSignal<EvidenceItem[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')

  const [drawerOpen, setDrawerOpen] = createSignal(false)
  const [drawerEntry, setDrawerEntry] = createSignal<LogEntry | null>(null)
  const [drawerLoading, setDrawerLoading] = createSignal(false)
  const [drawerError, setDrawerError] = createSignal('')
  const [drawerNote, setDrawerNote] = createSignal('')

  const [unmarkTarget, setUnmarkTarget] = createSignal<EvidenceItem | null>(null)
  const [unmarking, setUnmarking] = createSignal(false)

  const [addDocId, setAddDocId] = createSignal('')
  const [addNote, setAddNote] = createSignal('')
  const [adding, setAdding] = createSignal(false)
  const [addError, setAddError] = createSignal('')

  let loadSeq = 0
  let drawerSeq = 0

  const loadEvidence = async () => {
    const invID = props.investigationId
    if (!invID) {
      setEvidence([])
      return
    }
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.investigations.evidence(invID)
      if (seq !== loadSeq) return
      setEvidence(Array.isArray(list) ? list : [])
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载证据失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const invID = props.investigationId
    void props.projectId
    void props.reloadKey
    if (!invID) {
      setEvidence([])
      return
    }
    void loadEvidence()
  })

  // 供 Drawer 重试时复用当前选中证据；证据条目不保存副本，按 Doc ID 重新读取。
  let lastDocId = 0

  const reloadDrawer = async (docId: number) => {
    const seq = ++drawerSeq
    setDrawerLoading(true)
    setDrawerError('')
    try {
      const entry = await api.query.getEntry(props.projectId, docId)
      if (seq !== drawerSeq) return
      setDrawerEntry(entry)
    } catch (err) {
      if (seq !== drawerSeq) return
      setDrawerError(errorText(err))
    } finally {
      if (seq === drawerSeq) setDrawerLoading(false)
    }
  }

  const openLog = async (item: EvidenceItem) => {
    const seq = ++drawerSeq
    lastDocId = item.mark.doc_id
    setDrawerOpen(true)
    setDrawerNote(item.mark?.note ? `证据备注：${item.mark.note}` : '')
    setDrawerError('')
    if (item.entry) {
      setDrawerEntry(item.entry)
      setDrawerLoading(false)
      return
    }
    setDrawerEntry(null)
    setDrawerLoading(true)
    try {
      const entry = await api.query.getEntry(props.projectId, item.mark.doc_id)
      if (seq !== drawerSeq) return
      setDrawerEntry(entry)
    } catch (err) {
      if (seq !== drawerSeq) return
      setDrawerError(errorText(err))
    } finally {
      if (seq === drawerSeq) setDrawerLoading(false)
    }
  }

  const openLogWithId = async (item: EvidenceItem) => {
    lastDocId = item.mark.doc_id
    await openLog(item)
  }

  const addEvidence = async () => {
    const invID = props.investigationId
    if (!invID) {
      setAddError('当前工单无效，无法添加证据。')
      return
    }
    const raw = addDocId().trim()
    if (!/^\d+$/.test(raw)) {
      setAddError('请输入有效的 DocID（非负整数）。')
      return
    }
    const docID = Number(raw)
    if (!Number.isSafeInteger(docID) || docID < 0) {
      setAddError('DocID 超出有效范围。')
      return
    }
    setAdding(true)
    setAddError('')
    try {
      await api.investigations.markEvidence(invID, docID, addNote().trim() || '按 DocID 添加')
      notify('success', '已添加证据', `Doc #${docID} 已归入当前工单`)
      setAddDocId('')
      setAddNote('')
      await loadEvidence()
      props.onChanged?.()
    } catch (err) {
      const message = errorText(err)
      setAddError(message)
      notify('error', '添加证据失败', message)
    } finally {
      setAdding(false)
    }
  }

  const confirmUnmark = async () => {
    const item = unmarkTarget()
    if (!item) return
    setUnmarking(true)
    try {
      await api.investigations.unmarkEvidence(props.investigationId, item.mark.doc_id)
      notify('success', '已取消证据标记', `Doc #${item.mark.doc_id}`)
      setUnmarkTarget(null)
      await loadEvidence()
      props.onChanged?.()
    } catch (err) {
      notify('error', '取消证据标记失败', errorText(err))
    } finally {
      setUnmarking(false)
    }
  }

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          证据链
          <Badge tone="muted">{evidence().length}</Badge>
        </div>
        <div class="row">
          <span class="mono-sm faint">点击证据查看完整日志</span>
          <IconButton
            icon="refresh"
            title="刷新证据"
            disabled={loading()}
            onClick={() => void loadEvidence()}
          />
        </div>
      </div>

      <Show when={loadError()}>
        <div class="panel-body" style={{ 'padding-bottom': '10px' }}>
          <div class="danger-note">
            <div class="row between wrap">
              <span>{loadError()}</span>
              <Button size="sm" variant="ghost" onClick={() => void loadEvidence()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={loading()}>
        <div class="panel-body" style={{ 'padding-top': '4px', 'padding-bottom': '4px' }}>
          <PageBusy visible text="加载证据链…" />
        </div>
      </Show>

      <div class="panel-body" style={{ 'padding-top': '2px', 'padding-bottom': '12px' }}>
        <div class="section-label" style={{ 'margin-bottom': '6px' }}>
          按 DocID 添加证据
        </div>
        <div class="row wrap" style={{ 'align-items': 'flex-end', gap: '8px' }}>
          <div style={{ width: '160px' }}>
            <div class="label">DocID</div>
            <TextInput
              type="number"
              value={addDocId()}
              placeholder="例如 1024"
              disabled={adding()}
              onInput={(value) => {
                setAddDocId(value)
                if (addError()) setAddError('')
              }}
              onEnter={() => void addEvidence()}
            />
          </div>
          <div class="grow" style={{ 'min-width': '200px' }}>
            <div class="label">备注（可选）</div>
            <TextInput
              value={addNote()}
              placeholder="默认：按 DocID 添加"
              disabled={adding()}
              onInput={(value) => {
                setAddNote(value)
                if (addError()) setAddError('')
              }}
              onEnter={() => void addEvidence()}
            />
          </div>
          <Button
            size="sm"
            loading={adding()}
            disabled={!addDocId().trim()}
            onClick={() => void addEvidence()}
          >
            <Icon name="plus" size={12} />
            添加证据
          </Button>
        </div>
        <div class="row between wrap" style={{ 'margin-top': '6px', gap: '6px' }}>
          <span class="mono-sm faint">重复标记同一 DocID 为幂等操作，不会产生重复证据。</span>
          <Show when={addError()}>
            <span class="mono-sm ember">{addError()}</span>
          </Show>
        </div>
      </div>

      <Show when={!loading() && !loadError()}>
        <Show
          when={evidence().length > 0}
          fallback={
            <EmptyState
              icon="layers"
              title="暂无证据"
              text="在日志分析页对关键日志执行“标记为证据”，证据将自动归入当前工单。"
            />
          }
        >
          <div class="scrollable" style={{ 'max-height': '390px' }}>
            <table class="data-table">
              <thead>
                <tr>
                  <th style={{ width: '142px' }}>时间</th>
                  <th style={{ width: '78px' }}>级别</th>
                  <th style={{ width: '110px' }}>主机</th>
                  <th>消息摘要</th>
                  <th style={{ width: '120px' }}>备注</th>
                  <th style={{ width: '112px' }}>标记时间</th>
                  <th style={{ width: '48px' }} />
                </tr>
              </thead>
              <tbody>
                <For each={evidence()}>
                  {(item) => (
                    <tr
                      style={{ cursor: 'pointer' }}
                      onClick={() => void openLogWithId(item)}
                      title="查看日志详情"
                    >
                      <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                        {formatTime(item.entry?.timestamp)}
                      </td>
                      <td>
                        <Show when={item.entry} fallback={<span class="faint">—</span>}>
                          <LevelBadge level={item.entry?.level} />
                        </Show>
                      </td>
                      <td class="mono-sm truncate" title={item.entry?.source_host}>
                        {item.entry?.source_host || item.entry?.computer || '—'}
                      </td>
                      <td>
                        <div
                          class="truncate"
                          style={{ 'max-width': '460px' }}
                          title={item.entry?.message}
                        >
                          {item.entry?.message
                            ? truncate(friendlyLogMessage(item.entry.message), 180)
                            : '点击加载日志详情'}
                        </div>
                      </td>
                      <td class="mono-sm truncate" title={item.mark?.note}>
                        {item.mark?.note || '—'}
                      </td>
                      <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                        {formatTimeShort(item.mark?.marked_at)}
                      </td>
                      <td>
                        <IconButton
                          icon="trash"
                          title="取消证据标记"
                          onClick={(e) => {
                            e.stopPropagation()
                            setUnmarkTarget(item)
                          }}
                        />
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </Show>

      <LogEntryDrawer
        open={drawerOpen()}
        entry={drawerEntry()}
        loading={drawerLoading()}
        error={drawerError()}
        note={drawerNote()}
        title="证据日志详情"
        onClose={() => {
          drawerSeq++
          setDrawerOpen(false)
        }}
        onRetry={() => {
          if (lastDocId) void reloadDrawer(lastDocId)
        }}
      />

      <ConfirmDialog
        open={Boolean(unmarkTarget())}
        danger
        title="取消证据标记"
        message={`确定取消 Doc #${unmarkTarget()?.mark.doc_id ?? '—'} 的证据标记？日志本身不会被删除。`}
        confirmText="取消标记"
        loading={unmarking()}
        onConfirm={() => void confirmUnmark()}
        onCancel={() => {
          if (!unmarking()) setUnmarkTarget(null)
        }}
      />
    </div>
  )
}
