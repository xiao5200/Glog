import { For, Show, createMemo, createSignal } from 'solid-js'
import type { Investigation } from '../../types'
import { formatTimeShort, parseTime, riskTone, truncate } from '../../utils'
import { Badge, Button, EmptyState, Icon, PageBusy, Tabs } from '../ui'
import {
  clueTypeLabel,
  investigationStatusLabel,
  investigationStatusTone,
} from './meta'

export interface InvestigationListProps {
  investigations: Investigation[]
  selectedId?: string
  loading?: boolean
  error?: string
  onSelect: (investigation: Investigation) => void
  onRetry?: () => void
}

type StatusFilter = 'all' | 'open' | 'analyzing' | 'closed'

const FILTERS: Array<{ key: StatusFilter; label: string }> = [
  { key: 'all', label: '全部' },
  { key: 'open', label: '待处理' },
  { key: 'analyzing', label: '分析中' },
  { key: 'closed', label: '已结案' },
]

export function InvestigationList(props: InvestigationListProps) {
  const [filter, setFilter] = createSignal<StatusFilter>('all')
  const [search, setSearch] = createSignal('')

  const counts = createMemo(() => {
    const result: Record<StatusFilter, number> = {
      all: props.investigations.length,
      open: 0,
      analyzing: 0,
      closed: 0,
    }
    for (const item of props.investigations) {
      const status = String(item.status ?? '').toLowerCase()
      if (status === 'open' || status === 'analyzing' || status === 'closed') {
        result[status] += 1
      }
    }
    return result
  })

  const tabs = createMemo(() =>
    FILTERS.map((item) => ({ key: item.key, label: item.label, count: counts()[item.key] })),
  )

  const filtered = createMemo(() => {
    const active = filter()
    const query = search().trim().toLowerCase()
    return props.investigations
      .filter((item) => {
        if (active !== 'all' && String(item.status ?? '').toLowerCase() !== active) return false
        if (!query) return true
        const haystack = [
          item.title,
          item.description,
          item.clue_value,
          clueTypeLabel(item.clue_type),
          item.assigned_to,
          ...(item.tags ?? []),
        ]
          .map((value) => String(value ?? '').toLowerCase())
          .join('\n')
        return haystack.includes(query)
      })
      .slice()
      .sort((a, b) => {
        const left = parseTime(a.updated_at)?.getTime() ?? 0
        const right = parseTime(b.updated_at)?.getTime() ?? 0
        return right - left
      })
  })

  const riskLabel = (score: number | undefined) => (score != null ? String(score) : '—')

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          工单队列
          <Badge tone="muted">{props.investigations.length}</Badge>
        </div>
      </div>

      <div style={{ padding: '0 12px', 'border-bottom': '1px solid var(--border)' }}>
        <Tabs
          items={tabs()}
          active={filter()}
          onChange={(key) => setFilter(key as StatusFilter)}
        />
      </div>

      <div style={{ padding: '10px 12px' }}>
        <div class="search-box">
          <span class="search-prefix">
            <Icon name="search" size={12} />
          </span>
          <input
            value={search()}
            placeholder="搜索标题、描述、线索、标签…"
            aria-label="搜索调查工单"
            onInput={(e) => setSearch(e.currentTarget.value)}
          />
          <Show when={search()}>
            <button
              type="button"
              class="icon-btn"
              style={{ width: '22px', height: '22px' }}
              title="清空搜索"
              onClick={() => setSearch('')}
            >
              <Icon name="x" size={11} />
            </button>
          </Show>
        </div>
      </div>

      <Show when={props.error}>
        <div style={{ padding: '0 12px 10px' }}>
          <div class="danger-note">
            <div class="stack" style={{ gap: '6px' }}>
              <span>{props.error}</span>
              <div>
                <Button size="sm" variant="ghost" onClick={() => props.onRetry?.()}>
                  重试
                </Button>
              </div>
            </div>
          </div>
        </div>
      </Show>

      <Show when={props.loading}>
        <div style={{ padding: '0 12px' }}>
          <PageBusy visible text="加载工单…" />
        </div>
      </Show>

      <Show when={!props.loading && !props.error}>
        <Show
          when={filtered().length > 0}
          fallback={
            <EmptyState
              icon="shield-check"
              title={props.investigations.length === 0 ? '暂无调查工单' : '没有匹配的工单'}
              text={
                props.investigations.length === 0
                  ? '新建工单，将线索、证据与研判时间线聚合到一处。'
                  : '调整状态筛选或搜索关键字后重试。'
              }
            />
          }
        >
          <div class="scrollable" style={{ 'max-height': 'calc(100vh - 430px)', 'min-height': '260px' }}>
            <For each={filtered()}>
              {(item) => (
                <button
                  type="button"
                  class={`list-row ${props.selectedId === item.id ? 'active' : ''}`}
                  style={{
                    'align-items': 'flex-start',
                    'border-top': 'none',
                    'border-right': 'none',
                  }}
                  onClick={() => props.onSelect(item)}
                >
                  <div class="grow" style={{ 'min-width': 0 }}>
                    <div class="row" style={{ gap: '6px' }}>
                      <span class="title truncate grow" title={item.title}>
                        {item.title || '未命名工单'}
                      </span>
                      <Badge tone={investigationStatusTone(item.status)}>
                        {investigationStatusLabel(item.status)}
                      </Badge>
                    </div>

                    <div class="row" style={{ gap: '6px', 'margin-top': '3px', 'min-width': 0 }}>
                      <span class="mono-sm faint" style={{ flex: 'none' }}>
                        {clueTypeLabel(item.clue_type)}
                      </span>
                      <span
                        class="mono-sm truncate"
                        style={{ color: 'var(--text-muted)' }}
                        title={item.clue_value}
                      >
                        {item.clue_value || '未填写线索值'}
                      </span>
                    </div>

                    <div
                      class="meta"
                      style={{ display: 'flex', 'flex-wrap': 'wrap', gap: '5px', 'align-items': 'center' }}
                    >
                      <span>证据 {item.evidence_ids?.length ?? 0}</span>
                      <span>·</span>
                      <span>{formatTimeShort(item.updated_at)}</span>
                      <Show when={item.assigned_to}>
                        <span>·</span>
                        <span class="truncate" style={{ 'max-width': '90px' }} title={item.assigned_to}>
                          {item.assigned_to}
                        </span>
                      </Show>
                      <Show when={item.tags?.length}>
                        <span>·</span>
                        <span class="truncate" style={{ 'max-width': '110px' }} title={(item.tags ?? []).join(', ')}>
                          #{truncate((item.tags ?? []).join(' #'), 22)}
                        </span>
                      </Show>
                    </div>
                  </div>

                  <div style={{ 'flex': 'none', 'align-self': 'center' }}>
                    <Badge tone={riskTone(item.risk_score)}>风险 {riskLabel(item.risk_score)}</Badge>
                  </div>
                </button>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}
