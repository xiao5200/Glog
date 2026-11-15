export function formatBytes(bytes: number | undefined | null, digits = 1): string {
  const n = Number(bytes ?? 0)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let value = n
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx++
  }
  return `${value.toFixed(idx === 0 ? 0 : digits)} ${units[idx]}`
}

export function formatNumber(n: number | undefined | null): string {
  const v = Number(n ?? 0)
  if (!Number.isFinite(v)) return '0'
  if (Math.abs(v) >= 1_000_000_000) return `${(v / 1_000_000_000).toFixed(2)}B`
  if (Math.abs(v) >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`
  if (Math.abs(v) >= 10_000) return `${(v / 1_000).toFixed(1)}K`
  return v.toLocaleString('zh-CN')
}

export function formatInt(n: number | undefined | null): string {
  return Number(n ?? 0).toLocaleString('zh-CN')
}

export function parseTime(value: string | undefined | null): Date | null {
  if (!value) return null
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

export function formatTime(value: string | undefined | null, withDate = true): string {
  const d = parseTime(value)
  if (!d) return '—'
  const pad = (v: number) => String(v).padStart(2, '0')
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  if (!withDate) return time
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${time}`
}

export function formatTimeShort(value: string | undefined | null): string {
  const d = parseTime(value)
  if (!d) return '—'
  const pad = (v: number) => String(v).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function formatDuration(ms: number | undefined | null): string {
  const n = Number(ms ?? 0)
  if (n < 1000) return `${Math.max(0, Math.round(n))}ms`
  if (n < 60_000) return `${(n / 1000).toFixed(2)}s`
  const minutes = Math.floor(n / 60_000)
  const seconds = Math.round((n % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

export function formatRange(start?: string, end?: string): string {
  // 后端允许只设置时间范围的一端；未设置端会序列化为 0001 年零值时间。
  const valid = (value?: string) => {
    if (!value) return undefined
    const date = parseTime(value)
    return date && date.getFullYear() >= 1970 ? value : undefined
  }
  const from = valid(start)
  const to = valid(end)
  if (!start && !end) return '时间范围未知'
  if (!from && !to) return '不限'
  return `${from ? formatTime(from) : '不限'} → ${to ? formatTime(to) : '不限'}`
}

export function formatPercent(value: number | undefined | null, digits = 1): string {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return '0%'
  return `${n.toFixed(digits)}%`
}

export function truncate(value: string | undefined | null, max = 160): string {
  const s = String(value ?? '')
  return s.length > max ? `${s.slice(0, max)}…` : s
}

export function shortId(id: string | undefined | null, len = 8): string {
  const s = String(id ?? '')
  return s.length > len ? s.slice(0, len) : s
}

export function joinPath(base: string, name: string): string {
  if (!base) return name
  const sep = base.includes('\\') ? '\\' : '/'
  return `${base.replace(/[\\/]+$/, '')}${sep}${name}`
}

export function todayStamp(): string {
  const d = new Date()
  const pad = (v: number) => String(v).padStart(2, '0')
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`
}

export function levelTone(level: string | undefined): string {
  const l = String(level ?? 'UNKNOWN').toUpperCase()
  if (l === 'ERROR' || l === 'CRITICAL' || l === 'FATAL') return 'ember'
  if (l === 'WARN') return 'amber'
  if (l === 'INFO') return 'sky'
  if (l === 'DEBUG') return 'muted'
  return 'muted'
}

export function severityTone(severity: string | undefined): 'ember' | 'amber' | 'sky' | 'mint' {
  const s = String(severity ?? 'medium').toLowerCase()
  if (s === 'critical' || s === 'high') return 'ember'
  if (s === 'medium' || s === 'warn') return 'amber'
  if (s === 'low') return 'sky'
  return 'mint'
}

export function riskTone(score: number | undefined): 'ember' | 'amber' | 'sky' | 'mint' {
  const s = Number(score ?? 0)
  if (s >= 80) return 'ember'
  if (s >= 50) return 'amber'
  if (s >= 25) return 'sky'
  return 'mint'
}

export function percentWidth(count: number, total: number): string {
  if (!total) return '0%'
  return `${Math.max(0, Math.min(100, (count / total) * 100))}%`
}

export function sum(values: Record<string, number> | undefined): number {
  if (!values) return 0
  return Object.values(values).reduce((acc, v) => acc + (Number(v) || 0), 0)
}

export function debounce<T extends (...args: never[]) => void>(fn: T, delay = 300): T {
  let timer: number | undefined
  return ((...args: never[]) => {
    if (timer) window.clearTimeout(timer)
    timer = window.setTimeout(() => fn(...args), delay)
  }) as T
}

export function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text)
  const ta = document.createElement('textarea')
  ta.value = text
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  return Promise.resolve()
}

export function safeJson(value: unknown, max = 1200): string {
  try {
    const text = typeof value === 'string' ? value : JSON.stringify(value, null, 2)
    return text.length > max ? `${text.slice(0, max)}…` : text
  } catch {
    return String(value)
  }
}

export function fieldValue(entry: { fields?: Record<string, unknown> }, path: string): unknown {
  if (!entry?.fields) return undefined
  const parts = path.split('.')
  let cur: unknown = entry.fields
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[part]
  }
  return cur
}

/* ------------------------------------------------------------------ */
/* 字段名中文化                                                         */
/* ------------------------------------------------------------------ */

/**
 * 常见日志字段的中文名称。键使用「去分隔符 + 小写」形式归一化，
 * 因此 event_id / EventID / event-id / "Event ID" 会命中同一项。
 */
const FIELD_LABELS: Record<string, string> = {
  event_id: '事件 ID',
  provider: '提供程序',
  computer: '计算机',
  user_id: '用户',
  channel: '通道',
  task: '任务',
  opcode: '操作码',
  keywords: '关键字',
  process_id: '进程 ID',
  thread_id: '线程 ID',
  logon_type: '登录类型',
  target_user_name: '目标用户名',
  subject_user_name: '主体用户名',
  target_domain_name: '目标域名',
  subject_domain_name: '主体域名',
  ip_address: 'IP 地址',
  ip_port: 'IP 端口',
  process_name: '进程名称',
  command_line: '命令行',
  parent_process_name: '父进程名称',
  status: '状态',
  sub_status: '子状态',
  pid: '进程 ID',
  port: '端口',
  remote_addr: '远程地址',
  remote_user: '远程用户',
  user: '用户',
  host: '主机',
  source: '来源',
  message: '消息',
  timestamp: '时间',
  level: '级别',
  // 常见模型字段，便于详情页统一展示。
  doc_id: '文档 ID',
  source_file: '源文件',
  source_host: '来源主机',
  source_offset: '文件偏移',
  raw_log: '原始日志',
  risk_score: '风险评分',
  is_anomaly: '异常标记',
  is_evidence: '证据标记',
  tags: '标签',
  record_id: '记录 ID',
  event_record_id: '事件记录 ID',
  activity_id: '活动 ID',
  subject_logon_id: '主体登录 ID',
  target_logon_id: '目标登录 ID',
  logon_id: '登录 ID',
  session_id: '会话 ID',
  time_created: '创建时间',
  system_time: '系统时间',
  provider_name: '提供程序',
  channel_name: '通道',
  level_name: '级别',
  guid: 'GUID',
  name: '名称',
  package_identifier: '包标识',
  intended_package_state: '预期包状态',
  intended_package_state_textized: '包状态',
  error_code: '错误代码',
  client: '客户端',
  xmlns: '命名空间',
  version: '版本',
  normal_process_id: '正常进程 ID',
  trustlet_identity: '信任组件标识',
  subject_user_sid: '主体用户 SID',
  target_user_sid: '目标用户 SID',
  authentication_package_name: '认证包名称',
  workstation_name: '工作站名称',
  logon_process_name: '登录进程名称',
  key_length: '密钥长度',
  process_creation_time: '进程创建时间',
  new_process_name: '新进程名称',
  new_process_id: '新进程 ID',
  caller_process_id: '调用方进程 ID',
  caller_process_name: '调用方进程名称',
  target_image: '目标映像',
  granted_access: '授予访问权限',
  source_image: '来源映像',
  target_object: '目标对象',
  object_name: '对象名称',
  access_mask: '访问掩码',
  access_list: '访问列表',
  handle_id: '句柄 ID',
  token_elevation_type: '令牌提升类型',
  package_name: '包名称',
  target_session_id: '目标会话 ID',
  subject_session_id: '主体会话 ID',
  target_server_name: '目标服务器名称',
  service_name: '服务名称',
  service_file_name: '服务文件名',
  image_path: '映像路径',
  image: '映像',
  file_name: '文件名',
  file_path: '文件路径',
  destination_ip: '目标 IP',
  destination_port: '目标端口',
  source_ip: '来源 IP',
  source_port: '源端口',
  protocol: '协议',
  query_name: '查询名称',
  query_results: '查询结果',
  script_block_text: '脚本内容',
}

function normalizeFieldKey(key: string): string {
  return String(key ?? '')
    .trim()
    .toLowerCase()
    .replace(/[\s_\-./]+/g, '')
}

const FIELD_LABEL_LOOKUP = new Map<string, string>(
  Object.entries(FIELD_LABELS).map(([key, label]) => [normalizeFieldKey(key), label]),
)

/**
 * 返回字段名的中文显示名；未收录的字段原样返回。
 * 原始键仍应作为次要信息展示，便于对照查询语法与后端字段。
 */
export function fieldLabel(key: string): string {
  const raw = String(key ?? '').trim()
  if (!raw) return raw
  const direct = FIELD_LABEL_LOOKUP.get(normalizeFieldKey(raw))
  if (direct) return direct
  // EVTX 常见嵌套字段（如 System.EventID、System.Execution.ProcessID）：
  // 用最后一段再做一次匹配，仍保留原始路径作为次要信息。
  const segments = raw.split(/[./\\]+/).filter(Boolean)
  if (segments.length > 1) {
    const nested = FIELD_LABEL_LOOKUP.get(
      normalizeFieldKey(segments[segments.length - 1]),
    )
    if (nested) return nested
  }
  return raw
}

function stringifyFieldValue(value: unknown): string {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

/**
 * 将 Windows EVTX 的 System={...} 原始消息转换为中文字段摘要，
 * 避免日志列表直接显示 Channel / Computer / EventID 等英文键。
 * 非 Windows System 结构（例如业务 JSON 日志正文）保持原样。
 */
export function friendlyLogMessage(message: string): string {
  const raw = String(message ?? '').trim()
  if (!raw) return raw

  // EVTX 消息常见形式：System={...} UserData={...} EventData={...}，
  // 多个命名 JSON 段拼接，不能直接整体 JSON.parse。
  const segments = new Map<string, Record<string, unknown>>()
  const marker = /(?:^|\s)(System|UserData|EventData)=/g
  const matches = Array.from(raw.matchAll(marker))
  if (matches.length > 0) {
    for (let index = 0; index < matches.length; index++) {
      const match = matches[index]
      const start = (match.index ?? 0) + match[0].length
      const end = matches[index + 1]?.index ?? raw.length
      const jsonText = raw.slice(start, end).trim()
      try {
        const parsed = JSON.parse(jsonText)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          segments.set(match[1], parsed as Record<string, unknown>)
        }
      } catch {
        /* 单段解析失败时忽略该段 */
      }
    }
  } else {
    const candidate = raw.startsWith('{') && raw.endsWith('}') ? raw : ''
    if (candidate) {
      try {
        const parsed = JSON.parse(candidate)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          segments.set('System', parsed as Record<string, unknown>)
        }
      } catch {
        /* 非 JSON 消息保持原样 */
      }
    }
  }

  const system = segments.get('System') ?? null
  const eventData = segments.get('UserData') ?? segments.get('EventData') ?? null
  if (!system && !eventData) return raw

  const parts: string[] = []
  const push = (label: string, value: unknown) => {
    const text = stringifyFieldValue(value).trim()
    if (text) parts.push(`${label}：${text}`)
  }

  if (system) {
    push('通道', system.Channel)
    push('事件 ID', system.EventID)
    const provider = system.Provider
    if (provider && typeof provider === 'object') {
      push('提供程序', (provider as Record<string, unknown>).Name)
    } else {
      push('提供程序', provider)
    }
    push('计算机', system.Computer)
    const security = system.Security
    if (security && typeof security === 'object') {
      push('用户', (security as Record<string, unknown>).UserID)
    }
    push('记录 ID', system.EventRecordID)
  }

  if (eventData) {
    // EVTX UserData 通常只有一个事件专属对象（如 CbsPackageChangeState），
    // 展开一层后展示其字段，更易读。
    const entries = Object.entries(eventData)
    const flattened: Array<[string, unknown]> =
      entries.length === 1 &&
      entries[0][1] &&
      typeof entries[0][1] === 'object' &&
      !Array.isArray(entries[0][1])
        ? Object.entries(entries[0][1] as Record<string, unknown>)
        : entries
    for (const [key, value] of flattened.slice(0, 8)) {
      push(fieldLabel(key), value)
    }
  }

  return parts.length > 0 ? parts.join(' · ') : raw
}
