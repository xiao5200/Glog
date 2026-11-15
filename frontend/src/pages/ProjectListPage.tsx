import { For, Show, createMemo, createSignal, onCleanup, onMount } from 'solid-js'
import type {
  LogSource,
  Project,
  ProjectFilter,
  ProjectStatistics,
} from '../types'
import { api, errorText } from '../api'
import {
  activeTasks,
  closeProject,
  currentProject,
  notify,
  openProject,
  setCurrentProject,
} from '../stores/app'
import { PageHeader } from '../components/layout/PageHeader'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Icon,
  PageBusy,
  Select,
  Spinner,
  StatTile,
  TextInput,
} from '../components/ui'
import {
  CreateProjectModal,
  EditProjectModal,
  type ProjectFormValues,
} from '../components/project/CreateProjectModal'
import { ProjectCard } from '../components/project/ProjectCard'
import { ProjectDetailDrawer } from '../components/project/ProjectDetailDrawer'
import { LocalLogDiscovery } from '../components/project/LocalLogDiscovery'
import { formatBytes, formatNumber, shortId, truncate } from '../utils'

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '活跃 / active' },
  { value: 'archived', label: '已归档 / archived' },
]

const SORT_OPTIONS = [
  { value: 'updated_at', label: '最近更新' },
  { value: 'created_at', label: '创建时间' },
  { value: 'log_count', label: '日志数量' },
  { value: 'size_bytes', label: '占用空间' },
  { value: 'name', label: '项目名称' },
]

const DIRECTION_OPTIONS = [
  { value: 'desc', label: '降序' },
  { value: 'asc', label: '升序' },
]

interface LoadOutcome<T> {
  value: T | null
  error: unknown
}

interface SourceCountResult {
  id: string
  count: number | null
  error: unknown
}

async function settle<T>(promise: Promise<T>): Promise<LoadOutcome<T>> {
  try {
    return { value: await promise, error: null }
  } catch (error) {
    return { value: null, error }
  }
}

function isPackagePath(path: string): boolean {
  return /\.(zip|glogpkg)$/i.test(String(path ?? '').trim())
}

function packageFilename(name: string): string {
  const base = String(name ?? '')
    .trim()
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_')
    .replace(/[. ]+$/g, '')
    .slice(0, 80)
  return `${base || 'glog-project'}.glogpkg.zip`
}

export function ProjectListPage() {
  const [projects, setProjects] = createSignal<Project[]>([])
  const [sourceCounts, setSourceCounts] = createSignal<Record<string, number>>({})
  const [loading, setLoading] = createSignal(true)
  const [loaded, setLoaded] = createSignal(false)
  const [listError, setListError] = createSignal('')

  const [search, setSearch] = createSignal('')
  const [status, setStatus] = createSignal('')
  const [sortBy, setSortBy] = createSignal('updated_at')
  const [sortDesc, setSortDesc] = createSignal(true)

  const [createOpen, setCreateOpen] = createSignal(false)
  const [createBusy, setCreateBusy] = createSignal(false)
  const [createError, setCreateError] = createSignal('')

  const [editTarget, setEditTarget] = createSignal<Project | null>(null)
  const [editBusy, setEditBusy] = createSignal(false)
  const [editError, setEditError] = createSignal('')

  const [pendingArchive, setPendingArchive] = createSignal<Project | null>(null)
  const [pendingDelete, setPendingDelete] = createSignal<Project | null>(null)
  const [confirmBusy, setConfirmBusy] = createSignal(false)

  const [importing, setImporting] = createSignal(false)
  const [closingProject, setClosingProject] = createSignal(false)

  const [detailOpen, setDetailOpen] = createSignal(false)
  const [detailProject, setDetailProject] = createSignal<Project | null>(null)
  const [detailStatistics, setDetailStatistics] =
    createSignal<ProjectStatistics | null>(null)
  const [detailSources, setDetailSources] = createSignal<LogSource[]>([])
  const [detailLoading, setDetailLoading] = createSignal(false)
  const [detailError, setDetailError] = createSignal('')

  let requestSeq = 0
  let detailSeq = 0
  let searchTimer: number | undefined

  const totals = createMemo(() => {
    let projectCount = 0
    let logs = 0
    let bytes = 0
    let active = 0
    let archived = 0
    for (const project of projects()) {
      projectCount += 1
      logs += Number(project.log_count) || 0
      bytes += Number(project.size_bytes) || 0
      if (String(project.status ?? '').toLowerCase() === 'archived') {
        archived += 1
      } else {
        active += 1
      }
    }
    return { projectCount, logs, bytes, active, archived }
  })

  const filtersActive = () => Boolean(search().trim() || status())
  const direction = () => (sortDesc() ? 'desc' : 'asc')

  const currentName = () => {
    const project = currentProject()
    return project?.name ? truncate(project.name, 18) : '未打开'
  }

  const currentNote = () => {
    const project = currentProject()
    return project ? `#${shortId(project.id)} · 点击查看项目情报` : '创建或导入项目开始分析'
  }

  async function loadProjects(): Promise<void> {
    if (searchTimer != null) {
      window.clearTimeout(searchTimer)
      searchTimer = undefined
    }
    const seq = ++requestSeq
    setLoading(true)
    setListError('')

    const filter: ProjectFilter = {
      sort_by: sortBy(),
      sort_desc: sortDesc(),
    }
    const query = search().trim()
    if (query) filter.search = query
    if (status()) filter.status = status()

    try {
      const list = await api.projects.list(filter)
      if (seq !== requestSeq) return
      const safeList = Array.isArray(list) ? list : []
      setProjects(safeList)
      setSourceCounts({})
      setLoaded(true)
      void loadSourceCounts(safeList, seq)
    } catch (error) {
      if (seq !== requestSeq) return
      const message = errorText(error)
      setListError(message)
      notify('error', '项目档案加载失败', message, 8000)
    } finally {
      if (seq === requestSeq) setLoading(false)
    }
  }

  async function loadSourceCounts(list: Project[], seq: number): Promise<void> {
    if (list.length === 0 || seq !== requestSeq) return
    try {
      const results = await Promise.all(
        list.map(async (project): Promise<SourceCountResult> => {
          try {
            const sources = await api.projects.sources(project.id)
            return {
              id: project.id,
              count: Array.isArray(sources) ? sources.length : 0,
              error: null,
            }
          } catch (error) {
            return { id: project.id, count: null, error }
          }
        }),
      )
      if (seq !== requestSeq) return

      const counts: Record<string, number> = {}
      let failed = 0
      let firstError: unknown = null
      let hasFirstError = false
      for (const result of results) {
        if (result.count == null) {
          failed += 1
          if (!hasFirstError) {
            firstError = result.error
            hasFirstError = true
          }
        } else {
          counts[result.id] = result.count
        }
      }
      setSourceCounts(counts)
      if (failed > 0) {
        notify(
          'error',
          '部分项目来源数加载失败',
          `${failed}/${list.length} 个项目的来源统计不可用：${errorText(firstError)}`,
          8000,
        )
      }
    } catch (error) {
      if (seq !== requestSeq) return
      notify('error', '项目来源统计加载失败', errorText(error), 8000)
    }
  }

  function queueReload(): void {
    if (searchTimer != null) window.clearTimeout(searchTimer)
    searchTimer = window.setTimeout(() => {
      searchTimer = undefined
      void loadProjects()
    }, 260)
  }

  function reloadNow(): void {
    if (searchTimer != null) {
      window.clearTimeout(searchTimer)
      searchTimer = undefined
    }
    void loadProjects()
  }

  function handleSearchInput(value: string): void {
    setSearch(value)
    queueReload()
  }

  function handleStatusChange(value: string): void {
    setStatus(value)
    reloadNow()
  }

  function handleSortChange(value: string): void {
    setSortBy(value)
    reloadNow()
  }

  function handleDirectionChange(value: string): void {
    setSortDesc(value !== 'asc')
    reloadNow()
  }

  function resetFilters(): void {
    setSearch('')
    setStatus('')
    setSortBy('updated_at')
    setSortDesc(true)
    reloadNow()
  }

  function openCreateModal(): void {
    setCreateError('')
    setCreateOpen(true)
  }

  function closeCreateModal(): void {
    if (!createBusy()) setCreateOpen(false)
  }

  async function handleCreate(values: ProjectFormValues): Promise<void> {
    if (createBusy()) return
    setCreateBusy(true)
    setCreateError('')
    try {
      const created = await api.projects.create(values)
      notify('success', '项目已创建', created.name)
      setCreateOpen(false)
      try {
        await openProject(created.id)
      } catch (error) {
        notify('error', '项目已创建，但打开失败', errorText(error), 8000)
        await loadProjects()
      }
    } catch (error) {
      const message = errorText(error)
      setCreateError(message)
      notify('error', '创建项目失败', message, 8000)
    } finally {
      setCreateBusy(false)
    }
  }

  function openEditModal(project: Project): void {
    setEditError('')
    setEditTarget(project)
  }

  function closeEditModal(): void {
    if (!editBusy()) setEditTarget(null)
  }

  async function handleEditSubmit(values: ProjectFormValues): Promise<void> {
    const target = editTarget()
    if (!target || editBusy()) return
    setEditBusy(true)
    setEditError('')
    try {
      const updated = await api.projects.update(target.id, values)
      notify('success', '项目档案已更新', updated.name)
      setEditTarget(null)
      if (currentProject()?.id === updated.id) setCurrentProject(updated)
      if (detailProject()?.id === updated.id) setDetailProject(updated)
      await loadProjects()
    } catch (error) {
      const message = errorText(error)
      setEditError(message)
      notify('error', '更新项目失败', message, 8000)
    } finally {
      setEditBusy(false)
    }
  }

  async function handleOpenProject(project: Project): Promise<void> {
    try {
      await openProject(project.id)
    } catch (error) {
      notify('error', '打开项目失败', errorText(error), 8000)
    }
  }

  async function handleCloseProject(): Promise<void> {
    if (closingProject()) return
    setClosingProject(true)
    try {
      await closeProject()
    } catch (error) {
      notify('error', '关闭项目失败', errorText(error), 8000)
    } finally {
      setClosingProject(false)
    }
  }

  function requestArchive(project: Project): void {
    setPendingArchive(project)
  }

  function cancelArchive(): void {
    if (!confirmBusy()) setPendingArchive(null)
  }

  async function handleArchiveConfirm(): Promise<void> {
    const target = pendingArchive()
    if (!target || confirmBusy()) return
    setConfirmBusy(true)
    try {
      await api.projects.archive(target.id)
      notify('success', '项目已归档', target.name)
      setPendingArchive(null)
      if (detailProject()?.id === target.id) closeDetails()
      if (currentProject()?.id === target.id) {
        try {
          const refreshed = await api.projects.get(target.id)
          setCurrentProject(refreshed)
        } catch (error) {
          notify('error', '项目已归档，但当前项目状态刷新失败', errorText(error), 8000)
        }
      }
      await loadProjects()
    } catch (error) {
      notify('error', '归档项目失败', errorText(error), 8000)
    } finally {
      setConfirmBusy(false)
    }
  }

  function requestDelete(project: Project): void {
    setPendingDelete(project)
  }

  function cancelDelete(): void {
    if (!confirmBusy()) setPendingDelete(null)
  }

  async function handleDeleteConfirm(): Promise<void> {
    const target = pendingDelete()
    if (!target || confirmBusy()) return
    setConfirmBusy(true)
    try {
      await api.projects.remove(target.id)
      notify('success', '项目已删除', target.name)
      setPendingDelete(null)
      if (detailProject()?.id === target.id) closeDetails()
      if (currentProject()?.id === target.id) {
        try {
          await closeProject()
        } catch (error) {
          notify('error', '项目已删除，但清理项目会话失败', errorText(error), 8000)
        }
      }
      await loadProjects()
    } catch (error) {
      notify('error', '删除项目失败', errorText(error), 8000)
    } finally {
      setConfirmBusy(false)
    }
  }

  async function handleExport(project: Project): Promise<void> {
    try {
      const path = await api.system.selectSaveFile(
        packageFilename(project.name),
        'GLog 项目包',
        '*.zip',
      )
      if (!path) return
      const exportedPath = await api.projects.exportPackage(project.id, path)
      notify(
        'success',
        '项目包已导出',
        exportedPath || path,
        10000,
      )
    } catch (error) {
      notify('error', '导出项目失败', errorText(error), 8000)
    }
  }

  async function handleImport(): Promise<void> {
    if (importing()) return

    let selected: string[] = []
    try {
      selected = await api.system.selectFiles()
    } catch (error) {
      notify('error', '无法打开文件选择器', errorText(error), 8000)
      return
    }
    if (!Array.isArray(selected) || selected.length === 0) return

    const packages = selected.filter(isPackagePath)
    if (packages.length === 0) {
      notify('error', '未找到有效项目包', '请选择 .zip 或 .glogpkg 格式的项目包。', 7000)
      return
    }
    const packagePath = packages[0]
    if (!packagePath) return
    if (selected.length > 1) {
      notify('warn', '仅导入首个项目包', '一次处理一个项目包，其余选择已忽略。')
    }

    setImporting(true)
    try {
      const imported = await api.projects.importPackage(packagePath)
      notify('success', '项目包导入完成', imported.name)
      await loadProjects()
      try {
        await openProject(imported.id)
      } catch (error) {
        notify('error', '项目已导入，但打开失败', errorText(error), 8000)
      }
    } catch (error) {
      notify('error', '项目导入失败', errorText(error), 8000)
    } finally {
      setImporting(false)
    }
  }

  function openCurrentDetails(): void {
    const project = currentProject()
    if (project) showProjectDetails(project)
  }

  function showProjectDetails(project: Project): void {
    setDetailProject(project)
    setDetailStatistics(null)
    setDetailSources([])
    setDetailError('')
    setDetailOpen(true)
    const seq = ++detailSeq
    void loadProjectDetails(project, seq)
  }

  async function loadProjectDetails(project: Project, seq: number): Promise<void> {
    setDetailLoading(true)
    try {
      const [statisticsResult, sourcesResult] = await Promise.all([
        settle(api.projects.statistics(project.id)),
        settle(api.projects.sources(project.id)),
      ])
      if (seq !== detailSeq) return

      const failures: string[] = []
      if (statisticsResult.value !== null) {
        setDetailStatistics(statisticsResult.value)
      } else {
        setDetailStatistics(null)
        if (statisticsResult.error) {
          failures.push(`统计：${errorText(statisticsResult.error)}`)
        }
      }

      if (sourcesResult.value !== null) {
        setDetailSources(
          Array.isArray(sourcesResult.value) ? sourcesResult.value : [],
        )
      } else {
        setDetailSources([])
        if (sourcesResult.error) {
          failures.push(`来源：${errorText(sourcesResult.error)}`)
        }
      }

      if (failures.length > 0) {
        const message = failures.join('；')
        setDetailError(message)
        notify(
          'error',
          failures.length === 2 ? '项目详情加载失败' : '项目详情部分加载失败',
          message,
          8000,
        )
      } else {
        setDetailError('')
      }
    } catch (error) {
      if (seq !== detailSeq) return
      const message = errorText(error)
      setDetailError(message)
      notify('error', '项目详情加载失败', message, 8000)
    } finally {
      if (seq === detailSeq) setDetailLoading(false)
    }
  }

  function retryDetails(): void {
    const project = detailProject()
    if (!project) return
    setDetailError('')
    const seq = ++detailSeq
    void loadProjectDetails(project, seq)
  }

  function closeDetails(): void {
    detailSeq += 1
    setDetailOpen(false)
    setDetailLoading(false)
  }

  onMount(() => {
    void loadProjects()
  })

  onCleanup(() => {
    requestSeq += 1
    detailSeq += 1
    if (searchTimer != null) {
      window.clearTimeout(searchTimer)
      searchTimer = undefined
    }
  })

  return (
    <div class="page">
      <PageHeader
        eyebrow="工作区 / 项目档案"
        title="项目总览"
        subtitle="管理隔离的日志工作区、全文索引与调查证据。"
        actions={
          <>
            <Show when={currentProject()}>
              {(project) => (
                <div
                  class="row"
                  style={{
                    gap: '8px',
                    padding: '4px 8px 4px 10px',
                    border: '1px solid var(--accent)',
                    background: 'var(--accent-ghost)',
                    'clip-path': 'var(--clip-corner-sm)',
                  }}
                >
                  <span style={{ color: 'var(--accent)', display: 'inline-flex' }}>
                    <Icon name="database" size={13} />
                  </span>
                  <div style={{ 'min-width': '0', 'max-width': '170px' }}>
                    <div
                      class="mono-sm faint"
                      style={{ 'font-size': '8px', 'letter-spacing': '0.14em', 'line-height': '1.2' }}
                    >
                      当前已打开
                    </div>
                    <div
                      class="truncate"
                      style={{
                        'font-family': 'var(--font-display)',
                        'font-size': '12px',
                        'font-weight': '600',
                        'line-height': '1.25',
                      }}
                      title={project().name}
                    >
                      {project().name}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={closingProject()}
                    onClick={() => void handleCloseProject()}
                  >
                    <Icon name="x" size={11} />
                    关闭项目
                  </Button>
                </div>
              )}
            </Show>
            <Button
              variant="ghost"
              loading={importing()}
              title="导入 .zip / .glogpkg 项目包"
              onClick={() => void handleImport()}
            >
              <Icon name="package" size={12} />
              导入项目
            </Button>
            <Button variant="primary" onClick={openCreateModal}>
              <Icon name="plus" size={12} />
              新建项目
            </Button>
          </>
        }
      />

      <div class="stack">
        <div class="stat-grid">
          <StatTile
            label="项目档案"
            value={loaded() ? formatNumber(totals().projectCount) : '—'}
            note={`${formatNumber(totals().active)} 活跃 / ${formatNumber(totals().archived)} 归档`}
            tone="mint"
          />
          <StatTile
            label="日志总量"
            value={loaded() ? formatNumber(totals().logs) : '—'}
            note="当前筛选结果"
            tone="sky"
          />
          <StatTile
            label="占用空间"
            value={loaded() ? formatBytes(totals().bytes) : '—'}
            note="日志与索引存储"
            tone="amber"
          />
          <StatTile
            label="活跃任务"
            value={formatNumber(activeTasks().length)}
            note={activeTasks().length > 0 ? '采集 / 分析队列运行中' : '队列空闲'}
            tone={activeTasks().length > 0 ? 'amber' : 'mint'}
          />
          <StatTile
            label="当前项目"
            value={currentName()}
            note={currentNote()}
            tone={currentProject() ? 'mint' : undefined}
            onClick={currentProject() ? openCurrentDetails : undefined}
          />
        </div>

        <LocalLogDiscovery />

        <div class="panel">
          <div class="panel-head">
            <div class="panel-title">
              <span class="tick" />
              检索与排序
            </div>
            <div class="row" style={{ gap: '8px' }}>
              <span class="mono-sm faint">QUERY / ARCHIVE</span>
              <Show when={filtersActive()}>
                <Button size="sm" variant="ghost" onClick={resetFilters}>
                  <Icon name="x" size={11} />
                  清除筛选
                </Button>
              </Show>
            </div>
          </div>
          <div class="panel-body" style={{ padding: '12px 16px' }}>
            <div class="row wrap" style={{ 'align-items': 'flex-end', gap: '10px' }}>
              <div style={{ flex: '1 1 260px', 'min-width': '200px' }}>
                <Field label="检索" hint="名称 / 描述">
                  <TextInput
                    value={search()}
                    placeholder="输入项目名称或描述关键词…"
                    onInput={handleSearchInput}
                    onEnter={reloadNow}
                  />
                </Field>
              </div>
              <div style={{ flex: '0 0 142px', 'min-width': '122px' }}>
                <Field label="状态">
                  <Select
                    value={status()}
                    options={STATUS_OPTIONS}
                    onInput={handleStatusChange}
                  />
                </Field>
              </div>
              <div style={{ flex: '0 0 154px', 'min-width': '132px' }}>
                <Field label="排序字段">
                  <Select
                    value={sortBy()}
                    options={SORT_OPTIONS}
                    onInput={handleSortChange}
                  />
                </Field>
              </div>
              <div style={{ flex: '0 0 104px', 'min-width': '90px' }}>
                <Field label="顺序">
                  <Select
                    value={direction()}
                    options={DIRECTION_OPTIONS}
                    onInput={handleDirectionChange}
                  />
                </Field>
              </div>
              <Button
                variant="ghost"
                loading={loading()}
                disabled={loading()}
                onClick={reloadNow}
              >
                <Icon name="refresh" size={13} />
                刷新
              </Button>
            </div>
          </div>
        </div>

        <div class="panel">
          <div class="panel-head">
            <div class="panel-title">
              <span class="tick" />
              项目档案
            </div>
            <div class="row" style={{ gap: '10px' }}>
              <Show when={loading() && projects().length > 0}>
                <span class="row" style={{ gap: '6px' }}>
                  <Spinner />
                  <span class="mono-sm faint">同步中</span>
                </span>
              </Show>
              <span class="mono-sm faint">
                {loaded() ? `${formatNumber(projects().length)} 项` : '—'}
              </span>
            </div>
          </div>
          <div class="panel-body">
            <Show when={listError()}>
              <div
                class="danger-note"
                style={{ 'margin-bottom': projects().length > 0 ? '12px' : '0' }}
              >
                <div class="row" style={{ 'align-items': 'flex-start', gap: '8px' }}>
                  <span style={{ 'margin-top': '1px', display: 'inline-flex' }}>
                    <Icon name="alert" size={13} />
                  </span>
                  <div class="grow">
                    <div
                      style={{
                        'font-family': 'var(--font-display)',
                        'font-size': '12px',
                        'font-weight': '600',
                        'letter-spacing': '0.04em',
                      }}
                    >
                      项目档案读取失败
                    </div>
                    <div
                      class="mono-sm"
                      style={{ 'margin-top': '3px', 'line-height': '1.6', 'word-break': 'break-word' }}
                    >
                      {listError()}
                    </div>
                  </div>
                  <Button size="sm" variant="ghost" onClick={reloadNow}>
                    重试
                  </Button>
                </div>
              </div>
            </Show>

            <PageBusy
              visible={loading() && projects().length === 0}
              text={loaded() ? '正在刷新项目档案…' : '正在读取项目索引…'}
            />

            <Show when={projects().length > 0}>
              <div class="project-grid">
                <For each={projects()}>
                  {(project) => (
                    <ProjectCard
                      project={project}
                      active={currentProject()?.id === project.id}
                      sourceCount={sourceCounts()[project.id] ?? null}
                      onOpen={handleOpenProject}
                      onDetail={showProjectDetails}
                      onEdit={openEditModal}
                      onArchive={requestArchive}
                      onExport={handleExport}
                      onDelete={requestDelete}
                    />
                  )}
                </For>
              </div>
            </Show>

            <Show
              when={loaded() && !loading() && !listError() && projects().length === 0}
            >
              <EmptyState
                icon={filtersActive() ? 'search' : 'folder'}
                title={filtersActive() ? '未检索到匹配项目' : '尚无项目档案'}
                text={
                  filtersActive()
                    ? '调整关键词或状态筛选，或创建新的项目档案。'
                    : '创建项目以隔离日志、索引与调查证据，或从项目包恢复工作区。'
                }
                action={
                  <div
                    class="row wrap"
                    style={{ 'justify-content': 'center', gap: '8px' }}
                  >
                    <Show when={filtersActive()}>
                      <Button variant="ghost" onClick={resetFilters}>
                        清除筛选
                      </Button>
                    </Show>
                    <Button variant="primary" onClick={openCreateModal}>
                      <Icon name="plus" size={12} />
                      创建项目
                    </Button>
                    <Button
                      variant="ghost"
                      loading={importing()}
                      onClick={() => void handleImport()}
                    >
                      <Icon name="package" size={12} />
                      导入项目包
                    </Button>
                  </div>
                }
              />
            </Show>
          </div>
        </div>
      </div>

      <CreateProjectModal
        open={createOpen()}
        busy={createBusy()}
        error={createError()}
        onClose={closeCreateModal}
        onSubmit={handleCreate}
      />

      <EditProjectModal
        open={editTarget() !== null}
        project={editTarget()}
        busy={editBusy()}
        error={editError()}
        onClose={closeEditModal}
        onSubmit={handleEditSubmit}
      />

      <ProjectDetailDrawer
        open={detailOpen()}
        project={detailProject()}
        statistics={detailStatistics()}
        sources={detailSources()}
        loading={detailLoading()}
        error={detailError()}
        onClose={closeDetails}
        onReload={retryDetails}
        onOpen={(project) => {
          closeDetails()
          void handleOpenProject(project)
        }}
        onExport={handleExport}
        onArchive={requestArchive}
      />

      <ConfirmDialog
        open={pendingArchive() !== null}
        title="归档项目"
        message={`将项目「${pendingArchive()?.name ?? ''}」转入归档状态？归档后项目会从活跃视图中隐藏，日志与索引仍会保留。`}
        confirmText="确认归档"
        loading={confirmBusy()}
        onConfirm={() => void handleArchiveConfirm()}
        onCancel={cancelArchive}
      />

      <ConfirmDialog
        open={pendingDelete() !== null}
        title="删除项目"
        message={`将永久删除项目「${pendingDelete()?.name ?? ''}」及其日志目录与索引数据，此操作不可撤销。`}
        confirmText="永久删除"
        danger
        loading={confirmBusy()}
        onConfirm={() => void handleDeleteConfirm()}
        onCancel={cancelDelete}
      />
    </div>
  )
}
