export type BadgeTone = 'mint' | 'amber' | 'ember' | 'sky' | 'violet' | 'muted'

export const CLUE_TYPES: Array<{ value: string; label: string }> = [
  { value: 'ip', label: 'IP 地址' },
  { value: 'user', label: '用户' },
  { value: 'pid', label: '进程 ID' },
  { value: 'file', label: '文件' },
  { value: 'domain', label: '域名' },
  { value: 'event_id', label: '事件 ID' },
  { value: 'message', label: '日志消息' },
  { value: 'log_entry', label: '日志条目' },
]

export function clueTypeLabel(type: string | undefined): string {
  const value = String(type ?? '')
  return CLUE_TYPES.find((item) => item.value === value)?.label ?? (value || '未指定')
}

export function investigationStatusLabel(status: string | undefined): string {
  switch (String(status ?? '').toLowerCase()) {
    case 'open':
      return '待处理'
    case 'analyzing':
      return '分析中'
    case 'closed':
      return '已结案'
    default:
      return String(status ?? '未知')
  }
}

export function investigationStatusTone(status: string | undefined): BadgeTone {
  switch (String(status ?? '').toLowerCase()) {
    case 'open':
      return 'sky'
    case 'analyzing':
      return 'amber'
    case 'closed':
      return 'mint'
    default:
      return 'muted'
  }
}
