import { For, JSX, Show } from 'solid-js'
import type { Project } from '../../types'
import {
  formatBytes,
  formatNumber,
  formatRange,
  formatTime,
  shortId,
} from '../../utils'
import { Badge, Button, Icon, IconButton } from '../ui'
import { ProjectLevelStrip } from './LevelDistributionStrip'

export interface ProjectCardProps {
  project: Project
  active?: boolean
  sourceCount?: number | null
  onOpen: (project: Project) => void
  onDetail: (project: Project) => void
  onEdit: (project: Project) => void
  onArchive: (project: Project) => void
  onExport: (project: Project) => void
  onDelete: (project: Project) => void
}

const PRIORITY_LABEL: Record<string, string> = {
  high: '高',
  medium: '中',
  low: '低',
}

export function ProjectCard(props: ProjectCardProps): JSX.Element {
  const archived = () => String(props.project.status ?? '').toLowerCase() === 'archived'
  const priority = () => String(props.project.priority ?? 'medium').toLowerCase()
  const priorityLabel = () => PRIORITY_LABEL[priority()] ?? '中'
  const tags = () => (Array.isArray(props.project.tags) ? props.project.tags.filter(Boolean) : [])
  const sourceLabel = () =>
    props.sourceCount == null ? '—' : formatNumber(props.sourceCount)

  const cardClass = () =>
    [
      'project-card',
      archived() ? 'archived' : '',
      priority() === 'high' ? 'priority-high' : '',
      priority() === 'low' ? 'priority-low' : '',
    ]
      .filter(Boolean)
      .join(' ')

  const stop = (event: MouseEvent, action: () => void) => {
    event.stopPropagation()
    action()
  }

  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.target !== event.currentTarget) return
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    props.onOpen(props.project)
  }

  return (
    <article
      class={cardClass()}
      tabIndex={0}
      aria-label={`打开项目 ${props.project.name || '未命名项目'}`}
      onClick={() => props.onOpen(props.project)}
      onKeyDown={handleKeyDown}
    >
      <div class="row" style={{ 'align-items': 'flex-start', gap: '10px' }}>
        <div class="grow">
          <div class="project-name">{props.project.name || '未命名项目'}</div>
          <div class="row wrap mono-sm faint" style={{ gap: '6px', 'margin-top': '4px' }}>
            <span>#{shortId(props.project.id)}</span>
            <span>·</span>
            <span>优先级 {priorityLabel()}</span>
            <Show when={props.active}>
              <Badge tone="mint">当前会话</Badge>
            </Show>
          </div>
        </div>
        <Badge tone={archived() ? 'muted' : 'mint'}>{archived() ? '已归档' : '活跃'}</Badge>
      </div>

      <div class="project-desc">
        {props.project.description?.trim() || '尚无项目描述。'}
      </div>

      <Show when={tags().length > 0}>
        <div class="row wrap" style={{ gap: '4px' }}>
          <For each={tags()}>{(tag) => <span class="tag">#{tag}</span>}</For>
        </div>
      </Show>

      <ProjectLevelStrip
        distribution={props.project.level_distribution}
        fallbackTotal={props.project.log_count}
        showLegend
      />

      <div class="project-metrics">
        <div class="project-metric">
          <b>{formatNumber(props.project.log_count)}</b>
          <span>日志</span>
        </div>
        <div class="project-metric">
          <b>{formatBytes(props.project.size_bytes)}</b>
          <span>占用</span>
        </div>
        <div class="project-metric" title={props.sourceCount == null ? '来源数暂时不可用' : '日志来源数量'}>
          <b>{sourceLabel()}</b>
          <span>来源</span>
        </div>
      </div>

      <div class="row" style={{ gap: '6px', 'align-items': 'flex-start', color: 'var(--text-faint)' }}>
        <span style={{ 'margin-top': '1px', display: 'inline-flex' }}>
          <Icon name="clock" size={11} />
        </span>
        <span class="mono-sm" style={{ 'line-height': '1.55' }}>
          {formatRange(props.project.time_range?.start, props.project.time_range?.end)}
        </span>
      </div>

      <div class="project-footer" style={{ 'flex-wrap': 'wrap', 'row-gap': '8px' }}>
        <div class="row" style={{ gap: '6px', 'align-items': 'center' }}>
          <Icon name="calendar" size={11} />
          <span>更新 {formatTime(props.project.updated_at)}</span>
        </div>

        <div
          class="row"
          style={{ gap: '2px', 'align-items': 'center' }}
          onClick={(event) => event.stopPropagation()}
        >
          <Button
            size="sm"
            variant="primary"
            onClick={(event) => stop(event, () => props.onOpen(props.project))}
          >
            打开
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={(event) => stop(event, () => props.onDetail(props.project))}
          >
            详情
          </Button>
          <IconButton
            icon="edit"
            title="编辑项目"
            onClick={(event) => stop(event, () => props.onEdit(props.project))}
          />
          <IconButton
            icon="package"
            title={archived() ? '项目已归档' : '归档项目'}
            disabled={archived()}
            onClick={(event) => stop(event, () => props.onArchive(props.project))}
          />
          <IconButton
            icon="download"
            title="导出项目包"
            onClick={(event) => stop(event, () => props.onExport(props.project))}
          />
          <IconButton
            icon="trash"
            title="删除项目"
            onClick={(event) => stop(event, () => props.onDelete(props.project))}
          />
        </div>
      </div>
    </article>
  )
}
