import { Show, createMemo, type JSX } from 'solid-js'
import { tasks } from '../../stores/app'
import { formatInt, shortId } from '../../utils'
import { Badge, ProgressBar, Spinner } from '../ui'
import { taskStatusMeta } from './shared'

/** 当前 Tab 最近启动任务的紧凑进度条，数据来自 store 的 tasks 实时更新。 */
export function TaskProgressInline(props: {
  taskId?: string
  label?: string
}): JSX.Element {
  const task = createMemo(() => {
    const id = props.taskId
    if (!id) return undefined
    return tasks().find((item) => item.id === id)
  })

  return (
    <Show when={task()}>
      {(current) => {
        const meta = () => taskStatusMeta(current().status)
        return (
          <div
            class="stack"
            style={{
              gap: '6px',
              padding: '10px 12px',
              border: '1px solid var(--border)',
              background: 'var(--surface-soft)',
            }}
          >
            <div class="row between wrap" style={{ gap: '6px' }}>
              <div class="row" style={{ gap: '6px', 'min-width': 0 }}>
                <Show when={meta().active}>
                  <Spinner />
                </Show>
                <span class="mono-sm faint">
                  {props.label ?? '当前任务'} · {shortId(current().id)}
                </span>
                <Badge tone={meta().tone}>{meta().label}</Badge>
              </div>
              <span class="mono-sm numeric accent">
                {Math.round(Math.max(0, Math.min(1, current().progress || 0)) * 100)}%
              </span>
            </div>
            <ProgressBar value={current().progress || 0} />
            <div class="row between" style={{ gap: '8px' }}>
              <span class="mono-sm faint truncate" title={current().message}>
                {current().message || '等待后端进度…'}
              </span>
              <span class="mono-sm faint numeric">
                {formatInt(current().processed)} / {formatInt(current().total)}
              </span>
            </div>
            <Show when={current().error}>
              <div class="danger-note mono-sm">{current().error}</div>
            </Show>
          </div>
        )
      }}
    </Show>
  )
}
