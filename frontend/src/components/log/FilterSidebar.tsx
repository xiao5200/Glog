import { For, JSX, Show, createMemo, createSignal } from 'solid-js'
import type { LogSource } from '../../types'
import { Button, IconButton, LevelBadge, Spinner } from '../ui'
import { formatNumber } from '../../utils'
import {
  LOG_LEVELS,
  TIME_PRESETS,
  basename,
  parseEventIDs,
  type HostFacet,
  type TimePreset,
} from './logCommon'

export interface FilterSidebarProps {
  timePreset: TimePreset
  customStart: string
  customEnd: string
  levels: string[]
  hosts: HostFacet[]
  hostsLoading: boolean
  hostsError: string | null
  selectedHosts: string[]
  sources: LogSource[]
  sourcesLoading: boolean
  sourcesError: string | null
  selectedSources: string[]
  eventIDText: string
  activeCount: number
  extraFilterCount: number

  onTimePresetChange: (value: TimePreset) => void
  onCustomStartChange: (value: string) => void
  onCustomEndChange: (value: string) => void
  onToggleLevel: (level: string) => void
  onToggleHost: (host: string) => void
  onToggleSource: (id: string) => void
  onEventIDChange: (value: string) => void
  onClear: () => void
  onRetryHosts: () => void
  onRetrySources: () => void
  onCollapse: () => void
}

const compactInputStyle: JSX.CSSProperties = {
  'min-height': '26px',
  padding: '2px 7px',
  'font-size': '10px',
  'font-family': 'var(--font-mono)',
}

export function FilterSidebar(props: FilterSidebarProps): JSX.Element {
  const [hostQuery, setHostQuery] = createSignal('')

  const mergedHosts = createMemo(() => {
    const list = props.hosts.slice()
    const seen = new Set(list.map((item) => item.host))
    for (const host of props.selectedHosts) {
      if (!seen.has(host)) {
        list.unshift({ host, count: 0 })
        seen.add(host)
      }
    }
    return list
  })

  const filteredHosts = createMemo(() => {
    const term = hostQuery().trim().toLowerCase()
    if (!term) return mergedHosts()
    return mergedHosts().filter((item) => item.host.toLowerCase().includes(term))
  })

  const sortedSources = createMemo(() =>
    props.sources
      .slice()
      .sort((a, b) => (Number(b.log_count) || 0) - (Number(a.log_count) || 0)),
  )

  const validEventIDs = createMemo(() => parseEventIDs(props.eventIDText))

  const extraLevels = () =>
    props.levels.filter((level) => !LOG_LEVELS.some((known) => known === level))

  const invalidRange = () => {
    if (props.timePreset !== 'custom' || !props.customStart || !props.customEnd) return false
    const start = new Date(props.customStart).getTime()
    const end = new Date(props.customEnd).getTime()
    return Number.isFinite(start) && Number.isFinite(end) && start > end
  }

  return (
    <aside
      class="panel"
      style={{ height: '100%', 'min-height': 0, display: 'flex', 'flex-direction': 'column' }}
    >
      <div class="panel-head" style={{ 'min-height': '38px', padding: '7px 12px' }}>
        <div class="panel-title">
          <span class="tick" />
          筛选
        </div>
        <div class="row" style={{ gap: '4px' }}>
          <Show when={props.activeCount > 0}>
            <span class="badge badge-mint" title="当前启用的筛选条件数量">
              {props.activeCount}
            </span>
          </Show>
          <IconButton icon="x" title="收起筛选栏" onClick={props.onCollapse} />
        </div>
      </div>

      <div class="scrollable scroll-thin" style={{ flex: 1, 'min-height': 0, 'overflow-y': 'auto' }}>
        <div class="filter-block">
          <div class="filter-title">
            <span>时间范围</span>
            <span>{props.timePreset === 'all' ? 'ALL' : 'LIMITED'}</span>
          </div>
          <div class="row wrap" style={{ gap: '4px' }}>
            <For each={TIME_PRESETS}>
              {(preset) => (
                <button
                  type="button"
                  class={`chip ${props.timePreset === preset.value ? 'selected' : ''}`}
                  title={preset.title}
                  onClick={() => props.onTimePresetChange(preset.value)}
                >
                  {preset.label}
                </button>
              )}
            </For>
          </div>
          <Show when={props.timePreset === 'custom'}>
            <div class="stack" style={{ gap: '5px', 'margin-top': '7px' }}>
              <input
                type="datetime-local"
                class="input"
                style={compactInputStyle}
                value={props.customStart}
                title="开始时间（本地时间）"
                aria-label="开始时间"
                onInput={(e) => props.onCustomStartChange(e.currentTarget.value)}
              />
              <input
                type="datetime-local"
                class="input"
                style={compactInputStyle}
                value={props.customEnd}
                title="结束时间（本地时间）"
                aria-label="结束时间"
                onInput={(e) => props.onCustomEndChange(e.currentTarget.value)}
              />
              <Show when={invalidRange()}>
                <span class="mono-sm ember">开始时间晚于结束时间</span>
              </Show>
            </div>
          </Show>
        </div>

        <div class="filter-block">
          <div class="filter-title">
            <span>日志级别</span>
            <span>{props.levels.length ? `${props.levels.length} 项` : '全部'}</span>
          </div>
          <div class="row wrap" style={{ gap: '4px' }}>
            <For each={LOG_LEVELS}>
              {(level) => (
                <button
                  type="button"
                  class={`chip ${props.levels.includes(level) ? 'selected' : ''}`}
                  title={`${props.levels.includes(level) ? '取消' : '添加'} ${level} 级别`}
                  onClick={() => props.onToggleLevel(level)}
                >
                  <LevelBadge level={level} />
                </button>
              )}
            </For>
            <For each={extraLevels()}>
              {(level) => (
                <button
                  type="button"
                  class="chip selected"
                  title={`移除 ${level} 级别`}
                  onClick={() => props.onToggleLevel(level)}
                >
                  {level} ×
                </button>
              )}
            </For>
          </div>
        </div>

        <div class="filter-block">
          <div class="filter-title">
            <span>主机 TOP 50</span>
            <span class="row" style={{ gap: '5px' }}>
              <Show when={props.selectedHosts.length > 0}>
                <span style={{ color: 'var(--accent-dim)' }}>{props.selectedHosts.length} 选</span>
              </Show>
              <Show when={props.hostsLoading}>
                <Spinner />
              </Show>
            </span>
          </div>

          <Show when={mergedHosts().length > 8 || hostQuery()}>
            <input
              class="input"
              style={{ ...compactInputStyle, 'margin-bottom': '6px' }}
              value={hostQuery()}
              placeholder="搜索主机…"
              aria-label="搜索主机"
              onInput={(e) => setHostQuery(e.currentTarget.value)}
            />
          </Show>

          <Show when={props.hostsError}>
            <div class="detail-note" style={{ padding: '5px 7px', 'margin-bottom': '6px' }}>
              <div class="row between" style={{ gap: '6px' }}>
                <span class="grow truncate mono-sm" title={props.hostsError ?? ''}>
                  主机聚合失败
                </span>
                <Button size="sm" variant="ghost" onClick={props.onRetryHosts}>
                  重试
                </Button>
              </div>
            </div>
          </Show>

          <Show
            when={filteredHosts().length > 0}
            fallback={
              <div class="row" style={{ gap: '6px', 'min-height': '26px' }}>
                <Show when={props.hostsLoading} fallback={<span class="mono-sm faint">暂无主机数据</span>}>
                  <Spinner />
                  <span class="mono-sm faint">加载主机聚合…</span>
                </Show>
              </div>
            }
          >
            <div class="filter-list">
              <For each={filteredHosts()}>
                {(host) => {
                  const selected = () => props.selectedHosts.includes(host.host)
                  return (
                    <button
                      type="button"
                      class={`filter-option ${selected() ? 'active' : ''}`}
                      title={`${selected() ? '取消选择' : '选择'} ${host.host}`}
                      onClick={() => props.onToggleHost(host.host)}
                    >
                      <span class="grow truncate">{host.host}</span>
                      <span class="count">
                        {host.count > 0 ? formatNumber(host.count) : '已选'}
                      </span>
                    </button>
                  )
                }}
              </For>
            </div>
          </Show>
        </div>

        <div class="filter-block">
          <div class="filter-title">
            <span>日志源</span>
            <span class="row" style={{ gap: '5px' }}>
              <Show when={props.selectedSources.length > 0}>
                <span style={{ color: 'var(--accent-dim)' }}>{props.selectedSources.length} 选</span>
              </Show>
              <Show when={props.sourcesLoading}>
                <Spinner />
              </Show>
            </span>
          </div>

          <Show when={props.sourcesError}>
            <div class="detail-note" style={{ padding: '5px 7px', 'margin-bottom': '6px' }}>
              <div class="row between" style={{ gap: '6px' }}>
                <span class="grow truncate mono-sm" title={props.sourcesError ?? ''}>
                  日志源加载失败
                </span>
                <Button size="sm" variant="ghost" onClick={props.onRetrySources}>
                  重试
                </Button>
              </div>
            </div>
          </Show>

          <Show
            when={sortedSources().length > 0}
            fallback={
              <div class="row" style={{ gap: '6px', 'min-height': '26px' }}>
                <Show when={props.sourcesLoading} fallback={<span class="mono-sm faint">暂无日志源</span>}>
                  <Spinner />
                  <span class="mono-sm faint">加载日志源…</span>
                </Show>
              </div>
            }
          >
            <div class="filter-list">
              <For each={sortedSources()}>
                {(source) => {
                  const selected = () => props.selectedSources.includes(source.id)
                  const label = `${source.host || source.type || '来源'} · ${basename(source.path) || source.id}`
                  return (
                    <button
                      type="button"
                      class={`filter-option ${selected() ? 'active' : ''}`}
                      title={`${source.host || '未知主机'}\n${source.path}\n${selected() ? '取消选择' : '选择'}该日志源`}
                      onClick={() => props.onToggleSource(source.id)}
                    >
                      <span class="grow truncate">{label}</span>
                      <span class="count">{formatNumber(source.log_count)}</span>
                    </button>
                  )
                }}
              </For>
            </div>
          </Show>
        </div>

        <div class="filter-block">
          <div class="filter-title">
            <span>事件 ID</span>
            <span>{props.eventIDText.trim() ? `${validEventIDs().length} 个有效` : '未限制'}</span>
          </div>
          <input
            class="input mono"
            style={compactInputStyle}
            value={props.eventIDText}
            placeholder="如 4624, 4625"
            aria-label="事件 ID 过滤"
            onInput={(e) => props.onEventIDChange(e.currentTarget.value)}
          />
          <Show
            when={props.eventIDText.trim() && validEventIDs().length === 0}
            fallback={
              <div class="mono-sm faint" style={{ 'margin-top': '5px' }}>
                多个 ID 用逗号分隔，非法值会被忽略。
              </div>
            }
          >
            <div class="mono-sm ember" style={{ 'margin-top': '5px' }}>
              未解析到有效的事件 ID。
            </div>
          </Show>
        </div>
      </div>

      <div style={{ padding: '9px 12px', 'border-top': '1px solid var(--border)' }}>
        <Show when={props.extraFilterCount > 0}>
          <div class="mono-sm faint" style={{ 'margin-bottom': '6px' }}>
            另含 {props.extraFilterCount} 个 AI 字段条件
          </div>
        </Show>
        <Button
          size="sm"
          variant="ghost"
          class="btn-block"
          disabled={props.activeCount === 0}
          onClick={props.onClear}
        >
          清除筛选
        </Button>
      </div>
    </aside>
  )
}
