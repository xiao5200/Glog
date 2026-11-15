import type { ParseConfig, ParseTemplate, TaskInfo } from '../../types'

export type ParseTabKey = 'local' | 'evtx' | 'journal' | 'ssh' | 'import'
export type ParseMode = 'builtin' | 'ai' | 'native_evtx'
export type BadgeTone = 'mint' | 'amber' | 'ember' | 'sky' | 'violet' | 'muted'

export interface ParseState {
  mode: ParseMode
  template_id: string
  ai_provider: string
  save_template: boolean
  sample_size: number
  samples: string
  ai_template_name: string
  ai_category: string
}

/** 各采集 Tab 共用的解析配置初始值。 */
export function createParseState(): ParseState {
  return {
    mode: 'builtin',
    template_id: '',
    ai_provider: '',
    save_template: true,
    sample_size: 50,
    samples: '',
    ai_template_name: '',
    ai_category: 'custom',
  }
}

/** 将界面解析状态转换为后端 ParseConfig，native_evtx 时剥离无关字段。 */
export function buildParseConfig(state: ParseState, modeOverride?: ParseMode): ParseConfig {
  const mode = modeOverride ?? state.mode
  if (mode === 'native_evtx') return { mode }

  const config: ParseConfig = { mode }
  const templateID = state.template_id.trim()
  if (templateID) config.template_id = templateID

  if (mode === 'ai') {
    const provider = state.ai_provider.trim()
    if (provider) config.ai_provider = provider
    config.save_template = state.save_template
    const size = Number(state.sample_size)
    if (Number.isFinite(size) && size > 0) config.sample_size = Math.floor(size)
  }
  return config
}

export function parseModeOptionsForTab(
  tab: ParseTabKey,
): Array<{ value: ParseMode; label: string }> {
  if (tab === 'evtx') {
    return [
      { value: 'native_evtx', label: '原生 EVTX' },
      { value: 'builtin', label: '内置模板' },
    ]
  }
  return [
    { value: 'builtin', label: '内置模板' },
    { value: 'ai', label: 'AI 解析' },
  ]
}

export function defaultModeForTab(tab: ParseTabKey): ParseMode {
  return tab === 'evtx' ? 'native_evtx' : 'builtin'
}

/** 本地路径：换行或分号分隔，保留通配符。 */
export function splitLocalPaths(text: string): string[] {
  return String(text ?? '')
    .split(/[\n;]+/)
    .map((value) => value.trim())
    .filter(Boolean)
}

/** SSH 远程路径：换行或逗号分隔。 */
export function splitRemotePaths(text: string): string[] {
  return String(text ?? '')
    .split(/[\n,]+/)
    .map((value) => value.trim())
    .filter(Boolean)
}

/** 每行一条样本，保留行内原始空白。 */
export function splitSamples(text: string): string[] {
  return String(text ?? '')
    .split(/\r?\n/)
    .map((line) => line.replace(/\r$/, ''))
    .filter((line) => line.trim().length > 0)
}

export function parseEventIds(text: string): { ids: number[]; invalid: string[] } {
  const tokens = String(text ?? '')
    .split(/[,，\s]+/)
    .map((value) => value.trim())
    .filter(Boolean)
  const ids: number[] = []
  const invalid: string[] = []
  for (const token of tokens) {
    const value = Number(token)
    if (Number.isInteger(value) && value > 0 && value <= 4294967295) {
      if (!ids.includes(value)) ids.push(value)
    } else {
      invalid.push(token)
    }
  }
  return { ids, invalid }
}

/** datetime-local 按浏览器本地时区解释，再转为 ISO。空值或非法值返回 undefined。 */
export function datetimeLocalToISO(value: string): string | undefined {
  const input = String(value ?? '').trim()
  if (!input) return undefined
  const date = new Date(input)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

export function isInvalidDateTimeLocal(value: string): boolean {
  const input = String(value ?? '').trim()
  if (!input) return false
  return Number.isNaN(new Date(input).getTime())
}

/** 后端成功率/置信度按 0-1 返回，这里兼容 0-100。 */
export function ratePercent(value: number | undefined | null): number {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return 0
  const percent = n <= 1 ? n * 100 : n
  return Math.max(0, Math.min(100, percent))
}

export function rateRatio(value: number | undefined | null): number {
  return ratePercent(value) / 100
}

const CATEGORY_LABELS: Record<string, string> = {
  web_server: 'Web 服务器',
  webserver: 'Web 服务器',
  database: '数据库',
  middleware: '中间件',
  container: '容器',
  system: '系统',
  security: '安全',
  application: '应用',
  app: '应用',
  network: '网络设备',
  network_device: '网络设备',
  builtin: '内置',
  custom: '自定义',
  ai_generated: 'AI 生成',
  ai: 'AI 生成',
  other: '其他',
}

export function categoryLabel(value: string | undefined | null): string {
  const key = String(value ?? '').trim()
  if (!key) return '其他'
  return CATEGORY_LABELS[key.toLowerCase()] ?? key
}

export interface TemplateGroup {
  key: string
  label: string
  items: ParseTemplate[]
}

/** 按 category（缺省时按 type）分组模板。 */
export function groupTemplates(templates: ParseTemplate[]): TemplateGroup[] {
  const groups = new Map<string, ParseTemplate[]>()
  for (const template of templates) {
    const key = (template.category || template.type || 'other').trim() || 'other'
    const list = groups.get(key)
    if (list) list.push(template)
    else groups.set(key, [template])
  }
  return Array.from(groups.entries())
    .map(([key, items]) => ({
      key,
      label: categoryLabel(key),
      items: items.slice().sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')),
    }))
    .sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'))
}

export interface SelectOption {
  value: string
  label: string
}

export interface StrategyOption extends SelectOption {
  description: string
}

export const eventChannels: Array<{
  value: string
  label: string
  description: string
  admin?: boolean
}> = [
  { value: 'System', label: 'System', description: '系统日志' },
  { value: 'Application', label: 'Application', description: '应用程序日志' },
  {
    value: 'Security',
    label: 'Security',
    description: '安全日志 · 需要管理员权限',
    admin: true,
  },
  { value: 'Setup', label: 'Setup', description: '安装日志' },
  { value: 'Forwarded Events', label: 'Forwarded Events', description: '转发事件' },
]

export const eventLevels = ['Information', 'Warning', 'Error', 'Critical'] as const

export const sshStrategies: StrategyOption[] = [
  {
    value: 'compress',
    label: '压缩传输 compress',
    description: '在远端打包压缩后传输，适合目录与多文件批量采集。',
  },
  {
    value: 'filter',
    label: '正则过滤 filter',
    description: '在远端用 grep 正则过滤，仅下载匹配的日志行。',
  },
  {
    value: 'journalctl',
    label: 'systemd 日志 journalctl',
    description: '通过 journalctl 导出 systemd 日志，支持 since / until / unit。',
  },
  {
    value: 'incremental',
    label: '增量拉取 incremental',
    description: '记录偏移量，仅拉取上次同步后新增的内容。',
  },
  {
    value: 'plain',
    label: '直接读取 plain',
    description: '直接流式读取远程文本文件，适合少量小文件。',
  },
]

export const sshCompressions: SelectOption[] = [
  { value: 'gzip', label: 'gzip' },
  { value: 'zstd', label: 'zstd' },
  { value: 'none', label: 'none（不压缩）' },
]

export function sourceTypeLabel(type: string | undefined | null): string {
  const key = String(type ?? '').toLowerCase()
  switch (key) {
    case 'local':
    case 'file':
      return '本地文件'
    case 'directory':
      return '本地目录'
    case 'import':
      return '文件导入'
    case 'ssh':
      return 'SSH 远程'
    case 'evtx':
    case 'windows_event':
    case 'windows':
      return 'Windows 事件'
    default:
      return type || '未知来源'
  }
}

export function sourceTypeTone(type: string | undefined | null): BadgeTone {
  const key = String(type ?? '').toLowerCase()
  if (key === 'ssh') return 'violet'
  if (key === 'import') return 'sky'
  if (key === 'evtx' || key === 'windows_event' || key === 'windows') return 'amber'
  if (key === 'local' || key === 'file' || key === 'directory') return 'mint'
  return 'muted'
}

export function parseModeLabel(mode: string | undefined | null): string {
  switch (String(mode ?? '').toLowerCase()) {
    case 'builtin':
      return '内置模板'
    case 'ai':
      return 'AI 解析'
    case 'native_evtx':
      return '原生 EVTX'
    default:
      return mode || '未指定'
  }
}

export function taskTypeLabel(type: string | undefined | null): string {
  const key = String(type ?? '').toLowerCase()
  switch (key) {
    case 'local':
      return '本地文件'
    case 'ssh':
      return 'SSH 远程'
    case 'evtx':
    case 'windows_event':
    case 'windows':
      return 'Windows 事件'
    case 'import':
      return '文件导入'
    case 'analysis':
      return '分析任务'
    case 'rule':
      return '规则检测'
    default:
      return type || '采集任务'
  }
}

export function isTerminalTask(status: string | undefined | null): boolean {
  return status !== 'pending' && status !== 'running'
}

export function taskStatusMeta(status: string | undefined | null): {
  label: string
  tone: BadgeTone
  active: boolean
} {
  switch (String(status ?? '').toLowerCase()) {
    case 'pending':
      return { label: '等待中', tone: 'muted', active: true }
    case 'running':
      return { label: '运行中', tone: 'amber', active: true }
    case 'success':
      return { label: '完成', tone: 'mint', active: false }
    case 'failed':
      return { label: '失败', tone: 'ember', active: false }
    case 'stopped':
      return { label: '已停止', tone: 'muted', active: false }
    default:
      return { label: status || '未知', tone: 'sky', active: false }
  }
}

/**
 * 合并 store 中的事件驱动任务与手动刷新拿到的快照。
 * 远端已终态而本地仍在运行的任务以远端为准，其余以 store 实时状态为准。
 */
export function mergeTaskLists(live: TaskInfo[], remote: TaskInfo[]): TaskInfo[] {
  const merged = new Map<string, TaskInfo>()
  for (const task of remote) {
    if (task?.id) merged.set(task.id, task)
  }
  for (const task of live) {
    if (!task?.id) continue
    const snapshot = merged.get(task.id)
    if (!snapshot || !isTerminalTask(snapshot.status) || isTerminalTask(task.status)) {
      merged.set(task.id, task)
    }
  }
  return Array.from(merged.values()).sort((a, b) => {
    const activeA = isTerminalTask(a.status) ? 1 : 0
    const activeB = isTerminalTask(b.status) ? 1 : 0
    if (activeA !== activeB) return activeA - activeB
    const timeA = Date.parse(a.started_at || '')
    const timeB = Date.parse(b.started_at || '')
    return (Number.isNaN(timeB) ? 0 : timeB) - (Number.isNaN(timeA) ? 0 : timeA)
  })
}

export function fileBaseName(path: string | undefined | null): string {
  const normalized = String(path ?? '').replace(/\\/g, '/')
  const index = normalized.lastIndexOf('/')
  return index >= 0 ? normalized.slice(index + 1) : normalized
}

export function fileParentDir(path: string | undefined | null): string {
  const value = String(path ?? '')
  const index = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'))
  return index > 0 ? value.slice(0, index) : ''
}

export function isWindowsPlatform(os: string | undefined | null): boolean {
  const value = String(os ?? '').toLowerCase()
  if (value) return value.includes('windows')
  if (typeof navigator === 'undefined') return false
  return navigator.userAgent.toLowerCase().includes('windows')
}
