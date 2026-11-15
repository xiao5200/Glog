import { For, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatTime, friendlyLogMessage, truncate } from '../../utils'
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  IconButton,
  LevelBadge,
  PageBusy,
} from '../ui'

export interface RelatedLogsPanelProps {
  investigationId: string
  projectId: string
  evidenceIds: string[]
  reloadKey?: number
  onOpenLog: (docID: number) => void
  onChanged?: () => void
}

/**
 * 工单关联日志面板：后端优先返回证据日志，其次按线索值检索，最多 200 条。
 * 非证据行可一键标记为证据，标记后通知父组件刷新工单与关联日志。
 */
export function RelatedLogsPanel(props: RelatedLogsPanelProps) {
  const [entries, setEntries] = createSignal<LogEntry[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [markingDoc, setMarkingDoc] = createSignal<number | null>(null)

  let loadSeq = 0
  let lastInvID = ''

  const evidenceSet = createMemo(
    () => new Set((props.evidenceIds ?? []).map((id) => String(id))),
  )

  const isEvidence = (entry: LogEntry) => evidenceSet().has(String(entry.doc_id))

  const loadRelated = async () => {
    const invID = props.investigationId
    if (!invID) {
      loadSeq++
      setEntries([])
      setLoading(false)
      setLoadError('')
      return
    }
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.investigations.relatedLogs(invID, 200)
      if (seq !== loadSeq) return
      setEntries(Array.isArray(list) ? list : [])
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载关联日志失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const invID = props.investigationId
    // 项目或外部刷新信号变化时重新检索，保持与工单上下文一致。
    void props.projectId
    void props.reloadKey
    if (!invID) {
      lastInvID = ''
      loadSeq++
      setEntries([])
      setLoading(false)
      setLoadError('')
      return
    }
    // 切换工单时清空旧数据，避免加载期间展示上一工单的日志。
    if (invID !== lastInvID) {
      lastInvID = invID
      setEntries([])
      setMarkingDoc(null)
    }
    void loadRelated()
  })

  const markAsEvidence = async (entry: LogEntry) => {
    if (markingDoc() != null) return
    setMarkingDoc(entry.doc_id)
    try {
      await api.investigations.markEvidence(props.investigationId, entry.doc_id, '来自关联日志')
      notify('success', '已标记为证据', `Doc #${entry.doc_id}`)
      props.onChanged?.()
      await loadRelated()
    } catch (err) {
      notify('error', '标记证据失败', errorText(err))
    } finally {
      setMarkingDoc(null)
    }
  }

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          关联日志
          <Badge tone="muted">{entries().length}</Badge>
        </div>
        <div class="row wrap" style={{ gap: '8px' }}>
          <span class="mono-sm faint">证据优先，其次按线索值检索；最多 200 条</span>
          <IconButton
            icon="refresh"
            title="刷新关联日志"
            disabled={loading()}
            onClick={() => void loadRelated()}
          />
        </div>
      </div>

      <Show when={loadError()}>
        <div class="panel-body" style={{ 'padding-bottom': '10px' }}>
          <div class="danger-note">
            <div class="row between wrap">
              <span>{loadError()}</span>
              <Button size="sm" variant="ghost" onClick={() => void loadRelated()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={loading() && entries().length === 0}>
        <div class="panel-body" style={{ 'padding-top': '4px', 'padding-bottom': '4px' }}>
          <PageBusy visible text="检索关联日志…" />
        </div>
      </Show>

      <Show when={entries().length > 0}>
        <div class="scrollable" style={{ 'max-height': '360px' }}>
          <table class="data-table">
            <thead>
              <tr>
                <th style={{ width: '150px' }}>时间</th>
                <th style={{ width: '76px' }}>级别</th>
                <th style={{ width: '112px' }}>主机</th>
                <th style={{ width: '76px' }}>事件ID</th>
                <th>消息摘要</th>
                <th style={{ width: '148px' }}>来源</th>
                <th style={{ width: '186px' }}>操作</th>
              </tr>
            </thead>
            <tbody>
              <For each={entries()}>
                {(entry) => (
                  <tr>
                    <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                      {formatTime(entry.timestamp)}
                    </td>
                    <td>
                      <LevelBadge level={entry.level} />
                    </td>
                    <td
                      class="mono-sm truncate"
                      title={entry.source_host || entry.computer || ''}
                    >
                      {entry.source_host || entry.computer || '—'}
                    </td>
                    <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                      {entry.event_id != null ? entry.event_id : '—'}
                    </td>
                    <td>
                      <div
                        class="truncate"
                        style={{ 'max-width': '440px' }}
                        title={entry.message ? friendlyLogMessage(entry.message) : ''}
                      >
                        {entry.message
                          ? truncate(friendlyLogMessage(entry.message), 180)
                          : '—'}
                      </div>
                    </td>
                    <td>
                      <div class="row wrap" style={{ gap: '4px' }}>
                        <Show
                          when={isEvidence(entry)}
                          fallback={<Badge tone="sky">线索命中</Badge>}
                        >
                          <Badge tone="mint">证据</Badge>
                        </Show>
                        <Show when={entry.source}>
                          <span
                            class="mono-sm faint truncate"
                            style={{ 'max-width': '78px' }}
                            title={entry.source}
                          >
                            {entry.source}
                          </span>
                        </Show>
                      </div>
                    </td>
                    <td>
                      <div class="row wrap" style={{ gap: '4px' }}>
                        <Button
                          size="sm"
                          variant="ghost"
                          title="查看日志详情"
                          onClick={() => props.onOpenLog(entry.doc_id)}
                        >
                          <Icon name="eye" size={11} />
                          查看
                        </Button>
                        <Show when={!isEvidence(entry)}>
                          <Button
                            size="sm"
                            title="将该日志标记为工单证据"
                            loading={markingDoc() === entry.doc_id}
                            disabled={markingDoc() != null && markingDoc() !== entry.doc_id}
                            onClick={() => void markAsEvidence(entry)}
                          >
                            <Icon name="plus" size={11} />
                            标记为证据
                          </Button>
                        </Show>
                      </div>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={!loading() && !loadError() && entries().length === 0}>
        <EmptyState
          icon="layers"
          title="暂无关联日志"
          text="暂无可关联日志。可先在证据链中添加证据，或在工单中填写线索值。"
        />
      </Show>
    </div>
  )
}
