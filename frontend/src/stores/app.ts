import { createMemo, createSignal } from 'solid-js'
import { api, errorText, onEvent } from '../api'
import type {
  AppConfig,
  CollectionProgress,
  LocalLogDiscovery,
  Project,
  RouteName,
  SystemInfo,
  TaskInfo,
} from '../types'

export type ToastKind = 'success' | 'error' | 'warn' | 'info'

export interface ToastItem {
  id: number
  kind: ToastKind
  title: string
  message?: string
}

const ROUTE_TITLES: Record<RouteName, string> = {
  projects: '项目',
  collect: '数据采集',
  logs: '日志分析',
  'log-detail': '单条日志',
  investigations: '调查工单',
  threat: '威胁狩猎',
  settings: '设置',
}

const [route, setRouteSignal] = createSignal<RouteName>('projects')
const [currentProject, setCurrentProject] = createSignal<Project | null>(null)
const [systemInfo, setSystemInfo] = createSignal<SystemInfo | null>(null)
const [appConfig, setAppConfig] = createSignal<AppConfig | null>(null)
const [tasks, setTasks] = createSignal<TaskInfo[]>([])
const [toasts, setToasts] = createSignal<ToastItem[]>([])
const [booting, setBooting] = createSignal(true)
const [sidebarOpen, setSidebarOpen] = createSignal(true)
const [selectedLogDocID, setSelectedLogDocID] = createSignal<number | null>(null)
// 从日志列表/详情抽屉发起「打开独立页并自动弹出 AI 分析」的一次性请求。
const [logAIAutoOpenDocID, setLogAIAutoOpenDocID] = createSignal<number | null>(null)
const [logAIAutoOpenSeq, setLogAIAutoOpenSeq] = createSignal(0)
const [localDiscovery, setLocalDiscovery] = createSignal<LocalLogDiscovery | null>(null)
const [localDiscoveryLoading, setLocalDiscoveryLoading] = createSignal(false)
const [localDiscoveryError, setLocalDiscoveryError] = createSignal('')

let booted = false
let toastSeq = 0
let discoverySeq = 0
const taskTimers = new Map<string, number>()

export {
  route,
  currentProject,
  systemInfo,
  appConfig,
  tasks,
  toasts,
  booting,
  sidebarOpen,
  selectedLogDocID,
  logAIAutoOpenDocID,
  logAIAutoOpenSeq,
  localDiscovery,
  localDiscoveryLoading,
  localDiscoveryError,
}
export { setAppConfig, setCurrentProject, setSidebarOpen, setSelectedLogDocID }

export const activeTasks = createMemo(() =>
  tasks().filter((t) => t.status === 'running' || t.status === 'pending'),
)

export const routeTitle = createMemo(() => ROUTE_TITLES[route()] ?? route())

export function requiresProject(r: RouteName): boolean {
  return r === 'collect' || r === 'logs' || r === 'log-detail' || r === 'investigations' || r === 'threat'
}

/** 打开单条日志的独立查看页面。普通打开会取消未消费的 AI 自动弹窗请求。 */
export function openLogDetail(docID: number): void {
  if (!currentProject()) {
    notify('warn', '请先打开项目', '查看日志详情需要项目上下文。')
    return
  }
  setLogAIAutoOpenDocID(null)
  setSelectedLogDocID(docID)
  setRouteSignal('log-detail')
}

/**
 * 打开单条日志独立页，并请求该页在日志加载完成后自动打开 AI 分析弹窗。
 * 供日志列表的 AI 快捷按钮与详情抽屉使用。
 */
export function openLogDetailWithAI(docID: number): void {
  if (!currentProject()) {
    notify('warn', '请先打开项目', 'AI 分析需要项目上下文。')
    return
  }
  // 先走普通打开流程（会清空旧请求），再登记本次自动弹窗请求。
  openLogDetail(docID)
  setLogAIAutoOpenDocID(docID)
  setLogAIAutoOpenSeq((value) => value + 1)
}

/** 消费一次自动打开 AI 分析的请求；仅当文档匹配时清除，避免误触发。 */
export function consumeLogAIAutoOpen(docID: number | null | undefined): boolean {
  if (docID == null || logAIAutoOpenDocID() !== docID) return false
  setLogAIAutoOpenDocID(null)
  return true
}

/** 返回日志工作台。 */
export function backToLogs(): void {
  setLogAIAutoOpenDocID(null)
  setRouteSignal('logs')
}

export function navigate(next: RouteName): void {
  if (requiresProject(next) && !currentProject()) {
    notify('warn', '请先打开项目', '采集、分析与调查功能需要先打开一个项目。')
    setRouteSignal('projects')
    return
  }
  setRouteSignal(next)
}

export function notify(kind: ToastKind, title: string, message?: string, timeout = 4500): void {
  const id = ++toastSeq
  setToasts((prev) => [...prev, { id, kind, title, message }])
  window.setTimeout(() => dismissToast(id), timeout)
}

export function dismissToast(id: number): void {
  setToasts((prev) => prev.filter((t) => t.id !== id))
}

function upsertTask(task: TaskInfo): void {
  if (!task?.id) return
  setTasks((prev) => {
    const idx = prev.findIndex((t) => t.id === task.id)
    if (idx < 0) return [task, ...prev]
    const copy = prev.slice()
    copy[idx] = { ...copy[idx], ...task }
    return copy
  })
}

function updateTaskProgress(p: CollectionProgress): void {
  if (!p?.task_id) return
  setTasks((prev) => {
    const idx = prev.findIndex((t) => t.id === p.task_id)
    if (idx < 0) return prev
    const copy = prev.slice()
    copy[idx] = {
      ...copy[idx],
      status: 'running',
      progress: p.progress,
      message: p.message,
      processed: p.processed,
      total: p.total,
    }
    return copy
  })
}

/** 扫描本机常见日志。启动时自动执行一次，也可手动刷新。 */
export async function refreshLocalDiscovery(manual = false): Promise<void> {
  const seq = ++discoverySeq
  setLocalDiscoveryLoading(true)
  setLocalDiscoveryError('')
  try {
    const result = await api.collect.detectLocalLogs()
    if (seq !== discoverySeq) return
    setLocalDiscovery(result)
    if (manual) {
      notify('success', '本机日志扫描完成', `发现 ${result?.items?.length ?? 0} 个日志来源。`)
    }
  } catch (err) {
    if (seq !== discoverySeq) return
    const message = errorText(err)
    setLocalDiscoveryError(message)
    if (manual) notify('error', '扫描本机日志失败', message)
  } finally {
    if (seq === discoverySeq) setLocalDiscoveryLoading(false)
  }
}

async function refreshTasks(): Promise<void> {
  try {
    const list = await api.collect.tasks()
    setTasks(Array.isArray(list) ? list : [])
  } catch {
    /* 后端尚未就绪或运行在浏览器中 */
  }
}

async function refreshCurrentProject(): Promise<void> {
  const p = currentProject()
  if (!p) return
  try {
    const fresh = await api.projects.get(p.id)
    setCurrentProject(fresh)
  } catch {
    /* 项目可能已被删除 */
  }
}

function handleTaskDone(task: TaskInfo): void {
  if (!task?.id) return
  upsertTask(task)
  void refreshTasks()
  const ok = task.status === 'success'
  const status = task.status === 'stopped' ? '任务已停止' : ok ? '任务完成' : '任务失败'
  notify(
    ok ? 'success' : task.status === 'stopped' ? 'warn' : 'error',
    status,
    task.error || task.message || `${task.processed ?? 0} 条日志`,
    6000,
  )
  if (task.status === 'success') {
    void refreshCurrentProject()
  }
  const timer = taskTimers.get(task.id)
  if (timer) {
    window.clearTimeout(timer)
    taskTimers.delete(task.id)
  }
}

function subscribeEvents(): void {
  onEvent<TaskInfo>('collect:started', (t) => upsertTask(t))
  onEvent<CollectionProgress>('collect:progress', (p) => updateTaskProgress(p))
  onEvent<TaskInfo>('collect:done', (t) => handleTaskDone(t))
  onEvent<CollectionProgress>('analysis:progress', (p) => updateTaskProgress(p))
  onEvent<TaskInfo>('analysis:done', (t) => handleTaskDone(t))
}

export async function openProject(id: string): Promise<void> {
  await api.projects.open(id)
  const project = await api.projects.get(id)
  setCurrentProject(project)
  setRouteSignal('logs')
}

export async function closeProject(): Promise<void> {
  try {
    await api.projects.close()
  } finally {
    setCurrentProject(null)
    setRouteSignal('projects')
  }
}

export async function boot(): Promise<void> {
  if (booted) return
  booted = true
  try {
    const [info, cfg] = await Promise.all([
      api.system.info(),
      api.system.config().catch(() => null),
    ])
    setSystemInfo(info)
    if (cfg) setAppConfig(cfg)
    subscribeEvents()
    // 启动即发现本机日志，不阻塞界面加载。
    void refreshLocalDiscovery(false)
    await refreshTasks()

    let opened: Project | null = null
    try {
      opened = await api.projects.current()
    } catch {
      opened = null
    }

    // 启动后停留在项目总览，不自动打开任何项目：
    // 初始界面不加载日志，避免旧项目日志在启动时直接出现。
    if (opened) {
      setCurrentProject(opened)
      setRouteSignal('logs')
    }
  } catch (err) {
    notify('error', '初始化失败', errorText(err), 9000)
    setRouteSignal('projects')
  } finally {
    setBooting(false)
  }
}

export async function reloadConfig(): Promise<void> {
  try {
    const cfg = await api.system.config()
    setAppConfig(cfg)
  } catch {
    /* ignore */
  }
}

export function showTaskInStatus(task: TaskInfo): string {
  if (task.status === 'running') {
    return `${task.message || '处理中'} ${Math.round((task.progress || 0) * 100)}%`
  }
  return task.message || task.status
}
