import type { JSX } from 'solid-js'
import { Show } from 'solid-js'
import { errorText } from '../../api'
import { notify } from '../../stores/app'
import { copyText } from '../../utils'
import { IconButton } from '../ui'

/** 设置页分组面板：统一的 panel 标题、说明与操作区。 */
export function SettingsPanel(props: {
  title: string
  note?: string
  actions?: JSX.Element
  children: JSX.Element
  class?: string
}): JSX.Element {
  return (
    <section class={`panel ${props.class ?? ''}`}>
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          {props.title}
        </div>
        <Show when={props.actions}>
          <div class="row" style={{ gap: '6px' }}>
            {props.actions}
          </div>
        </Show>
      </div>
      <div class="panel-body stack">
        <Show when={props.note}>
          <p class="mono-sm faint" style={{ 'line-height': '1.7' }}>
            {props.note}
          </p>
        </Show>
        {props.children}
      </div>
    </section>
  )
}

/** 深拷贝 JSON 安全对象（设置表单编辑用，避免直接修改 store 中的引用）。 */
export function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

/** 等宽值 + 复制按钮。 */
export function CopyableValue(props: {
  value?: string | null
  label?: string
}): JSX.Element {
  const text = () => props.value ?? ''
  const copy = async (): Promise<void> => {
    if (!text()) return
    try {
      await copyText(text())
      notify('success', '已复制到剪贴板', props.label ?? '内容')
    } catch (err) {
      notify('error', '复制失败', errorText(err))
    }
  }
  return (
    <div class="row" style={{ gap: '4px', 'min-width': 0 }}>
      <span class="grow truncate mono-sm" title={text()}>
        {text() || '—'}
      </span>
      <IconButton
        icon="copy"
        title={`复制${props.label ?? ''}`}
        disabled={!text()}
        onClick={() => void copy()}
      />
    </div>
  )
}

/** 如果当前值不在选项列表中，将其补入，避免下拉框显示为空。 */
export function withCurrentOption(
  options: Array<{ value: string; label: string }>,
  current: string | undefined,
  labelPrefix = '',
): Array<{ value: string; label: string }> {
  const value = current ?? ''
  if (!value || options.some((opt) => opt.value === value)) return options
  return [{ value, label: `${labelPrefix}${value}` }, ...options]
}

export type JsonObjectResult =
  | { ok: true; value: Record<string, unknown> }
  | { ok: false; error: string }

/** 解析 JSON 对象；空文本视为空对象。 */
export function parseJsonObject(text: string, label: string): JsonObjectResult {
  const raw = text.trim()
  if (!raw) return { ok: true, value: {} }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (err) {
    return { ok: false, error: `${label} 不是有效 JSON：${errorText(err)}` }
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { ok: false, error: `${label} 必须是 JSON 对象` }
  }
  return { ok: true, value: parsed as Record<string, unknown> }
}

export type StringMapResult =
  | { ok: true; value: Record<string, string> }
  | { ok: false; error: string }

/** 解析 string -> string 映射（AI parameters、level_map 等）。 */
export function parseStringMap(text: string, label: string): StringMapResult {
  const base = parseJsonObject(text, label)
  if (!base.ok) return { ok: false, error: base.error }
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(base.value)) {
    if (value == null || typeof value === 'object') {
      return { ok: false, error: `${label}.${key} 必须是字符串` }
    }
    out[key] = String(value)
  }
  return { ok: true, value: out }
}

export type NumberMapResult =
  | { ok: true; value: Record<string, number> }
  | { ok: false; error: string }

/** 解析 string -> 非负整数映射（模板 field_map）。 */
export function parseNumberMap(text: string, label: string): NumberMapResult {
  const base = parseJsonObject(text, label)
  if (!base.ok) return { ok: false, error: base.error }
  const out: Record<string, number> = {}
  for (const [key, value] of Object.entries(base.value)) {
    const n = typeof value === 'number' ? value : Number.parseFloat(String(value))
    if (!Number.isFinite(n) || !Number.isInteger(n) || n < 0) {
      return { ok: false, error: `${label}.${key} 必须是非负整数` }
    }
    out[key] = n
  }
  return { ok: true, value: out }
}

/** 按行拆分样本文本；保留行内空格，过滤纯空行。 */
export function splitLines(text: string): string[] {
  return text.split(/\r?\n/).filter((line) => line.trim().length > 0)
}

/** 将输入值转换为整数；无法解析时返回 0，交由表单校验提示。 */
export function toInt(value: string): number {
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) ? n : 0
}
