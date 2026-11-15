import { JSX, Show, createSignal } from 'solid-js'
import type { ParseConfig, TaskInfo } from '../../types'
import { api, errorText } from '../../api'
import { systemInfo, notify } from '../../stores/app'
import { Button, Field, TextInput, Toggle } from '../ui'
import { TaskProgressInline } from './TaskProgressInline'

/** Linux 本机 systemd journal 采集。 */
export function JournalCollector(props: {
  projectId: string
  parse: () => ParseConfig
}): JSX.Element {
  const [since, setSince] = createSignal('1 hour ago')
  const [until, setUntil] = createSignal('now')
  const [unit, setUnit] = createSignal('')
  const [priority, setPriority] = createSignal('')
  const [lines, setLines] = createSignal('')
  const [follow, setFollow] = createSignal(false)
  const [busy, setBusy] = createSignal(false)
  const [taskId, setTaskId] = createSignal<string | undefined>()

  const supported = () => (systemInfo()?.os ?? '').toLowerCase() === 'linux'

  async function start() {
    if (!supported()) {
      notify('warn', '当前系统不支持 journalctl', 'systemd 日志仅在 Linux 上可用。')
      return
    }
    setBusy(true)
    try {
      const lineCount = Number(lines())
      const task: TaskInfo = await api.collect.startJournal({
        project_id: props.projectId,
        since: since().trim() || undefined,
        until: until().trim() || undefined,
        unit: unit().trim() || undefined,
        priority: priority().trim() || undefined,
        lines: Number.isFinite(lineCount) && lineCount > 0 ? Math.floor(lineCount) : undefined,
        follow: follow(),
        parse: props.parse(),
      })
      setTaskId(task.id)
      notify('success', '已启动 journald 采集', follow() ? '实时跟踪中，可在任务队列停止。' : '任务已进入队列。')
    } catch (err) {
      notify('error', '启动 journald 采集失败', errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '12px' }}>
      <Show when={!supported()}>
        <div class="detail-note">
          当前运行环境不是 Linux，本机 journalctl 不可用。可在 Linux 主机上使用本页签，或通过 SSH 采集页签连接远程主机。
        </div>
      </Show>

      <div class="form-row">
        <Field label="Since" hint="journalctl --since">
          <TextInput value={since()} mono onInput={setSince} placeholder="1 hour ago / 2024-10-10 00:00:00" />
        </Field>
        <Field label="Until" hint="journalctl --until">
          <TextInput value={until()} mono onInput={setUntil} placeholder="now" />
        </Field>
      </div>

      <div class="form-row">
        <Field label="Unit" hint="可选，如 nginx.service">
          <TextInput value={unit()} mono onInput={setUnit} placeholder="nginx.service" />
        </Field>
        <Field label="Priority" hint="可选：err / warning / info / debug">
          <TextInput value={priority()} mono onInput={setPriority} placeholder="err" />
        </Field>
        <Field label="最近行数" hint="可选；留空表示不限制">
          <TextInput value={lines()} type="number" onInput={setLines} placeholder="1000" />
        </Field>
      </div>

      <div class="row wrap" style={{ gap: '12px' }}>
        <div class="row" style={{ gap: '8px' }}>
          <Toggle checked={follow()} label="实时跟踪" onChange={setFollow} />
          <span class="mono-sm faint">实时跟踪（--follow）</span>
        </div>
        <div class="grow" />
        <Button variant="primary" loading={busy()} disabled={!supported()} onClick={() => void start()}>
          开始采集 journald
        </Button>
      </div>

      <Show when={taskId()}>
        {(id) => <TaskProgressInline taskId={id()} label="systemd journal" />}
      </Show>

      <div class="detail-note">
        journalctl 以 JSON 输出采集；应用内置 journald-json 模板解析 MESSAGE、PRIORITY（0-7 映射级别）与微秒时间戳。
        `--follow` 任务会持续运行，可在右侧任务队列中停止。
      </div>
    </div>
  )
}
