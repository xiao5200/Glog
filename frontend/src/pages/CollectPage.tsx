import {
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
  onMount,
  type JSX,
} from 'solid-js'
import { PageHeader } from '../components/layout/PageHeader'
import { Badge, Button, EmptyState, Icon, Tabs } from '../components/ui'
import { LocalCollector } from '../components/collector/LocalCollector'
import { EVTXCollector } from '../components/collector/EVTXCollector'
import { JournalCollector } from '../components/collector/JournalCollector'
import { SSHCollector } from '../components/collector/SSHCollector'
import { ImportFile } from '../components/collector/ImportFile'
import { ParseConfigPanel } from '../components/collector/ParseConfigPanel'
import { TaskList } from '../components/collector/TaskList'
import { SourceList } from '../components/collector/SourceList'
import { api, errorText } from '../api'
import type { AIProviderInfo, LogSource, ParseTemplate, TaskInfo } from '../types'
import {
  currentProject,
  navigate,
  notify,
  setCurrentProject,
  systemInfo,
  tasks,
} from '../stores/app'
import {
  buildParseConfig,
  createParseState,
  isWindowsPlatform,
  mergeTaskLists,
  parseModeOptionsForTab,
  type ParseMode,
  type ParseState,
  type ParseTabKey,
} from '../components/collector/shared'

const TAB_ITEMS = [
  { key: 'local', label: '本地文件' },
  { key: 'evtx', label: 'Windows 事件日志' },
  { key: 'journal', label: 'systemd 日志' },
  { key: 'ssh', label: 'SSH 远程' },
  { key: 'import', label: '文件导入' },
]

const INITIAL_MODES: Record<ParseTabKey, ParseMode> = {
  local: 'builtin',
  evtx: 'native_evtx',
  journal: 'builtin',
  ssh: 'builtin',
  import: 'builtin',
}

/** 项目内采集工作台：本地文件、Windows 事件、SSH 与文件导入。 */
export function CollectPage(): JSX.Element {
  const [activeTab, setActiveTab] = createSignal<ParseTabKey>('local')
  const [parse, setParse] = createSignal<ParseState>(createParseState())
  const [modeByTab, setModeByTab] = createSignal<Record<ParseTabKey, ParseMode>>({
    ...INITIAL_MODES,
  })
  const [templates, setTemplates] = createSignal<ParseTemplate[]>([])
  const [providers, setProviders] = createSignal<AIProviderInfo[]>([])
  const [dictLoading, setDictLoading] = createSignal(false)
  const [dictError, setDictError] = createSignal('')
  const [sources, setSources] = createSignal<LogSource[]>([])
  const [sourcesLoading, setSourcesLoading] = createSignal(false)
  const [sourcesError, setSourcesError] = createSignal('')
  const [remoteTasks, setRemoteTasks] = createSignal<TaskInfo[]>([])
  const [tasksLoading, setTasksLoading] = createSignal(false)
  const [tasksError, setTasksError] = createSignal('')
  const [previewLines, setPreviewLines] = createSignal<string[]>([])
  const [previewSource, setPreviewSource] = createSignal('')
  const [narrow, setNarrow] = createSignal(false)

  const windowsPlatform = createMemo(() => isWindowsPlatform(systemInfo()?.os))
  const mergedTasks = createMemo(() => mergeTaskLists(tasks(), remoteTasks()))
  // 任务队列只展示当前项目，避免新项目出现上一个项目的采集任务。
  const projectTasks = createMemo(() => {
    const projectID = currentProject()?.id
    if (!projectID) return []
    return mergedTasks().filter((task) => task.project_id === projectID)
  })
  const parseConfig = () => buildParseConfig(parse())

  function updateParse(patch: Partial<ParseState>) {
    if (patch.mode) {
      const tab = activeTab()
      const nextMode = patch.mode
      setModeByTab((prev) => {
        const next: Record<ParseTabKey, ParseMode> = { ...prev }
        next[tab] = nextMode
        return next
      })
    }
    setParse((prev) => ({ ...prev, ...patch }))
  }

  function changeTab(key: string) {
    const next = key as ParseTabKey
    const previous = activeTab()
    if (next === previous) return
    const previousMode = parse().mode
    setModeByTab((prev) => {
      const updated: Record<ParseTabKey, ParseMode> = { ...prev }
      updated[previous] = previousMode
      return updated
    })
    const options = parseModeOptionsForTab(next)
    const saved = modeByTab()[next]
    const mode = options.some((option) => option.value === saved)
      ? saved
      : options[0].value
    setParse((prev) => ({ ...prev, mode }))
    setActiveTab(next)
  }

  function applyPresetTemplate(templateID: string) {
    if (!templateID) return
    updateParse({ template_id: templateID, mode: 'builtin' })
  }

  function handleGeneratedTemplate(template: ParseTemplate) {
    setTemplates((prev) => [template, ...prev.filter((item) => item.id !== template.id)])
    updateParse({ template_id: template.id })
  }

  function handleSamples(lines: string[], source: string) {
    const list = Array.isArray(lines)
      ? lines.filter((line) => line.trim().length > 0).slice(0, 500)
      : []
    setPreviewLines(list)
    setPreviewSource(source)
    if (list.length > 0 && !parse().samples.trim()) {
      setParse((prev) => ({ ...prev, samples: list.join('\n') }))
    }
  }

  async function loadDictionaries(manual = false) {
    setDictLoading(true)
    setDictError('')
    try {
      const results = await Promise.allSettled([
        api.templates.list({}),
        api.ai.providers(),
      ] as const)
      const [templateResult, providerResult] = results
      const errors: string[] = []

      if (templateResult.status === 'fulfilled') {
        setTemplates(Array.isArray(templateResult.value) ? templateResult.value : [])
      } else {
        errors.push(`模板：${errorText(templateResult.reason)}`)
      }

      if (providerResult.status === 'fulfilled') {
        const list = Array.isArray(providerResult.value) ? providerResult.value : []
        setProviders(list)
        const currentProvider = parse().ai_provider
        if (!currentProvider || !list.some((item) => item.provider === currentProvider)) {
          const preferred =
            list.find((item) => item.configured && item.enabled) ??
            list.find((item) => item.enabled) ??
            list[0]
          if (preferred) {
            setParse((prev) => ({ ...prev, ai_provider: preferred.provider }))
          }
        }
      } else {
        errors.push(`AI 提供商：${errorText(providerResult.reason)}`)
      }

      const message = errors.join('；')
      setDictError(message)
      if (manual) {
        if (message) notify('error', '解析配置加载失败', message)
        else notify('success', '解析配置已刷新')
      }
    } catch (err) {
      const message = errorText(err)
      setDictError(message)
      if (manual) notify('error', '解析配置加载失败', message)
    } finally {
      setDictLoading(false)
    }
  }

  let sourceSeq = 0
  async function loadSources(projectID: string, manual = false): Promise<boolean> {
    const seq = ++sourceSeq
    setSourcesLoading(true)
    setSourcesError('')
    try {
      const result = await api.projects.sources(projectID)
      if (seq !== sourceSeq) return false
      setSources(Array.isArray(result) ? result : [])
      return true
    } catch (err) {
      if (seq === sourceSeq) {
        const message = errorText(err)
        setSourcesError(message)
        if (manual) notify('error', '刷新日志源失败', message)
      }
      return false
    } finally {
      if (seq === sourceSeq) setSourcesLoading(false)
    }
  }

  async function refreshSources() {
    const project = currentProject()
    if (!project) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return
    }
    const ok = await loadSources(project.id, true)
    if (!ok) return
    notify('success', '日志源已刷新', `共 ${sources().length} 个来源。`)
    try {
      const fresh = await api.projects.get(project.id)
      if (currentProject()?.id === project.id) setCurrentProject(fresh)
    } catch {
      /* 日志源已刷新，项目统计失败不阻塞 */
    }
  }

  async function refreshTaskList(manual = false) {
    setTasksLoading(true)
    try {
      const result = await api.collect.tasks()
      setRemoteTasks(Array.isArray(result) ? result : [])
      setTasksError('')
      if (manual) notify('success', '任务列表已刷新')
    } catch (err) {
      const message = errorText(err)
      setTasksError(message)
      if (manual) notify('error', '刷新任务失败', message)
    } finally {
      setTasksLoading(false)
    }
  }

  async function refreshAfterCollect(projectID: string) {
    await loadSources(projectID, false)
    try {
      const fresh = await api.projects.get(projectID)
      if (currentProject()?.id === projectID) setCurrentProject(fresh)
    } catch {
      /* 统计刷新失败不影响采集完成提示 */
    }
    void refreshTaskList(false)
  }

  let sourceProjectID = ''
  createEffect(() => {
    const project = currentProject()
    const id = project?.id ?? ''
    if (!id) {
      sourceProjectID = ''
      setSources([])
      setSourcesError('')
      return
    }
    if (sourceProjectID === id) return
    sourceProjectID = id
    // 切换项目时立即清空上一个项目的来源与预览，避免新项目短暂显示旧数据。
    setSources([])
    setSourcesError('')
    setPreviewLines([])
    setPreviewSource('')
    setParse(createParseState())
    setModeByTab({ ...INITIAL_MODES })
    void loadSources(id, false)
  })

  let knownSuccess = new Set<string>()
  let successProjectID = ''
  createEffect(() => {
    const project = currentProject()
    const list = tasks()
    if (!project) {
      knownSuccess = new Set()
      successProjectID = ''
      return
    }
    if (successProjectID !== project.id) {
      successProjectID = project.id
      knownSuccess = new Set(
        list.filter((task) => task.status === 'success').map((task) => task.id),
      )
      return
    }
    const fresh = list.filter(
      (task) =>
        task.status === 'success' &&
        task.project_id === project.id &&
        !knownSuccess.has(task.id),
    )
    if (!fresh.length) return
    for (const task of fresh) knownSuccess.add(task.id)
    void refreshAfterCollect(project.id)
  })

  onMount(() => {
    void refreshTaskList(false)
    void loadDictionaries(false)
    const updateViewport = () => setNarrow(window.innerWidth < 1200)
    updateViewport()
    window.addEventListener('resize', updateViewport)
    onCleanup(() => window.removeEventListener('resize', updateViewport))
  })

  const headerSubtitle = () => {
    const project = currentProject()
    return project ? `当前项目：${project.name}` : '未打开项目 · 请先在项目总览中选择项目'
  }

  return (
    <div class="page">
      <PageHeader
        eyebrow="数据接入 / 日志采集"
        title="数据采集"
        subtitle={headerSubtitle()}
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={sourcesLoading()}
              onClick={() => void refreshSources()}
            >
              <Show when={!sourcesLoading()}>
                <Icon name="database" size={12} />
              </Show>
              刷新来源
            </Button>
            <Button
              size="sm"
              variant="ghost"
              loading={tasksLoading()}
              onClick={() => void refreshTaskList(true)}
            >
              <Show when={!tasksLoading()}>
                <Icon name="refresh" size={12} />
              </Show>
              刷新任务
            </Button>
          </>
        }
      />

      <Show
        when={currentProject()}
        fallback={
          <div class="panel">
            <div class="panel-body">
              <EmptyState
                icon="folder"
                title="尚未打开项目"
                text="采集配置、任务队列与日志源均绑定到当前项目。请先在项目总览打开或创建一个项目。"
                action={
                  <Button variant="primary" onClick={() => navigate('projects')}>
                    <Icon name="arrow-right" size={12} />
                    前往项目总览
                  </Button>
                }
              />
            </div>
          </div>
        }
      >
        {(project) => (
          <div
            class="split"
            style={{
              'grid-template-columns': narrow()
                ? 'minmax(0, 1fr)'
                : 'minmax(0, 1fr) 368px',
            }}
          >
            <div class="panel">
              <div class="panel-head">
                <div class="panel-title">
                  <span class="tick" />
                  采集配置
                </div>
                <div class="row wrap" style={{ gap: '6px' }}>
                  <Badge tone="mint">
                    {project().status === 'active' ? 'ACTIVE' : project().status || 'PROJECT'}
                  </Badge>
                  <span
                    class="mono-sm faint truncate"
                    style={{ 'max-width': '190px' }}
                    title={project().project_path}
                  >
                    {project().project_path || project().name}
                  </span>
                </div>
              </div>

              <div style={{ padding: '0 16px' }}>
                <Tabs items={TAB_ITEMS} active={activeTab()} onChange={changeTab} />
              </div>

              <div class="panel-body stack" style={{ gap: '14px' }}>
                <div style={{ display: activeTab() === 'local' ? 'block' : 'none' }}>
                  <LocalCollector
                    projectId={project().id}
                    parse={parseConfig}
                    windows={windowsPlatform()}
                    onSamples={handleSamples}
                    onTemplate={applyPresetTemplate}
                  />
                </div>

                <div style={{ display: activeTab() === 'evtx' ? 'block' : 'none' }}>
                  <EVTXCollector
                    projectId={project().id}
                    parse={parseConfig}
                    windows={windowsPlatform()}
                  />
                </div>

                <div style={{ display: activeTab() === 'journal' ? 'block' : 'none' }}>
                  <JournalCollector projectId={project().id} parse={parseConfig} />
                </div>

                <div style={{ display: activeTab() === 'ssh' ? 'block' : 'none' }}>
                  <SSHCollector projectId={project().id} parse={parseConfig} />
                </div>

                <div style={{ display: activeTab() === 'import' ? 'block' : 'none' }}>
                  <ImportFile
                    projectId={project().id}
                    parse={parseConfig}
                    onSamples={handleSamples}
                  />
                </div>

                <div class="divider" style={{ margin: '0' }} />

                <ParseConfigPanel
                  tab={activeTab()}
                  parse={parse()}
                  onChange={updateParse}
                  templates={templates()}
                  providers={providers()}
                  previewLines={previewLines()}
                  previewSource={previewSource()}
                  templatesLoading={dictLoading()}
                  templatesError={dictError()}
                  onReload={() => void loadDictionaries(true)}
                  onGenerated={handleGeneratedTemplate}
                />
              </div>
            </div>

            <aside class="stack" style={{ gap: '14px' }}>
              <TaskList
                tasks={projectTasks()}
                loading={tasksLoading()}
                error={tasksError()}
                onRefresh={() => void refreshTaskList(true)}
              />

              <SourceList
                projectId={project().id}
                sources={sources()}
                loading={sourcesLoading()}
                error={sourcesError()}
                onRefresh={() => void refreshSources()}
                onTaskStarted={() => void refreshTaskList(true)}
              />

              <div class="panel">
                <div class="panel-head">
                  <div class="panel-title">
                    <span class="tick" />
                    采集说明
                  </div>
                  <Badge tone="muted">OPS</Badge>
                </div>
                <div class="panel-body stack" style={{ gap: '11px' }}>
                  <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                    <span class="accent" style={{ 'margin-top': '2px' }}>
                      <Icon name="package" size={13} />
                    </span>
                    <div>
                      <div style={{ 'font-size': '11.5px' }}>压缩日志</div>
                      <div class="mono-sm faint">
                        支持 .gz 自动解压，无需手工展开；EVTX 文件可直接解析。
                      </div>
                    </div>
                  </div>
                  <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                    <span class="accent" style={{ 'margin-top': '2px' }}>
                      <Icon name="layers" size={13} />
                    </span>
                    <div>
                      <div style={{ 'font-size': '11.5px' }}>跨平台 EVTX</div>
                      <div class="mono-sm faint">
                        EVTX 可在任意平台解析；Windows 事件通道采集需要 Windows 主机。
                      </div>
                    </div>
                  </div>
                  <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                    <span class="accent" style={{ 'margin-top': '2px' }}>
                      <Icon name="lock" size={13} />
                    </span>
                    <div>
                      <div style={{ 'font-size': '11.5px' }}>SSH 只读安全策略</div>
                      <div class="mono-sm faint">
                        SSH 采集只读取远程日志，不修改或删除远程文件。
                      </div>
                    </div>
                  </div>
                  <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                    <span class="accent" style={{ 'margin-top': '2px' }}>
                      <Icon name="folder" size={13} />
                    </span>
                    <div style={{ 'min-width': 0 }}>
                      <div style={{ 'font-size': '11.5px' }}>项目目录存储</div>
                      <div
                        class="mono-sm faint truncate"
                        title={project().project_path}
                      >
                        日志数据保存在 {project().project_path || '当前项目目录'}。
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </aside>
          </div>
        )}
      </Show>
    </div>
  )
}
