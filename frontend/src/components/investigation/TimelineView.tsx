import { For, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { TimelineEvent } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatTime, parseTime, severityTone } from '../../utils'
import { Badge, Button, EmptyState, Icon, IconButton, PageBusy } from '../ui'

export interface TimelineViewProps {
  investigationId: string
  reloadKey?: number
  onOpenLog?: (docID: number) => void
}

/** 解析时间线事件中的日志 Doc ID（十进制字符串），忽略非数字与非法值。 */
function parseDocIds(event: TimelineEvent): number[] {
  const ids: number[] = []
  for (const raw of event.log_entry_ids ?? []) {
    const text = String(raw ?? '').trim()
    if (!/^\d+$/.test(text)) continue
    const id = Number(text)
    if (!Number.isSafeInteger(id) || id < 0) continue
    if (!ids.includes(id)) ids.push(id)
  }
  return ids
}

export function TimelineView(props: TimelineViewProps) {
  const [events, setEvents] = createSignal<TimelineEvent[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  let loadSeq = 0

  const sorted = createMemo(() =>
    events()
      .slice()
      .sort((a, b) => {
        const left = parseTime(a.timestamp)?.getTime() ?? 0
        const right = parseTime(b.timestamp)?.getTime() ?? 0
        return left - right
      }),
  )

  const loadTimeline = async () => {
    const invID = props.investigationId
    if (!invID) {
      setEvents([])
      return
    }
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.investigations.timeline(invID)
      if (seq !== loadSeq) return
      setEvents(Array.isArray(list) ? list : [])
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载时间线失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const invID = props.investigationId
    void props.reloadKey
    if (!invID) {
      setEvents([])
      return
    }
    void loadTimeline()
  })

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          调查时间线
          <Badge tone="muted">{sorted().length}</Badge>
        </div>
        <div class="row">
          <span class="mono-sm faint">按时间升序</span>
          <IconButton
            icon="refresh"
            title="刷新时间线"
            disabled={loading()}
            onClick={() => void loadTimeline()}
          />
        </div>
      </div>

      <Show when={loadError()}>
        <div class="panel-body" style={{ 'padding-bottom': '10px' }}>
          <div class="danger-note">
            <div class="row between wrap">
              <span>{loadError()}</span>
              <Button size="sm" variant="ghost" onClick={() => void loadTimeline()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={loading()}>
        <div class="panel-body" style={{ 'padding-top': '4px', 'padding-bottom': '4px' }}>
          <PageBusy visible text="构建时间线…" />
        </div>
      </Show>

      <Show when={!loading() && !loadError()}>
        <Show
          when={sorted().length > 0}
          fallback={
            <EmptyState
              icon="clock"
              title="暂无时间线事件"
              text="AI 分析或关联日志后将在这里按时间顺序呈现调查进展。"
            />
          }
        >
          <div class="panel-body">
            <ol style={{ 'list-style': 'none', margin: 0, padding: 0 }}>
              <For each={sorted()}>
                {(event, index) => (
                  <li
                    style={{
                      position: 'relative',
                      'padding-left': '26px',
                      'padding-bottom': index() === sorted().length - 1 ? '0' : '16px',
                    }}
                  >
                    <span
                      style={{
                        position: 'absolute',
                        left: '4px',
                        top: '4px',
                        width: '9px',
                        height: '9px',
                        'border-radius': '50%',
                        background:
                          severityTone(event.severity) === 'ember'
                            ? 'var(--ember)'
                            : severityTone(event.severity) === 'amber'
                              ? 'var(--amber)'
                              : severityTone(event.severity) === 'sky'
                                ? 'var(--sky)'
                                : 'var(--accent)',
                        'box-shadow': '0 0 0 3px var(--accent-ghost)',
                      }}
                    />
                    <Show when={index() < sorted().length - 1}>
                      <span
                        style={{
                          position: 'absolute',
                          left: '8px',
                          top: '18px',
                          bottom: '0',
                          width: '1px',
                          background: 'var(--border)',
                        }}
                      />
                    </Show>
                    <div class="row wrap" style={{ gap: '7px' }}>
                      <span class="mono-sm faint">{formatTime(event.timestamp)}</span>
                      <Badge tone={severityTone(event.severity)}>
                        {event.severity || 'info'}
                      </Badge>
                      <Show when={event.source}>
                        <span class="tag">{event.source}</span>
                      </Show>
                    </div>
                    <div
                      style={{
                        'margin-top': '3px',
                        'font-size': '12px',
                        'line-height': '1.6',
                        color: 'var(--text)',
                      }}
                    >
                      {event.event || '未命名事件'}
                    </div>
                    <Show when={event.log_entry_ids?.length}>
                      <div class="row wrap" style={{ 'margin-top': '5px', gap: '4px' }}>
                        <span class="mono-sm faint">
                          {event.log_entry_ids.length} 条关联日志
                        </span>
                        <Show when={props.onOpenLog}>
                          {(open) => (
                            <For each={parseDocIds(event)}>
                              {(docId) => (
                                <button
                                  type="button"
                                  class="chip"
                                  style={{ padding: '1px 7px', 'font-size': '10px' }}
                                  title={`查看 Doc #${docId} 日志详情`}
                                  onClick={() => open()(docId)}
                                >
                                  <Icon name="file" size={9} />
                                  Doc #{docId}
                                </button>
                              )}
                            </For>
                          )}
                        </Show>
                      </div>
                    </Show>
                  </li>
                )}
              </For>
            </ol>
          </div>
        </Show>
      </Show>
    </div>
  )
}
