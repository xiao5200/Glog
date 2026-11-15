import {
  For,
  JSX,
  Show,
  createEffect,
  createMemo,
  createSignal,
  on,
  onCleanup,
  onMount,
} from 'solid-js'
import { createVirtualizer } from '@tanstack/solid-virtual'
import type { LogEntry } from '../../types'
import { Badge, Button, EmptyState, Icon, LevelBadge } from '../ui'
import {
  fieldLabel,
  formatInt,
  formatTime,
  formatTimeShort,
  friendlyLogMessage,
  truncate,
} from '../../utils'
import { basename, type SortField } from './logCommon'

export interface LogVirtualListProps {
  rows: LogEntry[]
  total: number
  hasMore: boolean
  loading: boolean
  loadingMore: boolean
  error: string | null

  selectedDocId: number | null
  expandedDocIds: ReadonlySet<number>

  sortBy: SortField
  sortOrder: 'asc' | 'desc'
  limit: number
  resetToken: number
  projectLogCount: number

  onSort: (field: SortField) => void
  onOpen: (entry: LogEntry) => void
  onOpenPage: (entry: LogEntry) => void
  onAnalyze?: (entry: LogEntry) => void
  onHighlight: (entry: LogEntry) => void
  onToggleExpand: (entry: LogEntry) => void
  onLoadMore: () => void
  onLimitChange: (limit: number) => void
  onRetry: () => void
  onCollect: () => void
  onClearFilters: () => void
}

const SKELETON_ROWS = Array.from({ length: 9 }, (_, index) => index)

const headButtonStyle: JSX.CSSProperties = {
  background: 'transparent',
  border: 'none',
  padding: '0',
  color: 'inherit',
  font: 'inherit',
  'letter-spacing': 'inherit',
  'text-transform': 'inherit',
  'text-align': 'left',
  cursor: 'pointer',
  display: 'inline-flex',
  'align-items': 'center',
  gap: '4px',
}

const skeletonBarStyle = (width: string, height = '9px'): JSX.CSSProperties => ({
  display: 'block',
  width,
  height,
  background: 'var(--border)',
  animation: 'pulseSoft 1.4s ease-in-out infinite',
})

const MAX_FIELD_ROWS = 100
const FIELD_VALUE_LIMIT = 1600

interface DisplayField {
  key: string
  label: string
  value: unknown
}

const CORE_FIELD_DEFS: Array<{ key: keyof LogEntry; raw: string }> = [
  { key: 'event_id', raw: 'event_id' },
  { key: 'provider', raw: 'provider' },
  { key: 'computer', raw: 'computer' },
  { key: 'user_id', raw: 'user_id' },
]

function collectDisplayFields(entry: LogEntry): DisplayField[] {
  const list: DisplayField[] = []
  const seen = new Set<string>()
  for (const def of CORE_FIELD_DEFS) {
    const value = entry[def.key]
    if (value == null || value === '') continue
    seen.add(def.raw.toLowerCase())
    list.push({ key: def.raw, label: fieldLabel(def.raw), value })
  }
  for (const [key, value] of Object.entries(entry.fields ?? {})) {
    if (seen.has(key.toLowerCase())) continue
    seen.add(key.toLowerCase())
    list.push({ key, label: fieldLabel(key), value })
  }
  return list
}

function fieldValueText(value: unknown): string {
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    const json = JSON.stringify(value)
    return json === undefined ? String(value) : json
  } catch {
    return String(value)
  }
}

function displayFieldValue(value: unknown): string {
  const text = fieldValueText(value)
  return text.length > FIELD_VALUE_LIMIT
    ? `${text.slice(0, FIELD_VALUE_LIMIT)}…`
    : text
}

function isComplexFieldValue(value: unknown): boolean {
  return value != null && typeof value === 'object'
}

/** 展开行内的全字段视图：结构化字段 + 核心识别字段。 */
function AllFieldsBlock(props: { entry: LogEntry }): JSX.Element {
  const fields = createMemo(() => collectDisplayFields(props.entry))
  const shown = createMemo(() => fields().slice(0, MAX_FIELD_ROWS))

  return (
    <div
      style={{
        'grid-column': '1 / -1',
        'min-width': 0,
        'margin-top': '2px',
        'padding-top': '7px',
        'border-top': '1px dashed var(--border)',
        cursor: 'default',
      }}
      onClick={(event) => event.stopPropagation()}
      onDblClick={(event) => event.stopPropagation()}
    >
      <div
        class="row between wrap"
        style={{ gap: '8px', 'margin-bottom': '5px' }}
      >
        <span
          style={{
            'font-size': '10.5px',
            'font-weight': 600,
            color: 'var(--text-muted)',
          }}
        >
          全部字段
        </span>
        <span class="mono-sm faint">
          共 {fields().length} 个字段
          <Show when={fields().length > MAX_FIELD_ROWS}>
            {' '}
            · 仅显示前 {MAX_FIELD_ROWS} 个
          </Show>
        </span>
      </div>
      <Show
        when={shown().length > 0}
        fallback={<div class="mono-sm faint">无结构化字段。</div>}
      >
        <div
          class="scrollable scroll-thin"
          style={{
            'max-height': '220px',
            overflow: 'auto',
            border: '1px solid var(--border)',
            'border-radius': 'var(--r-sm)',
            background: 'var(--surface-soft)',
          }}
        >
          <table class="data-table" style={{ 'font-size': '9.5px' }}>
            <thead>
              <tr>
                <th style={{ width: '28%', padding: '4px 8px', 'font-size': '9px' }}>
                  字段
                </th>
                <th style={{ padding: '4px 8px', 'font-size': '9px' }}>值</th>
              </tr>
            </thead>
            <tbody>
              <For each={shown()}>
                {(field) => (
                  <tr>
                    <td
                      style={{
                        padding: '4px 8px',
                        'word-break': 'break-all',
                        'vertical-align': 'top',
                      }}
                    >
                      <div
                        style={{
                          'font-family': 'var(--font-sans)',
                          'font-size': '10px',
                          color: 'var(--text)',
                        }}
                      >
                        {field.label}
                      </div>
                      <Show when={field.label !== field.key}>
                        <div
                          class="mono-sm faint"
                          style={{ 'font-size': '9px', 'line-height': '1.4' }}
                        >
                          {field.key}
                        </div>
                      </Show>
                    </td>
                    <td
                      style={{
                        padding: '4px 8px',
                        'vertical-align': 'top',
                      }}
                    >
                      <Show
                        when={isComplexFieldValue(field.value)}
                        fallback={
                          <span
                            class="mono-sm"
                            style={{
                              'font-size': '9.5px',
                              'word-break': 'break-all',
                              'line-height': '1.5',
                            }}
                          >
                            {displayFieldValue(field.value)}
                          </span>
                        }
                      >
                        <pre
                          style={{
                            margin: 0,
                            padding: '2px 0',
                            'font-family': 'var(--font-mono)',
                            'font-size': '9.3px',
                            'line-height': '1.45',
                            'white-space': 'pre-wrap',
                            'word-break': 'break-all',
                            color: 'var(--text-muted)',
                          }}
                        >
                          {displayFieldValue(field.value)}
                        </pre>
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
  )
}

export function LogVirtualList(props: LogVirtualListProps): JSX.Element {
  const [scrollEl, setScrollEl] = createSignal<HTMLDivElement | undefined>()
  const [viewportWidth, setViewportWidth] = createSignal(
    typeof window === 'undefined' ? 1440 : window.innerWidth,
  )

  onMount(() => {
    const handleResize = () => setViewportWidth(window.innerWidth)
    window.addEventListener('resize', handleResize)
    onCleanup(() => window.removeEventListener('resize', handleResize))
  })

  // 单击打开详情、双击展开字段；延迟单击以避免双击时先弹出抽屉。
  let rowClickTimer: number | undefined
  const handleRowClick = (entry: LogEntry) => {
    window.clearTimeout(rowClickTimer)
    rowClickTimer = window.setTimeout(() => props.onOpen(entry), 240)
  }
  const handleRowDblClick = (entry: LogEntry, event: MouseEvent) => {
    event.preventDefault()
    window.clearTimeout(rowClickTimer)
    props.onToggleExpand(entry)
  }
  onCleanup(() => window.clearTimeout(rowClickTimer))

  // 事件 ID 列在 >=900px 时始终可见。styles.css 在 <=1180px 会隐藏 .log-file
  // 并把网格降为 4 列，因此这里用行内样式覆盖表头与每行，保证最后一列仍然成立。
  const eventColumnVisible = () => viewportWidth() >= 900
  const gridColumns = () => {
    const width = viewportWidth()
    if (width > 1280) return '152px 66px 136px minmax(0, 1fr) 128px'
    if (width > 1180) return '138px 62px 118px minmax(0, 1fr) 112px'
    if (width >= 900) return '116px 52px 96px minmax(0, 1fr) 118px'
    return '108px 50px 88px minmax(0, 1fr)'
  }
  const eventCellStyle = (): JSX.CSSProperties => ({
    display: eventColumnVisible() ? 'flex' : 'none',
    'align-items': 'center',
    gap: '4px',
  })
  const eventBadgeStyle: JSX.CSSProperties = {
    display: 'inline-flex',
    'align-items': 'center',
    'align-self': 'flex-start',
    padding: '0 5px',
    border: '1px solid var(--accent-soft)',
    'border-radius': 'var(--r-xs)',
    background: 'var(--accent-ghost)',
    color: 'var(--accent-dim)',
    'font-family': 'var(--font-mono)',
    'font-size': '10px',
    'font-weight': 600,
    'font-variant-numeric': 'tabular-nums',
    'line-height': '1.6',
    'max-width': '100%',
    overflow: 'hidden',
    'text-overflow': 'ellipsis',
  }
  const eventSubtitleStyle: JSX.CSSProperties = {
    display: 'block',
    'max-width': '100%',
    'min-width': 0,
    overflow: 'hidden',
    'text-overflow': 'ellipsis',
    'white-space': 'nowrap',
    color: 'var(--text-faint)',
    'font-family': 'var(--font-sans)',
    'font-size': '9px',
    'line-height': '1.25',
  }

  const virtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
    get count() {
      return props.rows.length
    },
    getScrollElement: () => scrollEl() ?? null,
    estimateSize: () => 56,
    overscan: 10,
    measureElement: (element) => element.getBoundingClientRect().height,
  })

  createEffect(() => {
    // rows 或行展开状态变化时重新测量动态行高。
    void props.rows
    void props.expandedDocIds
    void props.selectedDocId
    queueMicrotask(() => virtualizer.measure())
  })

  createEffect(
    on(
      scrollEl,
      (element) => {
        if (!element) return
        // TanStack Virtual 在 WebView2 中首次挂载时可能拿不到 targetWindow，
        // 导致 scrollRect 恒为 0、虚拟行不渲染；轮询重新绑定观察器直到拿到尺寸。
        const v = virtualizer as unknown as {
          cleanup?: () => void
          _willUpdate?: () => void
          scrollRect?: { width: number; height: number } | null
        }
        if (typeof v.cleanup !== 'function' || typeof v._willUpdate !== 'function') return
        let attempts = 0
        const timer = window.setInterval(() => {
          attempts++
          v.cleanup?.()
          v._willUpdate?.()
          const height = v.scrollRect?.height ?? 0
          if (height > 0 || attempts >= 12) window.clearInterval(timer)
        }, 100)
        onCleanup(() => window.clearInterval(timer))
      },
    ),
  )

  createEffect(
    on(
      () => props.resetToken,
      () => {
        const el = scrollEl()
        if (el) el.scrollTop = 0
        virtualizer.scrollToOffset(0)
      },
      { defer: true },
    ),
  )

  const isExpanded = (entry: LogEntry) =>
    props.expandedDocIds.has(entry.doc_id) || props.selectedDocId === entry.doc_id

  const isErrorLevel = (entry: LogEntry) => {
    const level = String(entry.level ?? '').toUpperCase()
    return level === 'ERROR' || level === 'CRITICAL' || level === 'FATAL'
  }

  const sortMark = (field: SortField) => {
    if (props.sortBy !== field) return '↕'
    return props.sortOrder === 'asc' ? '↑' : '↓'
  }

  const handleScroll = () => {
    const el = scrollEl()
    if (!el) return
    if (props.loading || props.loadingMore || !props.hasMore) return
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 320) props.onLoadMore()
  }

  const handleKeyDown = (event: KeyboardEvent) => {
    const rows = props.rows
    if (!rows.length) return
    const current =
      props.selectedDocId == null
        ? -1
        : rows.findIndex((entry) => entry.doc_id === props.selectedDocId)

    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (current < 0) {
        props.onHighlight(rows[0])
        virtualizer.scrollToIndex(0, { align: 'auto' })
        return
      }
      const next = current + (event.key === 'ArrowDown' ? 1 : -1)
      if (next < 0) {
        props.onHighlight(rows[0])
        virtualizer.scrollToIndex(0, { align: 'auto' })
        return
      }
      if (next >= rows.length) {
        if (event.key === 'ArrowDown' && props.hasMore) props.onLoadMore()
        return
      }
      props.onHighlight(rows[next])
      virtualizer.scrollToIndex(next, { align: 'auto' })
      return
    }

    if (event.key === 'Enter') {
      if (current >= 0) {
        event.preventDefault()
        props.onOpen(rows[current])
      }
      return
    }

    if (event.key === ' ') {
      if (current >= 0) {
        event.preventDefault()
        props.onToggleExpand(rows[current])
      }
    }
  }

  return (
    <div
      style={{
        flex: 1,
        'min-height': 0,
        display: 'flex',
        'flex-direction': 'column',
      }}
    >
      <div class="log-table-head" style={{ 'grid-template-columns': gridColumns() }}>
        <button
          type="button"
          style={{ ...headButtonStyle, color: props.sortBy === 'timestamp' ? 'var(--accent)' : undefined }}
          title="按时间排序，再次点击切换升降序"
          onClick={() => props.onSort('timestamp')}
        >
          时间 <span>{sortMark('timestamp')}</span>
        </button>
        <button
          type="button"
          style={{ ...headButtonStyle, color: props.sortBy === 'level' ? 'var(--accent)' : undefined }}
          title="按级别排序，再次点击切换升降序"
          onClick={() => props.onSort('level')}
        >
          级别 <span>{sortMark('level')}</span>
        </button>
        <div>主机 / 日志源</div>
        <div>消息</div>
        <div class="log-file" style={eventCellStyle()} title="事件 ID（Windows EVTX / 结构化日志）">
          事件 ID
        </div>
      </div>

      <Show when={props.error && props.rows.length > 0}>
        <div class="danger-note" style={{ padding: '5px 12px', 'font-size': '10px' }}>
          <div class="row between" style={{ gap: '8px' }}>
            <span class="grow truncate" title={props.error ?? ''}>
              查询出错：{props.error}
            </span>
            <Button size="sm" variant="ghost" onClick={props.onRetry}>
              重试
            </Button>
          </div>
        </div>
      </Show>

      <Show when={props.loading && props.rows.length > 0}>
        <div class="loading-line" />
      </Show>

      <Show when={props.loading && props.rows.length === 0}>
        <div style={{ flex: 1, 'min-height': 0, 'overflow': 'hidden' }}>
          <For each={SKELETON_ROWS}>
            {(index) => (
              <div
                class="log-row"
                style={{
                  cursor: 'default',
                  'pointer-events': 'none',
                  'grid-template-columns': gridColumns(),
                }}
              >
                <span style={skeletonBarStyle(`${58 + (index % 3) * 9}%`)} />
                <span
                  style={{
                    ...skeletonBarStyle('44px', '13px'),
                  }}
                />
                <div class="stack" style={{ gap: '5px' }}>
                  <span style={skeletonBarStyle(`${62 + (index % 4) * 7}%`)} />
                  <span style={skeletonBarStyle('40%', '7px')} />
                </div>
                <div class="stack" style={{ gap: '5px' }}>
                  <span style={skeletonBarStyle(`${55 + (index % 5) * 8}%`)} />
                  <span style={skeletonBarStyle(`${30 + (index % 4) * 8}%`)} />
                </div>
                <div class="stack log-file" style={{ gap: '5px', ...eventCellStyle() }}>
                  <span style={skeletonBarStyle('72%')} />
                  <span style={skeletonBarStyle('44%', '7px')} />
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>

      <Show when={!props.loading && props.error && props.rows.length === 0}>
        <div
          style={{
            flex: 1,
            'min-height': 0,
            overflow: 'auto',
            display: 'grid',
            'place-items': 'center',
          }}
        >
          <EmptyState
            icon="alert"
            title="查询失败"
            text={`${props.error}。若刚完成采集，索引可能仍在构建中，请稍后重试。`}
            action={
              <div class="row" style={{ 'flex-wrap': 'wrap', 'justify-content': 'center' }}>
                <Button size="sm" variant="primary" onClick={props.onRetry}>
                  重试查询
                </Button>
                <Show when={props.projectLogCount === 0}>
                  <Button size="sm" variant="ghost" onClick={props.onCollect}>
                    前往采集
                  </Button>
                </Show>
              </div>
            }
          />
        </div>
      </Show>

      <Show when={!props.loading && !props.error && props.rows.length === 0}>
        <div
          style={{
            flex: 1,
            'min-height': 0,
            overflow: 'auto',
            display: 'grid',
            'place-items': 'center',
          }}
        >
          <Show
            when={props.projectLogCount === 0}
            fallback={
              <EmptyState
                icon="search"
                title="未找到匹配日志"
                text="尝试放宽时间范围、取消级别或主机筛选，或调整查询语法后重试。"
                action={
                  <div class="row" style={{ 'flex-wrap': 'wrap', 'justify-content': 'center' }}>
                    <Button size="sm" variant="primary" onClick={props.onClearFilters}>
                      清除筛选
                    </Button>
                    <Button size="sm" variant="ghost" onClick={props.onRetry}>
                      重新查询
                    </Button>
                  </div>
                }
              />
            }
          >
            <EmptyState
              icon="database"
              title="当前项目暂无日志"
              text="请先在数据采集页导入文件、采集本地日志，或连接 SSH 主机后再开始分析。"
              action={
                <Button size="sm" variant="primary" onClick={props.onCollect}>
                  前往采集日志
                </Button>
              }
            />
          </Show>
        </div>
      </Show>

      {/* 滚动容器必须始终挂载：虚拟滚动器在 mount 时观察滚动元素，
          若等 rows 到达后才创建元素，观察器不会重新绑定，导致 0 行渲染。 */}
      <div
        style={{
          flex: 1,
          'min-height': 0,
          display: props.rows.length > 0 ? 'flex' : 'none',
          'flex-direction': 'column',
        }}
      >
        <div
          ref={setScrollEl}
          class="scrollable scroll-thin"
          tabindex="0"
          role="grid"
          aria-label="日志查询结果"
          style={{ flex: 1, 'min-height': 0, outline: 'none' }}
          onScroll={handleScroll}
          onKeyDown={handleKeyDown}
          onMouseDown={() => scrollEl()?.focus({ preventScroll: true })}
        >
          <div
            style={{
              height: `${virtualizer.getTotalSize()}px`,
              width: '100%',
              position: 'relative',
            }}
          >
            <For each={virtualizer.getVirtualItems()}>
              {(virtualItem) => {
                const entry = () => props.rows[virtualItem.index]
                return (
                  <Show when={entry()}>
                    {(entryAccessor) => {
                      const row = () => entryAccessor()
                      return (
                        <div
                          data-index={virtualItem.index}
                          ref={(element) => virtualizer.measureElement(element)}
                          style={{
                            position: 'absolute',
                            top: 0,
                            left: 0,
                            width: '100%',
                            transform: `translateY(${virtualItem.start}px)`,
                          }}
                        >
                          <div
                            class={`log-row ${props.selectedDocId === row().doc_id ? 'selected' : ''} ${
                              isExpanded(row()) ? 'expanded' : ''
                            } ${isErrorLevel(row()) ? 'error' : ''}`}
                            role="row"
                            tabindex="-1"
                            aria-selected={props.selectedDocId === row().doc_id}
                            title={`doc #${row().doc_id} · 单击查看详情，双击展开消息与全部字段`}
                            style={{ 'grid-template-columns': gridColumns() }}
                            onClick={() => handleRowClick(row())}
                            onDblClick={(event) => handleRowDblClick(row(), event)}
                          >
                            <span
                              class="log-time"
                              title={`${formatTime(row().timestamp)} · doc #${row().doc_id}`}
                            >
                              {formatTimeShort(row().timestamp)}
                            </span>

                            <span>
                              <LevelBadge level={row().level} />
                            </span>

                            <div style={{ 'min-width': 0 }}>
                              <div class="log-source truncate" title={row().source_host || '未知主机'}>
                                {row().source_host || '—'}
                              </div>
                              <div
                                class="truncate log-source-sub"
                                title={row().source_file || row().provider || row().source || ''}
                              >
                                {basename(row().source_file) || row().provider || row().source || '未知来源'}
                              </div>
                            </div>

                            <div class="grow" style={{ 'min-width': 0 }}>
                              <div class="row" style={{ gap: '6px', 'align-items': 'flex-start' }}>
                                <span
                                  style={{
                                    width: '13px',
                                    flex: 'none',
                                    display: 'flex',
                                    'flex-direction': 'column',
                                    gap: '2px',
                                    'align-items': 'center',
                                    'padding-top': '2px',
                                  }}
                                >
                                  <Show when={row().is_anomaly}>
                                    <span title="AI 标记为异常" style={{ color: 'var(--ember)', display: 'inline-flex' }}>
                                      <Icon name="zap" size={11} />
                                    </span>
                                  </Show>
                                  <Show when={row().is_evidence}>
                                    <span title="已标记为证据" style={{ color: 'var(--accent)', display: 'inline-flex' }}>
                                      <Icon name="shield-check" size={11} />
                                    </span>
                                  </Show>
                                </span>
                                <div
                                  class="log-message grow"
                                  title={
                                    isExpanded(row())
                                      ? undefined
                                      : truncate(friendlyLogMessage(row().message), 500)
                                  }
                                >
                                  {friendlyLogMessage(row().message) || '(空消息)'}
                                </div>
                              </div>
                              <Show
                                when={
                                  isExpanded(row()) &&
                                  ((row().tags?.length ?? 0) > 0 ||
                                    row().is_anomaly ||
                                    row().is_evidence)
                                }
                              >
                                <div class="row wrap" style={{ gap: '4px', 'margin-top': '4px' }}>
                                  <Show when={row().is_anomaly}>
                                    <Badge tone="ember">异常</Badge>
                                  </Show>
                                  <Show when={row().is_evidence}>
                                    <Badge tone="mint">证据</Badge>
                                  </Show>
                                  <For each={row().tags ?? []}>
                                    {(tag) => <span class="tag">{tag}</span>}
                                  </For>
                                </div>
                              </Show>
                            </div>

                            <div class="log-file" style={eventCellStyle()}>
                              <div
                                style={{
                                  flex: 1,
                                  'min-width': 0,
                                  display: 'flex',
                                  'flex-direction': 'column',
                                  'align-items': 'flex-start',
                                  gap: '1px',
                                }}
                              >
                                <Show
                                  when={row().event_id != null}
                                  fallback={<span class="faint mono-sm">—</span>}
                                >
                                  <span
                                    style={eventBadgeStyle}
                                    title={`事件 ID ${row().event_id}${
                                      row().provider ? ` · ${row().provider}` : ''
                                    }`}
                                  >
                                    {row().event_id}
                                  </span>
                                  <Show when={row().provider}>
                                    <span style={eventSubtitleStyle} title={row().provider}>
                                      {row().provider}
                                    </span>
                                  </Show>
                                </Show>
                              </div>
                              <Show when={props.onAnalyze}>
                                <button
                                  type="button"
                                  class="log-open-page"
                                  title="AI 分析此日志"
                                  aria-label="AI 分析此日志"
                                  onClick={(event) => {
                                    event.stopPropagation()
                                    props.onAnalyze?.(row())
                                  }}
                                >
                                  <Icon name="wand" size={13} />
                                </button>
                              </Show>
                              <button
                                type="button"
                                class="log-open-page"
                                title="在独立页面查看完整日志"
                                aria-label="在独立页面查看完整日志"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  props.onOpenPage(row())
                                }}
                              >
                                <Icon name="external" size={13} />
                              </button>
                            </div>

                            <Show when={isExpanded(row())}>
                              <AllFieldsBlock entry={row()} />
                            </Show>
                          </div>
                        </div>
                      )
                    }}
                  </Show>
                )
              }}
            </For>
          </div>
        </div>

        <div
          class="row between"
          style={{
            padding: '6px 12px',
            'border-top': '1px solid var(--border)',
            'flex-wrap': 'wrap',
            gap: '8px',
            'flex': 'none',
          }}
        >
          <div class="row" style={{ gap: '10px', 'flex-wrap': 'wrap' }}>
            <span class="mono-sm faint">
              已加载{' '}
              <b style={{ color: 'var(--text-muted)', 'font-weight': 500 }}>
                {formatInt(props.rows.length)}
              </b>{' '}
              / {formatInt(props.total)}
            </span>
            <span class="mono-sm faint">
              {props.loadingMore
                ? '正在加载更多…'
                : props.hasMore
                  ? '滚动到底部自动加载'
                  : '已全部加载'}
            </span>
            <span
              class="mono-sm faint"
              title="键盘操作：上下键选择，Enter 打开详情，空格展开消息与全部字段"
            >
              ↑↓ 选择 · Enter 详情 · 双击展开消息与全部字段
            </span>
          </div>
          <div class="row" style={{ gap: '6px' }}>
            <span class="mono-sm faint">每页</span>
            <select
              class="select"
              style={{
                width: '76px',
                'min-height': '24px',
                padding: '1px 22px 1px 6px',
                'font-size': '9.5px',
              }}
              value={String(props.limit)}
              aria-label="每页数量"
              onChange={(event) => props.onLimitChange(Number(event.currentTarget.value))}
            >
              <option value="100">100</option>
              <option value="200">200</option>
              <option value="500">500</option>
            </select>
            <Show when={props.hasMore}>
              <Button size="sm" variant="ghost" loading={props.loadingMore} onClick={props.onLoadMore}>
                加载更多
              </Button>
            </Show>
          </div>
        </div>
      </div>
    </div>
  )
}
