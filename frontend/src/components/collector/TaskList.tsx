import { For, Show, createMemo, createSignal, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { TaskInfo } from '../../types'
import { formatInt, formatTime, shortId } from '../../utils'
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  IconButton,
  ProgressBar,
  Spinner,
} from '../ui'
import { isTerminalTask, taskStatusMeta, taskTypeLabel } from './shared'

export interface TaskListProps {
  tasks: TaskInfo[]
  loading?: boolean
  error?: string
  onRefresh?: () => void
}

/** 采集任务队列：store 事件驱动 + 手动刷新快照合并展示。 */
export function TaskList(props: TaskListProps): JSX.Element {
  const [stopping, setStopping] = createSignal<string[]>([])
  const activeCount = createMemo(
    () => props.tasks.filter((task) => !isTerminalTask(task.status)).length,
  )

  async function stopTask(task: TaskInfo) {
    if (isTerminalTask(task.status) || stopping().includes(task.id)) return
    setStopping((prev) => [...prev, task.id])
    try {
      await api.collect.stop(task.id)
      notify('info', '已发送停止请求', `任务 ${shortId(task.id)} 将在当前批次后停止。`)
      props.onRefresh?.()
    } catch (err) {
      notify('error', '停止任务失败', errorText(err))
    } finally {
      setStopping((prev) => prev.filter((id) => id !== task.id))
    }
  }

  const isStopping = (id: string) => stopping().includes(id)
  const percent = (task: TaskInfo) =>
    Math.round(Math.max(0, Math.min(1, task.progress || 0)) * 100)

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          任务队列
          <Show when={activeCount() > 0}>
            <Badge tone="amber">{activeCount()} 运行中</Badge>
          </Show>
        </div>
        <div class="row" style={{ gap: '4px' }}>
          <Show when={props.loading}>
            <Spinner />
          </Show>
          <IconButton
            icon="refresh"
            title="刷新任务"
            disabled={props.loading}
            onClick={() => props.onRefresh?.()}
          />
        </div>
      </div>

      <Show when={props.error}>
        <div style={{ padding: '10px 12px' }}>
          <div class="danger-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span class="mono-sm">{props.error}</span>
              <Button size="sm" variant="ghost" onClick={() => props.onRefresh?.()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show
        when={props.tasks.length > 0}
        fallback={
          <div class="panel-body tight">
            <EmptyState
              icon="activity"
              title="暂无采集任务"
              text="启动一次采集后，任务状态与进度会在此实时更新。"
            />
          </div>
        }
      >
        <div class="panel-body tight scrollable" style={{ 'max-height': '430px' }}>
          <For each={props.tasks}>
            {(task) => {
              const meta = () => taskStatusMeta(task.status)
              return (
                <div
                  class="stack"
                  style={{
                    gap: '6px',
                    padding: '10px 12px',
                    'border-bottom': '1px solid var(--border)',
                  }}
                >
                  <div class="row between wrap" style={{ gap: '6px' }}>
                    <div class="row" style={{ gap: '6px', 'min-width': 0 }}>
                      <Show when={meta().active}>
                        <Spinner />
                      </Show>
                      <Badge tone={meta().tone}>{meta().label}</Badge>
                      <span class="mono-sm faint truncate">
                        {taskTypeLabel(task.type)} · {shortId(task.id)}
                      </span>
                    </div>
                    <span class="mono-sm faint numeric">{percent(task)}%</span>
                  </div>

                  <div class="row between" style={{ gap: '8px' }}>
                    <span
                      class="mono-sm truncate"
                      title={task.message}
                      style={{ color: 'var(--text-muted)' }}
                    >
                      {task.message || '等待开始…'}
                    </span>
                    <span class="mono-sm faint numeric">
                      {formatInt(task.processed)} / {formatInt(task.total)}
                    </span>
                  </div>

                  <ProgressBar value={task.progress || 0} />

                  <div class="row between wrap" style={{ gap: '6px' }}>
                    <div class="row wrap" style={{ gap: '6px' }}>
                      <span class="mono-sm faint">{formatTime(task.started_at)}</span>
                      <Show when={task.finished_at}>
                        <span class="mono-sm faint">→ {formatTime(task.finished_at)}</span>
                      </Show>
                      <Show when={task.result}>
                        <span class="mono-sm faint">
                          解析 {formatInt(task.result?.log_count)} · 失败{' '}
                          {formatInt(task.result?.failed_count)}
                        </span>
                      </Show>
                    </div>
                    <Show when={!isTerminalTask(task.status)}>
                      <Button
                        size="sm"
                        variant="danger"
                        loading={isStopping(task.id)}
                        onClick={() => void stopTask(task)}
                      >
                        <Show when={!isStopping(task.id)}>
                          <Icon name="stop" size={10} />
                        </Show>
                        停止
                      </Button>
                    </Show>
                  </div>

                  <Show when={task.error}>
                    <div class="danger-note mono-sm">{task.error}</div>
                  </Show>
                </div>
              )
            }}
          </For>
        </div>
      </Show>
    </div>
  )
}
