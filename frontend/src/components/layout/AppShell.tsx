import { For, JSX, Show, createSignal } from 'solid-js'
import { navigate, activeTasks, currentProject, route, systemInfo, closeProject } from '../../stores/app'
import type { RouteName } from '../../types'
import { formatNumber } from '../../utils'
import { api } from '../../api'
import { Icon, IconButton, Spinner, type IconName } from '../ui'
import { currentTheme, toggleTheme } from '../../theme'
import appIcon from '../../assets/app-icon.png'

interface NavEntry {
  key: RouteName
  label: string
  icon: IconName
  needsProject?: boolean
  section?: string
}

const NAV: NavEntry[] = [
  { key: 'projects', label: '项目总览', icon: 'folder', section: '工作区' },
  { key: 'collect', label: '数据采集', icon: 'upload', needsProject: true },
  { key: 'logs', label: '日志分析', icon: 'activity', needsProject: true },
  { key: 'investigations', label: '调查工单', icon: 'shield-check', needsProject: true, section: '响应' },
  { key: 'threat', label: '威胁狩猎', icon: 'crosshair', needsProject: true },
  { key: 'settings', label: '系统设置', icon: 'settings', section: '系统' },
]

export function AppShell(props: { children: JSX.Element }): JSX.Element {
  const [refreshing, setRefreshing] = createSignal(false)
  const [theme, setTheme] = createSignal(currentTheme())
  const currentTask = () => activeTasks()[0]
  const navLocked = (entry: NavEntry) => Boolean(entry.needsProject && !currentProject())

  const refreshAll = async () => {
    setRefreshing(true)
    try {
      await api.collect.tasks()
      if (currentProject()) await api.projects.get(currentProject()!.id)
    } catch {
      /* ignore */
    } finally {
      setRefreshing(false)
    }
  }

  const projectMeta = () => {
    const p = currentProject()
    if (!p) return ''
    const sources = p.sources?.length ?? 0
    return `${sources} 个来源 · ${formatNumber(p.log_count)} 条日志`
  }

  return (
    <div class="app-shell">
      <header class="topbar">
        <div class="brand" role="button" tabindex="0" onClick={() => navigate('projects')}>
          <div class="brand-mark">
            <img src={appIcon} alt="GLog" />
          </div>
          <div>
            <div class="brand-name">GLog</div>
            <span class="brand-sub">日志分析工作台 · {document.documentElement.dataset.theme ?? 'none'}</span>
          </div>
        </div>

        <button
          type="button"
          class="topbar-project"
          title={currentProject() ? '切换项目' : '打开项目'}
          onClick={() => navigate('projects')}
        >
          <span class="topbar-project-label">项目</span>
          <Icon name={currentProject() ? 'database' : 'folder'} size={16} />
          <div class="grow" style={{ 'min-width': 0 }}>
            <div class="proj-name truncate">{currentProject()?.name ?? '未打开项目'}</div>
            <div class="proj-meta truncate">
              {currentProject() ? projectMeta() : '选择或创建一个项目开始'}
            </div>
          </div>
          <Icon name="chevron-down" size={14} />
        </button>

        <Show when={currentProject()}>
          <button type="button" class="btn btn-primary btn-sm" onClick={() => navigate('collect')}>
            <Icon name="plus" size={14} />
            采集日志
          </button>
        </Show>

        <div class="grow" />

        <Show when={currentTask()}>
          {(task) => (
            <button type="button" class="task-pill" onClick={() => navigate('collect')} title={task().message}>
              <Spinner />
              <span class="truncate">{task().message || '任务运行中'}</span>
              <span>{Math.round((task().progress || 0) * 100)}%</span>
            </button>
          )}
        </Show>

        <IconButton
          icon="refresh"
          title={refreshing() ? '刷新中' : '刷新任务状态'}
          onClick={() => void refreshAll()}
          disabled={refreshing()}
        />
        <IconButton
          icon={theme() === 'dark' ? 'sun' : 'moon'}
          title={theme() === 'dark' ? '切换到浅色主题' : '切换到深色主题'}
          onClick={() => setTheme(toggleTheme())}
        />
        <button type="button" class="btn btn-ghost btn-sm" onClick={() => navigate('settings')}>
          <Icon name="settings" size={14} />
          设置
        </button>
      </header>

      <aside class="sidebar">
        <For each={NAV}>
          {(entry, index) => (
            <>
              <Show when={entry.section && (index() === 0 || NAV[index() - 1]?.section !== entry.section)}>
                <div class="nav-section">{entry.section}</div>
              </Show>
              <button
                type="button"
                class={`nav-item ${
                  route() === entry.key || (entry.key === 'logs' && route() === 'log-detail') ? 'active' : ''
                } ${navLocked(entry) ? 'locked' : ''}`}
                aria-disabled={navLocked(entry)}
                title={navLocked(entry) ? '打开项目后可用' : entry.label}
                onClick={() => navigate(entry.key)}
              >
                <Icon name={entry.icon} size={17} />
                <span class="nav-label">{entry.label}</span>
                <Show when={navLocked(entry)}>
                  <Icon name="lock" size={12} class="nav-lock" />
                </Show>
              </button>
            </>
          )}
        </For>

        <div class="sidebar-footer">
          <Show
            when={currentProject()}
            fallback={
              <div class="sidebar-empty">
                <Icon name="folder" size={18} />
                <span>尚未打开项目</span>
              </div>
            }
          >
            {(p) => (
              <div class="sidebar-project">
                <div class="sidebar-project-title">
                  <Icon name="database" size={14} />
                  <span class="truncate">{p().name}</span>
                </div>
                <div class="sidebar-project-metrics">
                  <span>{formatNumber(p().log_count)} 条</span>
                  <span>{(p().size_bytes / 1024 / 1024).toFixed(1)} MB</span>
                  <span>
                    E{p().level_distribution?.ERROR ?? 0} / W{p().level_distribution?.WARN ?? 0}
                  </span>
                </div>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost btn-block"
                  onClick={() => void closeProject().catch(() => undefined)}
                >
                  关闭项目
                </button>
              </div>
            )}
          </Show>
        </div>
      </aside>

      <main class="main-area">{props.children}</main>

      <footer class="statusbar">
        <span class={`dot ${activeTasks().length ? 'busy' : ''}`} />
        <span>{activeTasks().length ? `正在处理 ${activeTasks().length} 个任务` : '本地模式 · 就绪'}</span>
        <span class="status-sep">/</span>
        <span>
          {systemInfo()?.os ?? '—'} · {systemInfo()?.arch ?? '—'}
        </span>
        <span class="status-sep">/</span>
        <span title={systemInfo()?.data_path} class="truncate">
          数据目录：{systemInfo()?.data_path ?? '—'}
        </span>
        <span class="spacer" />
        <span>v{systemInfo()?.version ?? '1.0.0'}</span>
      </footer>
    </div>
  )
}

export function BootScreen(): JSX.Element {
  return (
    <div class="boot-screen">
      <div class="boot-inner">
        <img class="boot-icon" src={appIcon} alt="" />
        <div class="boot-logo">GLog</div>
        <div class="boot-sub">正在加载日志分析工作台…</div>
        <Spinner large />
      </div>
    </div>
  )
}
