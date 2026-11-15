import { For, Show, createMemo, createSignal, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { ParseConfig, WindowsEventLogRequest } from '../../types'
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Icon,
  SectionLabel,
  TextInput,
} from '../ui'
import { TaskProgressInline } from './TaskProgressInline'
import {
  datetimeLocalToISO,
  eventChannels,
  eventLevels,
  isInvalidDateTimeLocal,
  parseEventIds,
  parseModeLabel,
} from './shared'

export interface EVTXCollectorProps {
  projectId: string
  parse: () => ParseConfig
  windows: boolean
}

const DEFAULT_CHANNELS = ['System', 'Application']

/** Windows 事件日志通道采集配置。 */
export function EVTXCollector(props: EVTXCollectorProps): JSX.Element {
  const [channels, setChannels] = createSignal<string[]>(DEFAULT_CHANNELS)
  const [eventIDText, setEventIDText] = createSignal('')
  const [levels, setLevels] = createSignal<string[]>([])
  const [startText, setStartText] = createSignal('')
  const [endText, setEndText] = createSignal('')
  const [starting, setStarting] = createSignal(false)
  const [lastTaskId, setLastTaskId] = createSignal<string | undefined>(undefined)

  const selectedCount = createMemo(() => channels().length)

  function toggleChannel(value: string, checked: boolean) {
    setChannels((prev) => {
      if (checked) return prev.includes(value) ? prev : [...prev, value]
      return prev.filter((item) => item !== value)
    })
  }

  function toggleLevel(level: string) {
    setLevels((prev) =>
      prev.includes(level) ? prev.filter((item) => item !== level) : [...prev, level],
    )
  }

  async function start() {
    if (!props.projectId) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return
    }
    const selectedChannels = channels()
    if (!selectedChannels.length) {
      notify('warn', '未选择事件通道', '请至少选择一个 Windows 事件日志通道。')
      return
    }
    if (isInvalidDateTimeLocal(startText()) || isInvalidDateTimeLocal(endText())) {
      notify('error', '时间格式无效', '请检查开始时间与结束时间。')
      return
    }
    const startTime = datetimeLocalToISO(startText())
    const endTime = datetimeLocalToISO(endText())
    if (startTime && endTime && new Date(startTime).getTime() > new Date(endTime).getTime()) {
      notify('error', '时间范围无效', '开始时间不能晚于结束时间。')
      return
    }

    const parsedIDs = parseEventIds(eventIDText())
    if (parsedIDs.invalid.length > 0) {
      notify('warn', '已忽略无效事件 ID', parsedIDs.invalid.join(', '))
    }

    setStarting(true)
    try {
      const request: WindowsEventLogRequest = {
        project_id: props.projectId,
        channels: selectedChannels,
        start_time: startTime,
        end_time: endTime,
        event_ids: parsedIDs.ids.length > 0 ? parsedIDs.ids : undefined,
        levels: levels().length > 0 ? levels() : undefined,
        parse: props.parse(),
      }
      const task = await api.collect.startWindowsEvent(request)
      if (task?.id) setLastTaskId(task.id)
      notify(
        'success',
        'Windows 事件采集已启动',
        task?.message || `任务 ${task?.id ?? ''} 已提交，进度见右侧任务队列。`,
      )
    } catch (err) {
      notify('error', '启动 Windows 事件采集失败', errorText(err))
    } finally {
      setStarting(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '14px' }}>
      <Show when={!props.windows}>
        <div class="detail-note">
          当前系统不是 Windows。Windows 事件通道采集需要 Windows 主机；EVTX 文件可在“文件导入”中跨平台解析。
        </div>
      </Show>

      <div class="detail-note">
        Security 与 Forwarded Events 通道通常需要管理员权限；读取 Security.evtx 时请以管理员身份运行 GLog。
      </div>

      <div>
        <SectionLabel>事件通道</SectionLabel>
        <div class="stack" style={{ gap: '6px' }}>
          <For each={eventChannels}>
            {(channel) => (
              <div
                class="row between"
                style={{
                  gap: '8px',
                  padding: '5px 0',
                  'border-bottom': '1px solid var(--border)',
                }}
              >
                <Checkbox
                  checked={channels().includes(channel.value)}
                  label={`${channel.label} · ${channel.description}`}
                  onChange={(checked) => toggleChannel(channel.value, checked)}
                />
                <Show when={channel.admin}>
                  <Badge tone="amber">需管理员</Badge>
                </Show>
              </div>
            )}
          </For>
        </div>
      </div>

      <Field label="事件 ID 过滤" hint="逗号分隔 · 留空表示全部">
        <TextInput
          mono
          value={eventIDText()}
          placeholder="如 4624, 4625, 7045"
          onInput={(value) => setEventIDText(value)}
        />
      </Field>

      <Field label="事件级别" hint="不选表示全部">
        <div class="row wrap" style={{ gap: '5px' }}>
          <For each={eventLevels}>
            {(level) => (
              <button
                type="button"
                class={`chip ${levels().includes(level) ? 'selected' : ''}`}
                onClick={() => toggleLevel(level)}
              >
                {level}
              </button>
            )}
          </For>
        </div>
      </Field>

      <div class="form-row">
        <Field label="开始时间" hint="按本地时区解释">
          <TextInput
            type="datetime-local"
            value={startText()}
            onInput={(value) => setStartText(value)}
          />
        </Field>
        <Field label="结束时间" hint="按本地时区解释">
          <TextInput
            type="datetime-local"
            value={endText()}
            onInput={(value) => setEndText(value)}
          />
        </Field>
      </div>

      <div class="row between wrap" style={{ gap: '10px' }}>
        <span class="mono-sm faint">
          通道 {selectedCount()} 个 · 级别 {levels().length || '全部'} · 解析{' '}
          {parseModeLabel(props.parse().mode)}
        </span>
        <Button
          variant="primary"
          loading={starting()}
          disabled={selectedCount() === 0}
          onClick={() => void start()}
        >
          <Show when={!starting()}>
            <Icon name="play" size={12} />
          </Show>
          开始采集
        </Button>
      </div>

      <Show when={lastTaskId()}>
        <TaskProgressInline taskId={lastTaskId()} label="Windows 事件采集" />
      </Show>
    </div>
  )
}
