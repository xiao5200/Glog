import { For, JSX, Show, createSignal } from 'solid-js'
import type { QuerySnapshot } from '../../types'
import { Button, Icon, IconButton, LevelBadge, Modal, Spinner } from '../ui'
import { formatDuration, formatInt, formatTime } from '../../utils'
import { LOG_LEVELS, TIME_PRESETS, type AIMode, type TimePreset } from './logCommon'

export interface LogToolbarProps {
  loading: boolean
  queryText: string
  onQueryTextChange: (value: string) => void
  onSearch: () => void
  onReset: () => void

  timePreset: TimePreset
  customStart: string
  customEnd: string
  onTimePresetChange: (value: TimePreset) => void
  onCustomStartChange: (value: string) => void
  onCustomEndChange: (value: string) => void

  levels: string[]
  onToggleLevel: (level: string) => void

  showFilters: boolean
  onToggleFilters: () => void
  showCharts: boolean
  onToggleCharts: () => void
  onCluster: () => void

  snapshots: QuerySnapshot[]
  snapshotsLoading: boolean
  snapshotsError: string | null
  selectedSnapshotID: string
  onSelectSnapshot: (id: string) => void
  onSaveSnapshot: () => void
  onDeleteSnapshot: (id: string) => void

  onAI: (mode: AIMode) => void

  fieldFilterCount: number
  onClearFieldFilters: () => void

  total: number
  took: number
}

const compactInputStyle: JSX.CSSProperties = {
  width: '172px',
  'min-height': '26px',
  padding: '2px 6px',
  'font-size': '10px',
  'font-family': 'var(--font-mono)',
}

const compactSelectStyle: JSX.CSSProperties = {
  width: '172px',
  'min-height': '26px',
  padding: '2px 24px 2px 7px',
  'font-size': '10px',
}

const SYNTAX_EXAMPLES: Array<{ example: string; desc: string }> = [
  { example: 'timeout', desc: '全文关键词检索' },
  { example: 'error AND nginx', desc: '同时包含多个关键词' },
  { example: 'WARN OR ERROR', desc: '任一关键词匹配' },
  { example: 'NOT debug', desc: '排除关键词（也支持 -debug）' },
  { example: 'level:ERROR', desc: '按字段匹配，支持 level / source_host / source / event_id / message' },
  { example: 'source_host:web-01', desc: '按来源主机精确过滤' },
  { example: 'event_id:4625', desc: '按 Windows 事件 ID 过滤' },
  { example: 'timestamp:[2026-01-01 TO 2026-01-31]', desc: '字段范围语法（含边界）' },
  { example: 'message:"connection reset"', desc: '包含空格的短语使用引号' },
]

function ToolbarLabel(props: { children: JSX.Element }): JSX.Element {
  return (
    <span
      style={{
        'font-family': 'var(--font-mono)',
        'font-size': '8.5px',
        'letter-spacing': '0.14em',
        'text-transform': 'uppercase',
        color: 'var(--text-faint)',
        'margin-right': '2px',
        'white-space': 'nowrap',
      }}
    >
      {props.children}
    </span>
  )
}

function GroupDivider(): JSX.Element {
  return (
    <span
      style={{
        width: '1px',
        height: '16px',
        background: 'var(--border)',
        'flex': 'none',
        margin: '0 3px',
      }}
    />
  )
}

export function LogToolbar(props: LogToolbarProps): JSX.Element {
  const [syntaxOpen, setSyntaxOpen] = createSignal(false)

  const invalidRange = () => {
    if (props.timePreset !== 'custom' || !props.customStart || !props.customEnd) return false
    const start = new Date(props.customStart).getTime()
    const end = new Date(props.customEnd).getTime()
    return Number.isFinite(start) && Number.isFinite(end) && start > end
  }

  const extraLevels = () =>
    props.levels.filter((level) => !LOG_LEVELS.some((known) => known === level))

  return (
    <div class="panel" style={{ flex: 'none' }}>
      <div class="log-toolbar" style={{ 'flex-wrap': 'wrap', 'row-gap': '6px' }}>
        <div class="search-box" style={{ 'min-width': '280px', flex: '1 1 400px' }}>
          <Icon name="search" size={13} class="faint" />
          <input
            value={props.queryText}
            placeholder="关键词 / AND OR NOT / level:ERROR / event_id:4625"
            aria-label="日志查询"
            autocomplete="off"
            onInput={(e) => props.onQueryTextChange(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                props.onSearch()
              }
            }}
          />
          <Show when={props.queryText}>
            <IconButton
              icon="x"
              title="清空查询文本"
              onClick={() => props.onQueryTextChange('')}
            />
          </Show>
          <IconButton icon="info" title="查询语法帮助" onClick={() => setSyntaxOpen(true)} />
        </div>

        <Button variant="primary" size="sm" loading={props.loading} onClick={props.onSearch}>
          执行
        </Button>
        <Button variant="ghost" size="sm" onClick={props.onReset} disabled={props.loading}>
          重置
        </Button>

        <div class="grow" />

        <div class="metric-inline" title="当前查询结果总数">
          <span>总数</span>
          <b>{formatInt(props.total)}</b>
        </div>
        <div class="metric-inline" title="查询耗时">
          <span>耗时</span>
          <b>{formatDuration(props.took)}</b>
        </div>
        <Show when={props.fieldFilterCount > 0}>
          <button
            type="button"
            class="chip selected"
            title="清除 AI 生成的字段条件"
            onClick={props.onClearFieldFilters}
          >
            字段条件 {props.fieldFilterCount} ×
          </button>
        </Show>
      </div>

      <div
        class="log-toolbar"
        style={{
          'border-bottom': 'none',
          'padding-top': '4px',
          'flex-wrap': 'wrap',
          'row-gap': '6px',
          'column-gap': '8px',
        }}
      >
        <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
          <ToolbarLabel>时间</ToolbarLabel>
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
          <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
            <input
              type="datetime-local"
              class="input"
              style={compactInputStyle}
              value={props.customStart}
              title="开始时间（本地时间）"
              aria-label="自定义开始时间"
              onInput={(e) => props.onCustomStartChange(e.currentTarget.value)}
            />
            <span class="faint mono-sm">→</span>
            <input
              type="datetime-local"
              class="input"
              style={compactInputStyle}
              value={props.customEnd}
              title="结束时间（本地时间）"
              aria-label="自定义结束时间"
              onInput={(e) => props.onCustomEndChange(e.currentTarget.value)}
            />
            <Show when={invalidRange()}>
              <span class="mono-sm ember">范围无效</span>
            </Show>
          </div>
        </Show>

        <GroupDivider />

        <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
          <ToolbarLabel>级别</ToolbarLabel>
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
                title={`移除 ${level} 级别（来自查询快照或 AI）`}
                onClick={() => props.onToggleLevel(level)}
              >
                <LevelBadge level={level} /> ×
              </button>
            )}
          </For>
        </div>

        <GroupDivider />

        <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
          <ToolbarLabel>视图</ToolbarLabel>
          <button
            type="button"
            class={`chip ${props.showFilters ? 'selected' : ''}`}
            title="显示或隐藏左侧筛选栏"
            onClick={props.onToggleFilters}
          >
            <Icon name="filter" size={11} />
            筛选
          </button>
          <button
            type="button"
            class={`chip ${props.showCharts ? 'selected' : ''}`}
            title="显示或隐藏聚合图表面板"
            onClick={props.onToggleCharts}
          >
            <Icon name="chart" size={11} />
            图表
          </button>
          <button type="button" class="chip" title="按模式聚类当前查询结果" onClick={props.onCluster}>
            <Icon name="layers" size={11} />
            聚类
          </button>
        </div>

        <GroupDivider />

        <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
          <ToolbarLabel>快照</ToolbarLabel>
          <Show when={props.snapshotsLoading}>
            <Spinner />
          </Show>
          <select
            class="select"
            style={compactSelectStyle}
            value={props.selectedSnapshotID}
            title="应用查询快照"
            aria-label="查询快照"
            disabled={props.snapshotsLoading}
            onChange={(e) => props.onSelectSnapshot(e.currentTarget.value)}
          >
            <option value="">{props.snapshots.length ? '选择快照…' : '暂无快照'}</option>
            <For each={props.snapshots}>
              {(snapshot) => (
                <option
                  value={snapshot.id}
                  title={`${snapshot.name} · ${formatInt(snapshot.result_count)} 条 · ${formatTime(snapshot.created_at)}`}
                >
                  {snapshot.name}
                </option>
              )}
            </For>
          </select>
          <IconButton
            icon="save"
            title="保存当前查询为快照"
            onClick={props.onSaveSnapshot}
            class={props.loading ? '' : 'accent'}
          />
          <IconButton
            icon="trash"
            title="删除当前选中的快照"
            disabled={!props.selectedSnapshotID || props.snapshotsLoading}
            onClick={() => {
              if (props.selectedSnapshotID) props.onDeleteSnapshot(props.selectedSnapshotID)
            }}
          />
          <Show when={props.snapshotsError}>
            <span class="badge badge-ember" title={props.snapshotsError ?? ''}>
              快照错误
            </span>
          </Show>
        </div>

        <GroupDivider />

        <div class="row" style={{ gap: '4px', 'flex-wrap': 'wrap' }}>
          <ToolbarLabel>AI</ToolbarLabel>
          <button type="button" class="chip" title="AI 异常检测" onClick={() => props.onAI('anomaly')}>
            <Icon name="zap" size={11} />
            异常
          </button>
          <button type="button" class="chip" title="自然语言转查询" onClick={() => props.onAI('nlquery')}>
            <Icon name="wand" size={11} />
            NL 查询
          </button>
          <button type="button" class="chip" title="AI 根因分析" onClick={() => props.onAI('rootcause')}>
            <Icon name="branch" size={11} />
            根因
          </button>
          <button type="button" class="chip" title="生成分析报告" onClick={() => props.onAI('report')}>
            <Icon name="book" size={11} />
            报告
          </button>
        </div>
      </div>

      <Modal
        open={syntaxOpen()}
        title="查询语法"
        subtitle="全文检索 / 布尔逻辑 / 字段过滤 / 范围"
        size="lg"
        onClose={() => setSyntaxOpen(false)}
        footer={
          <Button variant="primary" onClick={() => setSyntaxOpen(false)}>
            关闭
          </Button>
        }
      >
        <div class="stack" style={{ gap: '13px' }}>
          <p class="muted" style={{ 'font-size': '11.5px', 'line-height': '1.7' }}>
            查询文本会与工具栏中的时间、级别以及左侧筛选条件按 AND 组合。字段名与值区分大小写请以索引结果为准。
          </p>
          <div class="stack" style={{ gap: '6px' }}>
            <For each={SYNTAX_EXAMPLES}>
              {(item) => (
                <div class="row" style={{ gap: '10px', 'align-items': 'flex-start' }}>
                  <code
                    class="kbd"
                    style={{
                      width: '280px',
                      flex: 'none',
                      padding: '3px 7px',
                      'font-size': '9px',
                      'white-space': 'pre-wrap',
                      'word-break': 'break-word',
                      'line-height': '1.5',
                    }}
                  >
                    {item.example}
                  </code>
                  <span class="muted" style={{ 'font-size': '11px', 'line-height': '1.6' }}>
                    {item.desc}
                  </span>
                </div>
              )}
            </For>
          </div>
          <div class="success-note">
            可用字段示例：text、level、source_host、source、source_file、event_id、provider、computer、user_id、risk_score、message。
          </div>
          <div class="detail-note">
            时间范围也可通过 timestamp:[开始 TO 结束] 表达；左侧时间快捷选项始终优先用于结构化时间过滤。复杂布尔表达式请使用大写 AND / OR / NOT。
          </div>
        </div>
      </Modal>
    </div>
  )
}
