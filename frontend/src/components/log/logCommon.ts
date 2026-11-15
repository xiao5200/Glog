// 日志分析工作台的共享类型与小工具。
// 仅包含纯函数与常量，避免组件之间产生循环依赖。

export type TimePreset = 'all' | '15m' | '1h' | '24h' | '7d' | 'custom'

export type SortField = 'timestamp' | 'level'

export type AIMode = 'anomaly' | 'nlquery' | 'rootcause' | 'report'

export interface HostFacet {
  host: string
  count: number
}

export const LOG_LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR', 'CRITICAL'] as const

export const TIME_PRESETS: ReadonlyArray<{ value: TimePreset; label: string; title: string }> = [
  { value: 'all', label: '全部', title: '不限制时间范围' },
  { value: '15m', label: '15m', title: '最近 15 分钟' },
  { value: '1h', label: '1h', title: '最近 1 小时' },
  { value: '24h', label: '24h', title: '最近 24 小时' },
  { value: '7d', label: '7d', title: '最近 7 天' },
  { value: 'custom', label: '自定义', title: '自定义本地时间范围' },
]

const PRESET_MS: Record<'15m' | '1h' | '24h' | '7d', number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
}

export function toggleValue(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((item) => item !== value) : [...list, value]
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

/** datetime-local 输入值（本地时间）转换为 ISO 字符串。 */
export function localInputToIso(value: string): string | undefined {
  if (!value) return undefined
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

/** ISO 时间字符串转换为 datetime-local 输入值（本地时间）。 */
export function toLocalInput(value: string | undefined | null): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/**
 * 将时间快捷选项解析为 ISO 起止时间。快捷范围以 anchor（查询锚点）为终点，
 * 由页面控制锚点，避免每次构建查询都产生新的时间范围。
 */
export function resolveTimeBounds(
  preset: TimePreset,
  customStart: string,
  customEnd: string,
  anchor = Date.now(),
): { start?: string; end?: string } {
  if (preset === 'all') return {}
  if (preset === 'custom') {
    return {
      start: localInputToIso(customStart),
      end: localInputToIso(customEnd),
    }
  }
  const now = Number.isFinite(anchor) ? anchor : Date.now()
  return {
    start: new Date(now - PRESET_MS[preset]).toISOString(),
    end: new Date(now).toISOString(),
  }
}

/** 解析事件 ID 输入，支持逗号、空格、分号、顿号分隔。 */
export function parseEventIDs(text: string): number[] {
  if (!text.trim()) return []
  const seen = new Set<number>()
  const ids: number[] = []
  for (const token of text.split(/[\s,;，、]+/)) {
    if (!token) continue
    const n = Number(token)
    if (!Number.isFinite(n) || !Number.isInteger(n) || n < 0) continue
    if (seen.has(n)) continue
    seen.add(n)
    ids.push(n)
  }
  return ids
}

export function basename(path: string | undefined | null): string {
  const value = String(path ?? '')
  if (!value) return ''
  const parts = value.split(/[\\/]/)
  return parts[parts.length - 1] || value
}

/** 部分后端百分比字段可能返回 0-1，也可能返回 0-100。 */
export function percentDisplay(value: number | undefined | null, digits = 1): string {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return '0%'
  const pct = n > 0 && n <= 1 ? n * 100 : n
  return `${pct.toFixed(digits)}%`
}

/** 将置信度/百分比归一化为 0-1。 */
export function normalizedFraction(value: number | undefined | null): number {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return 0
  const fraction = n > 1 ? n / 100 : n
  return Math.max(0, Math.min(1, fraction))
}

export function parseMaxLogs(value: string | number, fallback = 100): number {
  const n = Math.floor(Number(value))
  if (!Number.isFinite(n) || n <= 0) return fallback
  return Math.min(1000, n)
}

/** 用于判断查询快照是否与当前条件一致（忽略分页与排序）。 */
export function queryComparableKey(query: {
  text?: string
  start_time?: string
  end_time?: string
  levels?: string[]
  hosts?: string[]
  sources?: string[]
  event_ids?: number[]
  field_filters?: unknown[]
}): string {
  const normalizeIds = (ids?: number[]) => [...(ids ?? [])].sort((a, b) => a - b)
  const normalizeStrings = (values?: string[]) => [...(values ?? [])].sort()
  return JSON.stringify({
    text: query.text ?? '',
    start: query.start_time ?? '',
    end: query.end_time ?? '',
    levels: normalizeStrings(query.levels),
    hosts: normalizeStrings(query.hosts),
    sources: normalizeStrings(query.sources),
    ids: normalizeIds(query.event_ids),
    fields: query.field_filters ?? [],
  })
}
