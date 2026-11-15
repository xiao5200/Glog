import { For, JSX, Show, createMemo } from 'solid-js'
import type { LogSource, Project, ProjectStatistics } from '../../types'
import {
  Badge,
  Button,
  Drawer,
  EmptyState,
  Icon,
  PageBusy,
  SectionLabel,
  StatTile,
} from '../ui'
import {
  formatBytes,
  formatNumber,
  formatRange,
  formatTime,
  percentWidth,
  shortId,
} from '../../utils'
import { ProjectLevelStrip } from './LevelDistributionStrip'

export interface ProjectDetailDrawerProps {
  open: boolean
  project: Project | null
  statistics: ProjectStatistics | null
  sources: LogSource[]
  loading: boolean
  error: string
  onClose: () => void
  onReload: () => void
  onOpen: (project: Project) => void
  onExport: (project: Project) => void
  onArchive: (project: Project) => void
}

export function ProjectDetailDrawer(props: ProjectDetailDrawerProps): JSX.Element {
  const archived = () =>
    String(props.project?.status ?? '').toLowerCase() === 'archived'
  const priority = () => String(props.project?.priority ?? 'medium').toLowerCase()
  const priorityLabel = () =>
    priority() === 'high' ? '高' : priority() === 'low' ? '低' : '中'
  const priorityTone = (): 'ember' | 'amber' | 'muted' =>
    priority() === 'high' ? 'ember' : priority() === 'low' ? 'muted' : 'amber'
  const range = () => props.statistics?.time_range ?? props.project?.time_range

  const topHosts = createMemo(() => {
    const rawHosts = props.statistics?.top_hosts
    const list = Array.isArray(rawHosts) ? rawHosts : []
    return [...list]
      .sort((left, right) => Number(right.log_count) - Number(left.log_count))
      .slice(0, 10)
  })
  const maxHostCount = createMemo(() =>
    Math.max(1, ...topHosts().map((host) => Number(host.log_count) || 0)),
  )

  const sourceTimeRange = (source: LogSource) =>
    formatRange(source.time_range?.start, source.time_range?.end)

  return (
    <Drawer
      open={props.open}
      title={props.project?.name || '项目情报'}
      subtitle={
        props.project
          ? `项目情报档案 · ${shortId(props.project.id, 12)} · ${archived() ? '已归档' : '活跃'}`
          : 'PROJECT INTELLIGENCE'
      }
      wide
      onClose={props.onClose}
      footer={
        <Show when={props.project}>
          {(project) => (
            <>
              <Button
                variant="ghost"
                onClick={() => props.onExport(project())}
              >
                <Icon name="download" size={12} />
                导出项目包
              </Button>
              <Button
                variant="ghost"
                disabled={archived()}
                title={archived() ? '项目已归档' : '归档项目'}
                onClick={() => props.onArchive(project())}
              >
                <Icon name="package" size={12} />
                {archived() ? '已归档' : '归档项目'}
              </Button>
              <Button
                variant="primary"
                onClick={() => props.onOpen(project())}
              >
                <Icon name="arrow-right" size={12} />
                打开项目
              </Button>
            </>
          )}
        </Show>
      }
    >
      <Show
        when={props.project}
        fallback={<EmptyState icon="folder" title="未选择项目" text="请从项目档案中选择一个项目。" />}
      >
        {(project) => (
          <div class="stack">
            <div class="row between wrap" style={{ 'align-items': 'flex-start' }}>
              <div class="row wrap" style={{ gap: '7px' }}>
                <Badge tone={archived() ? 'muted' : 'mint'}>
                  {archived() ? '已归档' : '活跃项目'}
                </Badge>
                <Badge tone={priorityTone()}>{priorityLabel()}优先级</Badge>
                <span class="mono-sm faint">ID {shortId(project().id, 12)}</span>
              </div>
              <Show when={!props.loading}>
                <span class="mono-sm faint">
                  数据源 {formatNumber(props.sources.length)} 个
                </span>
              </Show>
            </div>

            <Show when={project().description?.trim()}>
              <p class="muted" style={{ 'font-size': '11.5px', 'line-height': '1.7', 'white-space': 'pre-wrap' }}>
                {project().description}
              </p>
            </Show>

            <Show when={(project().tags?.length ?? 0) > 0}>
              <div class="row wrap" style={{ gap: '4px' }}>
                <For each={project().tags ?? []}>
                  {(tag) => <span class="tag">#{tag}</span>}
                </For>
              </div>
            </Show>

            <PageBusy visible={props.loading} text="正在读取项目统计与来源清单…" />

            <Show when={props.error}>
              <div class="danger-note">
                <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                  <span style={{ 'margin-top': '1px', display: 'inline-flex' }}>
                    <Icon name="alert" size={13} />
                  </span>
                  <div class="grow" style={{ 'font-size': '11px', 'line-height': '1.65' }}>
                    {props.error}
                  </div>
                  <Button size="sm" variant="ghost" onClick={props.onReload}>
                    重试
                  </Button>
                </div>
              </div>
            </Show>

            <SectionLabel>项目统计</SectionLabel>
            <div class="stat-grid">
              <StatTile
                label="日志总量"
                value={formatNumber(props.statistics?.total_logs ?? project().log_count)}
                note="已解析条目"
                tone="mint"
              />
              <StatTile
                label="占用空间"
                value={formatBytes(props.statistics?.size_bytes ?? project().size_bytes)}
                note="日志存储"
                tone="sky"
              />
              <StatTile
                label="来源数量"
                value={formatNumber(props.statistics?.source_count ?? props.sources.length)}
                note="采集通道"
              />
              <StatTile
                label="索引大小"
                value={props.statistics ? formatBytes(props.statistics.index_size_bytes) : '—'}
                note="全文索引"
                tone="amber"
              />
              <StatTile
                label="唯一词项"
                value={props.statistics ? formatNumber(props.statistics.unique_terms) : '—'}
                note="索引词条"
              />
            </div>

            <div
              class="row wrap"
              style={{
                gap: '8px 20px',
                padding: '9px 12px',
                border: '1px solid var(--border)',
                background: 'var(--surface)',
              }}
            >
              <div class="row" style={{ gap: '6px' }}>
                <Icon name="calendar" size={12} />
                <span class="mono-sm faint">时间范围</span>
                <span class="mono-sm">{formatRange(range()?.start, range()?.end)}</span>
              </div>
              <div class="row" style={{ gap: '6px' }}>
                <Icon name="clock" size={12} />
                <span class="mono-sm faint">档案更新</span>
                <span class="mono-sm">{formatTime(project().updated_at)}</span>
              </div>
            </div>

            <div
              style={{
                display: 'grid',
                'grid-template-columns': 'repeat(auto-fit, minmax(250px, 1fr))',
                gap: '12px',
              }}
            >
              <div class="panel">
                <div class="panel-head">
                  <div class="panel-title">
                    <span class="tick" />
                    级别分布
                  </div>
                  <Badge tone="muted">LEVEL</Badge>
                </div>
                <div class="panel-body">
                  <ProjectLevelStrip
                    distribution={
                      props.statistics?.level_distribution ??
                      project().level_distribution
                    }
                    fallbackTotal={props.statistics?.total_logs ?? project().log_count}
                    height={6}
                    showLegend
                    detailedLegend
                  />
                </div>
              </div>

              <div class="panel">
                <div class="panel-head">
                  <div class="panel-title">
                    <span class="tick" />
                    Top 主机
                  </div>
                  <Badge tone="muted">TOP {topHosts().length}</Badge>
                </div>
                <div class="panel-body">
                  <Show
                    when={topHosts().length > 0}
                    fallback={
                      <div class="mono-sm faint">
                        {props.loading ? '正在聚合主机统计…' : '暂无主机统计。'}
                      </div>
                    }
                  >
                    <div class="stack" style={{ gap: '10px' }}>
                      <For each={topHosts()}>
                        {(host) => (
                          <div>
                            <div class="row between" style={{ gap: '10px' }}>
                              <span
                                class="mono-sm truncate"
                                style={{ 'flex': '1' }}
                                title={host.host || '未知主机'}
                              >
                                {host.host || '未知主机'}
                              </span>
                              <span class="mono-sm faint numeric">
                                {formatNumber(host.log_count)}
                              </span>
                            </div>
                            <div class="mini-bar" style={{ 'margin-top': '4px' }}>
                              <span
                                style={{
                                  width: percentWidth(
                                    Number(host.log_count) || 0,
                                    maxHostCount(),
                                  ),
                                }}
                              />
                            </div>
                          </div>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>
              </div>
            </div>

            <div class="panel">
              <div class="panel-head">
                <div class="panel-title">
                  <span class="tick" />
                  日志来源
                </div>
                <span class="mono-sm faint">
                  {formatNumber(props.sources.length)} 个来源
                </span>
              </div>
              <div class="panel-body tight" style={{ 'overflow-x': 'auto' }}>
                <Show
                  when={props.sources.length > 0}
                  fallback={
                    <Show
                      when={props.loading}
                      fallback={
                        <Show
                          when={props.error}
                          fallback={
                            <EmptyState
                              icon="server"
                              title="尚无日志来源"
                              text="打开项目后可开始本地采集、SSH 采集或导入日志文件。"
                            />
                          }
                        >
                          <div
                            class="row"
                            style={{
                              padding: '22px',
                              'justify-content': 'center',
                              color: 'var(--text-faint)',
                              'font-size': '11px',
                            }}
                          >
                            来源清单暂不可用，请重试统计加载。
                          </div>
                        </Show>
                      }
                    >
                      <div
                        class="row"
                        style={{
                          padding: '22px',
                          'justify-content': 'center',
                          color: 'var(--text-faint)',
                          'font-size': '11px',
                        }}
                      >
                        正在读取来源清单…
                      </div>
                    </Show>
                  }
                >
                  <table class="data-table" style={{ 'min-width': '780px' }}>
                    <thead>
                      <tr>
                        <th>类型</th>
                        <th>主机</th>
                        <th>路径</th>
                        <th>模板</th>
                        <th style={{ 'text-align': 'right' }}>条数</th>
                        <th style={{ 'text-align': 'right' }}>大小</th>
                        <th>时间范围</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={props.sources}>
                        {(source) => (
                          <tr>
                            <td>
                              <Badge tone="muted">{source.type || 'unknown'}</Badge>
                            </td>
                            <td>
                              <span class="mono-sm" title={source.host || '未知主机'}>
                                {source.host || '—'}
                              </span>
                            </td>
                            <td>
                              <span
                                class="mono-sm truncate"
                                style={{ display: 'block', 'max-width': '260px' }}
                                title={source.path || '—'}
                              >
                                {source.path || '—'}
                              </span>
                            </td>
                            <td>
                              <span
                                class="mono-sm faint truncate"
                                style={{ display: 'block', 'max-width': '160px' }}
                                title={source.template_id || source.parse_mode || '—'}
                              >
                                {source.template_id || source.parse_mode || '—'}
                              </span>
                            </td>
                            <td class="numeric" style={{ 'text-align': 'right' }}>
                              {formatNumber(source.log_count)}
                            </td>
                            <td class="numeric" style={{ 'text-align': 'right' }}>
                              {formatBytes(source.size_bytes)}
                            </td>
                            <td>
                              <span class="mono-sm faint">{sourceTimeRange(source)}</span>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </Show>
              </div>
            </div>
          </div>
        )}
      </Show>
    </Drawer>
  )
}
