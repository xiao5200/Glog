import {
  JSX,
  Show,
  createEffect,
  createMemo,
  createSignal,
  on,
  onCleanup,
} from 'solid-js'
import type { FieldFilter, LogEntry, LogSource, Query, QuerySnapshot } from '../types'
import { api, errorText } from '../api'
import {
  currentProject,
  navigate,
  notify,
  openLogDetail,
  openLogDetailWithAI,
} from '../stores/app'
import { Button, ConfirmDialog, EmptyState, Icon, IconButton, Spinner } from '../components/ui'
import { formatInt, parseTime, todayStamp, truncate } from '../utils'
import {
  localInputToIso,
  parseEventIDs,
  queryComparableKey,
  resolveTimeBounds,
  toLocalInput,
  toggleValue,
  type AIMode,
  type HostFacet,
  type SortField,
  type TimePreset,
} from '../components/log/logCommon'
import { LogToolbar } from '../components/log/LogToolbar'
import { FilterSidebar } from '../components/log/FilterSidebar'
import { LogVirtualList } from '../components/log/LogVirtualList'
import { LogAnalysisAIPanel } from '../components/log/LogAnalysisAIPanel'
import { LogDetailDrawer } from '../components/log/LogDetailDrawer'
import { ContextModal } from '../components/log/ContextModal'
import { ClusterModal } from '../components/log/ClusterModal'
import { SnapshotModal } from '../components/log/SnapshotModal'
import {
  AnomalyModal,
  NLQueryModal,
  ReportModal,
  RootCauseModal,
} from '../components/log/AIModals'
import { LogCharts } from '../components/charts/logCharts'

const PAGE_SIZES = [100, 200, 500]

function minuteKey(value: string | undefined): string {
  const date = parseTime(value)
  return date ? String(Math.floor(date.getTime() / 60000)) : value ?? ''
}

function normalizedQueryKey(query: Query): string {
  return queryComparableKey({
    ...query,
    start_time: minuteKey(query.start_time),
    end_time: minuteKey(query.end_time),
  })
}

function snapshotMatches(query: Query, current: Query): boolean {
  return normalizedQueryKey(query) === normalizedQueryKey(current)
}

export function LogViewPage(): JSX.Element {
  const [queryDraft, setQueryDraft] = createSignal('')
  const [appliedText, setAppliedText] = createSignal('')
  const [timePreset, setTimePreset] = createSignal<TimePreset>('all')
  const [customStart, setCustomStart] = createSignal('')
  const [customEnd, setCustomEnd] = createSignal('')
  const [timeAnchor, setTimeAnchor] = createSignal(Date.now())
  const [levels, setLevels] = createSignal<string[]>([])
  const [selectedHosts, setSelectedHosts] = createSignal<string[]>([])
  const [selectedSources, setSelectedSources] = createSignal<string[]>([])
  const [eventIDText, setEventIDText] = createSignal('')
  const [fieldFilters, setFieldFilters] = createSignal<FieldFilter[]>([])
  const [sortBy, setSortBy] = createSignal<SortField>('timestamp')
  const [sortOrder, setSortOrder] = createSignal<'asc' | 'desc'>('desc')
  const [limit, setLimit] = createSignal(200)

  const [rows, setRows] = createSignal<LogEntry[]>([])
  const [total, setTotal] = createSignal(0)
  const [took, setTook] = createSignal(0)
  const [hasMore, setHasMore] = createSignal(false)
  const [loading, setLoading] = createSignal(false)
  const [loadingMore, setLoadingMore] = createSignal(false)
  const [queryError, setQueryError] = createSignal<string | null>(null)
  const [listResetToken, setListResetToken] = createSignal(0)

  const [hostFacets, setHostFacets] = createSignal<HostFacet[]>([])
  const [hostsLoading, setHostsLoading] = createSignal(false)
  const [hostsError, setHostsError] = createSignal<string | null>(null)

  const [logSources, setLogSources] = createSignal<LogSource[]>([])
  const [sourcesLoading, setSourcesLoading] = createSignal(false)
  const [sourcesError, setSourcesError] = createSignal<string | null>(null)

  const [snapshots, setSnapshots] = createSignal<QuerySnapshot[]>([])
  const [snapshotsLoading, setSnapshotsLoading] = createSignal(false)
  const [snapshotsError, setSnapshotsError] = createSignal<string | null>(null)
  const [selectedSnapshotID, setSelectedSnapshotID] = createSignal('')

  const [showFilters, setShowFilters] = createSignal(true)
  const [showCharts, setShowCharts] = createSignal(false)
  const [chartReloadKey, setChartReloadKey] = createSignal(0)

  const [selectedDocId, setSelectedDocId] = createSignal<number | null>(null)
  const [expandedDocs, setExpandedDocs] = createSignal<ReadonlySet<number>>(new Set<number>())
  const [drawerEntry, setDrawerEntry] = createSignal<LogEntry | null>(null)
  const [contextDocID, setContextDocID] = createSignal<number | null>(null)

  const [clusterOpen, setClusterOpen] = createSignal(false)
  const [logAIPanelOpen, setLogAIPanelOpen] = createSignal(false)
  const [snapshotModalOpen, setSnapshotModalOpen] = createSignal(false)
  const [savingSnapshot, setSavingSnapshot] = createSignal(false)
  const [snapshotSaveError, setSnapshotSaveError] = createSignal<string | null>(null)
  const [deleteTarget, setDeleteTarget] = createSignal<QuerySnapshot | null>(null)
  const [deletingSnapshot, setDeletingSnapshot] = createSignal(false)
  const [aiMode, setAiMode] = createSignal<AIMode | null>(null)

  let searchSeq = 0
  let facetSeq = 0
  let sourcesSeq = 0
  let snapshotsSeq = 0
  let locateSeq = 0
  let autoTimer: number | undefined
  let facetTimer: number | undefined
  let searchSignature = ''
  let lastQueryErrorNotified = ''
  let lastRangeWarningKey = ''
  let lastFacetErrorNotified = ''
  let lastSourcesErrorNotified = ''
  let lastSnapshotsErrorNotified = ''

  const projectID = () => currentProject()?.id ?? ''

  const sourceCount = () => {
    const loaded = logSources().length
    return loaded > 0 ? loaded : currentProject()?.sources?.length ?? 0
  }

  function buildFilterQuery(): Query {
    const bounds = resolveTimeBounds(
      timePreset(),
      customStart(),
      customEnd(),
      timeAnchor(),
    )
    const ids = parseEventIDs(eventIDText())
    const query: Query = {}
    const text = appliedText().trim()
    if (text) query.text = text
    if (bounds.start) query.start_time = bounds.start
    if (bounds.end) query.end_time = bounds.end
    if (levels().length > 0) query.levels = levels().slice()
    if (selectedHosts().length > 0) query.hosts = selectedHosts().slice()
    if (selectedSources().length > 0) query.sources = selectedSources().slice()
    if (ids.length > 0) query.event_ids = ids
    if (fieldFilters().length > 0) {
      query.field_filters = fieldFilters().map((filter) => ({ ...filter }))
    }
    return query
  }

  function buildSearchQuery(offset: number): Query {
    return {
      ...buildFilterQuery(),
      sort_by: sortBy(),
      sort_order: sortOrder(),
      offset,
      limit: limit(),
    }
  }

  function buildSnapshotQuery(): Query {
    return {
      ...buildFilterQuery(),
      sort_by: sortBy(),
      sort_order: sortOrder(),
    }
  }

  const filterQuery = createMemo(() => buildFilterQuery())

  const filterSignature = createMemo(
    () =>
      `${queryComparableKey(filterQuery())}|${sortBy()}|${sortOrder()}|${limit()}`,
  )

  const facetSignature = createMemo(() => {
    const query = buildFilterQuery()
    delete query.hosts
    return queryComparableKey(query)
  })

  const activeFilterCount = createMemo(() => {
    let count = 0
    if (timePreset() !== 'all') count++
    if (levels().length > 0) count++
    if (selectedHosts().length > 0) count++
    if (selectedSources().length > 0) count++
    if (eventIDText().trim()) count++
    if (fieldFilters().length > 0) count++
    return count
  })

  const customRangeInvalid = createMemo(() => {
    if (timePreset() !== 'custom') return false
    const start = localInputToIso(customStart())
    const end = localInputToIso(customEnd())
    if (!start || !end) return false
    return new Date(start).getTime() > new Date(end).getTime()
  })

  const defaultSnapshotName = () => {
    const text = appliedText().trim()
    return `${text ? truncate(text, 26) : '全部日志'} · ${todayStamp()}`
  }

  const resultSummary = () => {
    const parts: string[] = []
    const text = appliedText().trim()
    parts.push(text ? `“${truncate(text, 44)}”` : '全部日志')
    if (activeFilterCount() > 0) parts.push(`${activeFilterCount()} 项筛选`)
    if (sortBy() === 'level') parts.push(`按级别${sortOrder() === 'asc' ? '升序' : '降序'}`)
    return parts.join(' · ')
  }

  async function loadSources(id: string) {
    if (!id || currentProject()?.id !== id) return
    const seq = ++sourcesSeq
    setSourcesLoading(true)
    setSourcesError(null)
    try {
      const list = await api.projects.sources(id)
      if (seq !== sourcesSeq || currentProject()?.id !== id) return
      setLogSources(Array.isArray(list) ? list : [])
      setSourcesError(null)
      lastSourcesErrorNotified = ''
    } catch (err) {
      if (seq !== sourcesSeq) return
      const text = errorText(err)
      setSourcesError(text)
      if (text !== lastSourcesErrorNotified) {
        notify('error', '日志源加载失败', text, 6000)
        lastSourcesErrorNotified = text
      }
    } finally {
      if (seq === sourcesSeq) setSourcesLoading(false)
    }
  }

  async function loadSnapshots(id: string) {
    if (!id || currentProject()?.id !== id) return
    const seq = ++snapshotsSeq
    setSnapshotsLoading(true)
    setSnapshotsError(null)
    try {
      const list = await api.query.snapshots(id)
      if (seq !== snapshotsSeq || currentProject()?.id !== id) return
      const items = Array.isArray(list) ? list : []
      setSnapshots(items)
      setSnapshotsError(null)
      lastSnapshotsErrorNotified = ''
      const selected = selectedSnapshotID()
      if (selected && !items.some((item) => item.id === selected)) {
        setSelectedSnapshotID('')
      }
    } catch (err) {
      if (seq !== snapshotsSeq) return
      const text = errorText(err)
      setSnapshotsError(text)
      if (text !== lastSnapshotsErrorNotified) {
        notify('error', '查询快照加载失败', text, 6000)
        lastSnapshotsErrorNotified = text
      }
    } finally {
      if (seq === snapshotsSeq) setSnapshotsLoading(false)
    }
  }

  async function loadHostFacets() {
    const project = currentProject()
    if (!project) return
    if (customRangeInvalid()) return
    const seq = ++facetSeq
    setHostsLoading(true)
    setHostsError(null)
    try {
      const query = buildFilterQuery()
      delete query.hosts
      const result = await api.query.aggregate(project.id, {
        query,
        aggregate_by: ['source_host'],
        top_n: 50,
      })
      if (seq !== facetSeq) return
      const raw = result.distribution?.length
        ? result.distribution
        : result.top_items ?? []
      const merged = new Map<string, number>()
      for (const item of raw) {
        const host = String(item.key ?? '').trim()
        if (!host) continue
        merged.set(host, (merged.get(host) ?? 0) + (Number(item.count) || 0))
      }
      const facets = Array.from(merged.entries())
        .map(([host, count]) => ({ host, count }))
        .sort((a, b) => b.count - a.count)
        .slice(0, 50)
      setHostFacets(facets)
      setHostsError(null)
      lastFacetErrorNotified = ''
    } catch (err) {
      if (seq !== facetSeq) return
      const text = errorText(err)
      setHostsError(text)
      if (text !== lastFacetErrorNotified) {
        notify('error', '主机聚合失败', text, 5000)
        lastFacetErrorNotified = text
      }
    } finally {
      if (seq === facetSeq) setHostsLoading(false)
    }
  }

  async function runSearch(options: { force?: boolean } = {}) {
    const project = currentProject()
    if (!project) return

    if (customRangeInvalid()) {
      searchSeq++
      setLoading(false)
      setLoadingMore(false)
      setQueryError('自定义时间范围无效：开始时间晚于结束时间。')
      const key = `${customStart()}|${customEnd()}`
      if (key !== lastRangeWarningKey) {
        notify('warn', '时间范围无效', '开始时间晚于结束时间，请调整后再查询。', 6000)
        lastRangeWarningKey = key
      }
      return
    }

    const query = buildSearchQuery(0)
    const signature = `${queryComparableKey(query)}|${query.sort_by}|${query.sort_order}|${query.limit}`
    if (!options.force && signature === searchSignature) return

    searchSignature = signature
    const seq = ++searchSeq
    setLoading(true)
    setLoadingMore(false)
    setQueryError(null)
    setListResetToken((value) => value + 1)

    try {
      const result = await api.query.search(project.id, query)
      if (seq !== searchSeq) return
      const hits = Array.isArray(result.hits) ? result.hits : []
      setRows(hits)
      setTotal(Number(result.total) || 0)
      setTook(Number(result.took) || 0)
      setHasMore(Boolean(result.has_more))
      setQueryError(null)
      lastQueryErrorNotified = ''
      lastRangeWarningKey = ''

      const selected = selectedDocId()
      if (selected != null && !hits.some((entry) => entry.doc_id === selected)) {
        setSelectedDocId(null)
      }

      const open = drawerEntry()
      if (open) {
        const fresh = hits.find((entry) => entry.doc_id === open.doc_id)
        if (fresh) setDrawerEntry(fresh)
      }

      setExpandedDocs((previous) => {
        const next = new Set<number>()
        for (const docID of previous) {
          if (hits.some((entry) => entry.doc_id === docID)) next.add(docID)
        }
        return next
      })
    } catch (err) {
      if (seq !== searchSeq) return
      const text = errorText(err)
      setQueryError(text)
      if (text !== lastQueryErrorNotified) {
        notify('error', '日志查询失败', text, 7000)
        lastQueryErrorNotified = text
      }
    } finally {
      if (seq === searchSeq) setLoading(false)
    }
  }

  async function loadMore() {
    const project = currentProject()
    if (!project || loading() || loadingMore() || !hasMore()) return
    if (customRangeInvalid()) return
    const seq = searchSeq
    const offset = rows().length
    setLoadingMore(true)
    try {
      const result = await api.query.search(project.id, buildSearchQuery(offset))
      if (seq !== searchSeq) return
      const hits = Array.isArray(result.hits) ? result.hits : []
      setRows((previous) => {
        const seen = new Set(previous.map((entry) => entry.doc_id))
        const appended = hits.filter((entry) => !seen.has(entry.doc_id))
        return appended.length > 0 ? [...previous, ...appended] : previous
      })
      setTotal(Number(result.total) || 0)
      setTook(Number(result.took) || 0)
      setHasMore(Boolean(result.has_more))
      setQueryError(null)
    } catch (err) {
      if (seq !== searchSeq) return
      const text = errorText(err)
      setQueryError(text)
      notify('error', '加载更多日志失败', text, 6000)
    } finally {
      if (seq === searchSeq) setLoadingMore(false)
    }
  }

  function executeSearch() {
    setAppliedText(queryDraft())
    setSelectedSnapshotID('')
    setTimeAnchor(Date.now())
    void runSearch({ force: true })
  }

  function retrySearch() {
    setTimeAnchor(Date.now())
    void runSearch({ force: true })
  }

  function resetFilters(includeText: boolean) {
    if (includeText) {
      setQueryDraft('')
      setAppliedText('')
    }
    setTimePreset('all')
    setCustomStart('')
    setCustomEnd('')
    setLevels([])
    setSelectedHosts([])
    setSelectedSources([])
    setEventIDText('')
    setFieldFilters([])
    setSelectedSnapshotID('')
    setTimeAnchor(Date.now())
  }

  function resetAll() {
    resetFilters(true)
    setSortBy('timestamp')
    setSortOrder('desc')
    setLimit(200)
    void runSearch({ force: true })
  }

  function clearFilters() {
    resetFilters(false)
    void runSearch({ force: true })
  }

  function handleTimePresetChange(value: TimePreset) {
    setTimePreset(value)
    setTimeAnchor(Date.now())
  }

  function toggleLevel(level: string) {
    setLevels((previous) => toggleValue(previous, level))
  }

  function toggleHost(host: string) {
    setSelectedHosts((previous) => toggleValue(previous, host))
  }

  function toggleSource(id: string) {
    setSelectedSources((previous) => toggleValue(previous, id))
  }

  function handleSort(field: SortField) {
    if (sortBy() === field) {
      setSortOrder((previous) => (previous === 'desc' ? 'asc' : 'desc'))
    } else {
      setSortBy(field)
      setSortOrder('desc')
    }
  }

  function handleLimitChange(value: number) {
    if (!PAGE_SIZES.includes(value)) return
    setLimit(value)
  }

  function openEntry(entry: LogEntry) {
    setSelectedDocId(entry.doc_id)
    setDrawerEntry(entry)
  }

  function closeDrawer() {
    setDrawerEntry(null)
    setSelectedDocId(null)
  }

  function toggleExpand(entry: LogEntry) {
    setExpandedDocs((previous) => {
      const next = new Set(previous)
      if (next.has(entry.doc_id)) next.delete(entry.doc_id)
      else next.add(entry.doc_id)
      return next
    })
  }

  function handleEntryUpdated(updated: LogEntry) {
    setDrawerEntry(updated)
    setRows((previous) =>
      previous.map((entry) =>
        entry.doc_id === updated.doc_id ? { ...entry, ...updated } : entry,
      ),
    )
  }

  async function openDocByID(docID: number) {
    const project = currentProject()
    if (!project) return
    const seq = ++locateSeq
    try {
      const entry = await api.query.getEntry(project.id, docID)
      if (seq !== locateSeq) return
      if (!entry) {
        notify('warn', '日志不存在', `未找到 doc #${docID}，可能已被删除或索引已重建。`)
        return
      }
      openEntry(entry)
    } catch (err) {
      if (seq !== locateSeq) return
      notify('error', '打开日志失败', errorText(err), 7000)
    }
  }

  function applyQueryToState(query: Query) {
    setQueryDraft(query.text ?? '')
    setAppliedText(query.text ?? '')
    setLevels(query.levels ? query.levels.slice() : [])
    setSelectedHosts(query.hosts ? query.hosts.slice() : [])
    setSelectedSources(query.sources ? query.sources.slice() : [])
    setEventIDText(
      query.event_ids && query.event_ids.length > 0
        ? query.event_ids.join(', ')
        : '',
    )
    setFieldFilters(
      query.field_filters
        ? query.field_filters.map((filter) => ({ ...filter }))
        : [],
    )
    if (query.start_time || query.end_time) {
      setTimePreset('custom')
      setCustomStart(toLocalInput(query.start_time))
      setCustomEnd(toLocalInput(query.end_time))
    } else {
      setTimePreset('all')
      setCustomStart('')
      setCustomEnd('')
    }
    if (query.sort_by === 'level' || query.sort_by === 'timestamp') {
      setSortBy(query.sort_by)
    }
    if (query.sort_order === 'asc' || query.sort_order === 'desc') {
      setSortOrder(query.sort_order)
    }
    const requestedLimit = Number(query.limit)
    if (PAGE_SIZES.includes(requestedLimit)) setLimit(requestedLimit)
    setTimeAnchor(Date.now())
  }

  function handleSelectSnapshot(id: string) {
    if (!id) {
      setSelectedSnapshotID('')
      return
    }
    const snapshot = snapshots().find((item) => item.id === id)
    if (!snapshot) {
      setSelectedSnapshotID('')
      notify('warn', '快照不可用', '未找到该查询快照，可能已被删除。')
      const project = currentProject()
      if (project) void loadSnapshots(project.id)
      return
    }
    applyQueryToState(snapshot.query)
    setSelectedSnapshotID(id)
    setTimeAnchor(Date.now())
    void runSearch({ force: true })
    notify('info', '已应用查询快照', snapshot.name)
  }

  function openSnapshotDialog() {
    if (!currentProject()) {
      notify('warn', '未选择项目', '请先打开项目后再保存查询快照。')
      return
    }
    setSnapshotSaveError(null)
    setSnapshotModalOpen(true)
  }

  async function saveSnapshot(name: string, description: string) {
    const project = currentProject()
    if (!project) return
    setSavingSnapshot(true)
    setSnapshotSaveError(null)
    try {
      const snapshot: QuerySnapshot = {
        id: '',
        project_id: project.id,
        name,
        description,
        query: buildSnapshotQuery(),
        result_count: total(),
        created_at: new Date().toISOString(),
      }
      await api.query.saveSnapshot(snapshot)
      notify('success', '查询快照已保存', name)
      setSnapshotModalOpen(false)
      await loadSnapshots(project.id)
    } catch (err) {
      const text = errorText(err)
      setSnapshotSaveError(text)
      notify('error', '保存快照失败', text, 7000)
    } finally {
      setSavingSnapshot(false)
    }
  }

  function requestDeleteSnapshot(id: string) {
    const snapshot = snapshots().find((item) => item.id === id)
    if (!snapshot) return
    setDeleteTarget(snapshot)
  }

  async function confirmDeleteSnapshot() {
    const target = deleteTarget()
    if (!target) return
    setDeletingSnapshot(true)
    try {
      await api.query.deleteSnapshot(target.id)
      notify('success', '快照已删除', target.name)
      if (selectedSnapshotID() === target.id) setSelectedSnapshotID('')
      setDeleteTarget(null)
      const project = currentProject()
      if (project) await loadSnapshots(project.id)
    } catch (err) {
      notify('error', '删除快照失败', errorText(err), 7000)
    } finally {
      setDeletingSnapshot(false)
    }
  }

  function handleAI(mode: AIMode) {
    if (!currentProject()) {
      notify('warn', '未选择项目', '请先打开项目后再使用 AI 分析。')
      return
    }
    setAiMode(mode)
  }

  function goAISettings() {
    setAiMode(null)
    navigate('settings')
  }

  function applyAIQuery(query: Query) {
    setAiMode(null)
    applyQueryToState(query)
    setSelectedSnapshotID('')
    setTimeAnchor(Date.now())
    void runSearch({ force: true })
    notify('success', 'AI 查询已应用', '已按生成的查询条件执行搜索。')
  }

  function refreshAll() {
    const project = currentProject()
    if (!project) return
    setTimeAnchor(Date.now())
    void loadSources(project.id)
    void loadSnapshots(project.id)
    void loadHostFacets()
    void runSearch({ force: true })
    setChartReloadKey((value) => value + 1)
  }

  createEffect(
    on(
      filterSignature,
      () => {
        if (!currentProject()) return
        if (autoTimer) window.clearTimeout(autoTimer)
        autoTimer = window.setTimeout(() => {
          autoTimer = undefined
          void runSearch()
        }, 300)
      },
      { defer: true },
    ),
  )

  createEffect(
    on(
      facetSignature,
      () => {
        if (!currentProject()) return
        if (facetTimer) window.clearTimeout(facetTimer)
        facetTimer = window.setTimeout(() => {
          facetTimer = undefined
          void loadHostFacets()
        }, 400)
      },
      { defer: true },
    ),
  )

  createEffect(
    on(filterSignature, () => {
      const id = selectedSnapshotID()
      if (!id) return
      const snapshot = snapshots().find((item) => item.id === id)
      if (!snapshot) return
      if (!snapshotMatches(snapshot.query, filterQuery())) {
        setSelectedSnapshotID('')
      }
    }),
  )

  createEffect(
    on(
      () => currentProject()?.id,
      (id, previous) => {
        if (!id) {
          searchSeq++
          facetSeq++
          sourcesSeq++
          snapshotsSeq++
          locateSeq++
          setRows([])
          setTotal(0)
          setTook(0)
          setHasMore(false)
          setLoading(false)
          setLoadingMore(false)
          setQueryError(null)
          setHostFacets([])
          setHostsLoading(false)
          setHostsError(null)
          setLogSources([])
          setSourcesLoading(false)
          setSourcesError(null)
          setSnapshots([])
          setSnapshotsLoading(false)
          setSnapshotsError(null)
          setSelectedSnapshotID('')
          setDrawerEntry(null)
          setSelectedDocId(null)
          setContextDocID(null)
          setClusterOpen(false)
          setLogAIPanelOpen(false)
          setSnapshotModalOpen(false)
          setDeleteTarget(null)
          setAiMode(null)
          setExpandedDocs(new Set<number>())
          return
        }

        if (previous !== undefined && previous !== id) {
          setQueryDraft('')
          setAppliedText('')
          setTimePreset('all')
          setCustomStart('')
          setCustomEnd('')
          setLevels([])
          setSelectedHosts([])
          setSelectedSources([])
          setEventIDText('')
          setFieldFilters([])
          setSortBy('timestamp')
          setSortOrder('desc')
          setLimit(200)
          setSelectedSnapshotID('')
          setRows([])
          setTotal(0)
          setTook(0)
          setHasMore(false)
          setQueryError(null)
          // 立即移除上一个项目的来源/主机/快照，避免新项目打开瞬间显示旧数据。
          setLogSources([])
          setHostFacets([])
          setSnapshots([])
          setSourcesLoading(false)
          setHostsLoading(false)
          setSnapshotsLoading(false)
          setSourcesError(null)
          setHostsError(null)
          setSnapshotsError(null)
          setDrawerEntry(null)
          setSelectedDocId(null)
          setContextDocID(null)
          setClusterOpen(false)
          setLogAIPanelOpen(false)
          setSnapshotModalOpen(false)
          setDeleteTarget(null)
          setAiMode(null)
          setExpandedDocs(new Set<number>())
        }

        searchSignature = ''
        lastQueryErrorNotified = ''
        lastRangeWarningKey = ''
        lastFacetErrorNotified = ''
        lastSourcesErrorNotified = ''
        lastSnapshotsErrorNotified = ''

        void loadSources(id)
        void loadSnapshots(id)
        void loadHostFacets()
        void runSearch({ force: true })
      },
    ),
  )

  onCleanup(() => {
    if (autoTimer) window.clearTimeout(autoTimer)
    if (facetTimer) window.clearTimeout(facetTimer)
    searchSeq++
    facetSeq++
    sourcesSeq++
    snapshotsSeq++
    locateSeq++
  })

  return (
    <>
      <Show
        when={currentProject()}
        fallback={
          <div
            class="page wide"
            style={{
              height: '100%',
              'min-height': 0,
              padding: '16px 20px',
              display: 'flex',
              'flex-direction': 'column',
              gap: '10px',
            }}
          >
            <header
              style={{
                flex: 'none',
                display: 'flex',
                'align-items': 'center',
                'padding-bottom': '7px',
                'border-bottom': '1px solid var(--border)',
              }}
            >
              <div class="grow">
                <div class="eyebrow" style={{ 'margin-bottom': '2px' }}>
                  日志分析 / 检索工作台
                </div>
                <h1 class="page-title" style={{ 'font-size': '20px', 'line-height': '1.1' }}>
                  日志分析工作台
                </h1>
              </div>
            </header>
            <div
              class="panel"
              style={{
                flex: 1,
                'min-height': 0,
                display: 'grid',
                'place-items': 'center',
              }}
            >
              <EmptyState
                icon="database"
                title="未打开项目"
                text="日志分析需要先打开一个项目。创建或导入项目后，即可检索、筛选、聚合与 AI 分析。"
                action={
                  <Button variant="primary" onClick={() => navigate('projects')}>
                    选择项目
                  </Button>
                }
              />
            </div>
          </div>
        }
      >
        <div
          class="page wide"
          style={{
            height: '100%',
            'min-height': 0,
            padding: '14px 18px 12px',
            display: 'flex',
            'flex-direction': 'column',
            gap: '9px',
            overflow: 'hidden',
          }}
        >
          <header
            style={{
              flex: 'none',
              display: 'flex',
              'align-items': 'center',
              gap: '12px',
              'padding-bottom': '7px',
              'border-bottom': '1px solid var(--border)',
              position: 'relative',
            }}
          >
            <div class="grow" style={{ 'min-width': 0 }}>
              <div class="eyebrow" style={{ 'margin-bottom': '2px' }}>
                日志分析 / 检索工作台
              </div>
              <div
                class="row"
                style={{ gap: '10px', 'align-items': 'baseline', 'flex-wrap': 'wrap' }}
              >
                <h1
                  class="page-title"
                  style={{ 'font-size': '20px', 'line-height': '1.1', 'white-space': 'nowrap' }}
                >
                  日志分析工作台
                </h1>
                <span
                  class="mono-sm faint truncate"
                  title={`${currentProject()?.name ?? ''} · ${formatInt(currentProject()?.log_count)} 条日志 · ${sourceCount()} 个来源`}
                >
                  {currentProject()?.name ?? '—'} · {formatInt(currentProject()?.log_count)} 条 ·{' '}
                  {sourceCount()} 源
                </span>
              </div>
            </div>
            <div class="page-actions" style={{ gap: '6px', 'flex-wrap': 'nowrap' }}>
              <span class="mono-sm faint" style={{ 'font-size': '9px', 'white-space': 'nowrap' }}>
                ↑↓ 选择 · Enter 详情
              </span>
              <Button
                size="sm"
                variant="primary"
                title="按日志类型与时间范围进行聚合或逐条 AI 分析"
                onClick={() => setLogAIPanelOpen(true)}
              >
                <Icon name="wand" size={12} />
                AI 分析
              </Button>
              <IconButton
                icon="refresh"
                title="刷新来源、快照、聚合与查询"
                disabled={loading()}
                onClick={refreshAll}
              />
            </div>
          </header>

          <LogToolbar
            loading={loading()}
            queryText={queryDraft()}
            onQueryTextChange={setQueryDraft}
            onSearch={executeSearch}
            onReset={resetAll}
            timePreset={timePreset()}
            customStart={customStart()}
            customEnd={customEnd()}
            onTimePresetChange={handleTimePresetChange}
            onCustomStartChange={setCustomStart}
            onCustomEndChange={setCustomEnd}
            levels={levels()}
            onToggleLevel={toggleLevel}
            showFilters={showFilters()}
            onToggleFilters={() => setShowFilters((value) => !value)}
            showCharts={showCharts()}
            onToggleCharts={() => setShowCharts((value) => !value)}
            onCluster={() => {
              if (!currentProject()) return
              setClusterOpen(true)
            }}
            snapshots={snapshots()}
            snapshotsLoading={snapshotsLoading()}
            snapshotsError={snapshotsError()}
            selectedSnapshotID={selectedSnapshotID()}
            onSelectSnapshot={handleSelectSnapshot}
            onSaveSnapshot={openSnapshotDialog}
            onDeleteSnapshot={requestDeleteSnapshot}
            onAI={handleAI}
            fieldFilterCount={fieldFilters().length}
            onClearFieldFilters={() => setFieldFilters([])}
            total={total()}
            took={took()}
          />

          <div
            class="split"
            style={{
              flex: 1,
              'min-height': 0,
              height: '100%',
              'align-items': 'stretch',
              gap: '10px',
              ...(showFilters() ? {} : { 'grid-template-columns': 'minmax(0, 1fr)' }),
            }}
          >
            <Show when={showFilters()}>
              <div style={{ height: '100%', 'min-height': 0, 'min-width': 0 }}>
                <FilterSidebar
                  timePreset={timePreset()}
                  customStart={customStart()}
                  customEnd={customEnd()}
                  levels={levels()}
                  hosts={hostFacets()}
                  hostsLoading={hostsLoading()}
                  hostsError={hostsError()}
                  selectedHosts={selectedHosts()}
                  sources={logSources()}
                  sourcesLoading={sourcesLoading()}
                  sourcesError={sourcesError()}
                  selectedSources={selectedSources()}
                  eventIDText={eventIDText()}
                  activeCount={activeFilterCount()}
                  extraFilterCount={fieldFilters().length}
                  onTimePresetChange={handleTimePresetChange}
                  onCustomStartChange={setCustomStart}
                  onCustomEndChange={setCustomEnd}
                  onToggleLevel={toggleLevel}
                  onToggleHost={toggleHost}
                  onToggleSource={toggleSource}
                  onEventIDChange={setEventIDText}
                  onClear={clearFilters}
                  onRetryHosts={() => void loadHostFacets()}
                  onRetrySources={() => {
                    const project = currentProject()
                    if (project) void loadSources(project.id)
                  }}
                  onCollapse={() => setShowFilters(false)}
                />
              </div>
            </Show>

            <div
              style={{
                height: '100%',
                'min-height': 0,
                'min-width': 0,
                display: 'flex',
                'flex-direction': 'column',
                gap: '10px',
              }}
            >
              <section
                class="panel"
                style={{
                  flex: 1,
                  'min-height': 0,
                  display: 'flex',
                  'flex-direction': 'column',
                  overflow: 'hidden',
                }}
              >
                <div
                  class="panel-head"
                  style={{ 'min-height': '32px', padding: '5px 10px', 'flex': 'none' }}
                >
                  <div class="panel-title">
                    <span class="tick" />
                    查询结果
                    <Show when={loading()}>
                      <Spinner />
                    </Show>
                  </div>
                  <div class="row" style={{ gap: '7px', 'min-width': 0 }}>
                    <span
                      class="mono-sm faint truncate"
                      style={{ 'max-width': '42vw' }}
                      title={resultSummary()}
                    >
                      {resultSummary()}
                    </span>
                    <IconButton
                      icon="refresh"
                      title="重新执行查询"
                      disabled={loading()}
                      onClick={retrySearch}
                    />
                  </div>
                </div>

                <LogVirtualList
                  rows={rows()}
                  total={total()}
                  hasMore={hasMore()}
                  loading={loading()}
                  loadingMore={loadingMore()}
                  error={queryError()}
                  selectedDocId={selectedDocId()}
                  expandedDocIds={expandedDocs()}
                  sortBy={sortBy()}
                  sortOrder={sortOrder()}
                  limit={limit()}
                  resetToken={listResetToken()}
                  projectLogCount={currentProject()?.log_count ?? 0}
                  onSort={handleSort}
                  onOpen={openEntry}
                  onOpenPage={(entry) => openLogDetail(entry.doc_id)}
                  onAnalyze={(entry) => openLogDetailWithAI(entry.doc_id)}
                  onHighlight={(entry) => setSelectedDocId(entry.doc_id)}
                  onToggleExpand={toggleExpand}
                  onLoadMore={() => void loadMore()}
                  onLimitChange={handleLimitChange}
                  onRetry={retrySearch}
                  onCollect={() => navigate('collect')}
                  onClearFilters={clearFilters}
                />
              </section>

              <Show when={showCharts()}>
                <LogCharts
                  projectID={projectID()}
                  query={filterQuery()}
                  reloadKey={chartReloadKey()}
                  onCollapse={() => setShowCharts(false)}
                />
              </Show>
            </div>
          </div>
        </div>
      </Show>

      <LogDetailDrawer
        open={drawerEntry() !== null}
        projectID={projectID()}
        entry={drawerEntry()}
        onClose={closeDrawer}
        onOpenContext={(entry) => setContextDocID(entry.doc_id)}
        onOpenFullLog={(entry) => openLogDetail(entry.doc_id)}
        onEntryUpdated={handleEntryUpdated}
      />

      <ContextModal
        open={contextDocID() !== null}
        projectID={projectID()}
        docID={contextDocID()}
        onClose={() => setContextDocID(null)}
        onSelectEntry={(entry) => {
          setContextDocID(null)
          openEntry(entry)
        }}
      />

      <ClusterModal
        open={clusterOpen()}
        projectID={projectID()}
        query={filterQuery()}
        onClose={() => setClusterOpen(false)}
      />

      <SnapshotModal
        open={snapshotModalOpen()}
        query={filterQuery()}
        resultCount={total()}
        defaultName={defaultSnapshotName()}
        saving={savingSnapshot()}
        error={snapshotSaveError()}
        onClose={() => {
          if (!savingSnapshot()) setSnapshotModalOpen(false)
        }}
        onSave={(name, description) => void saveSnapshot(name, description)}
      />

      <AnomalyModal
        open={aiMode() === 'anomaly'}
        projectID={projectID()}
        query={filterQuery()}
        onClose={() => setAiMode(null)}
        onOpenSettings={goAISettings}
        onLocate={(docID) => {
          setAiMode(null)
          void openDocByID(docID)
        }}
      />

      <NLQueryModal
        open={aiMode() === 'nlquery'}
        projectID={projectID()}
        onClose={() => setAiMode(null)}
        onOpenSettings={goAISettings}
        onApply={applyAIQuery}
      />

      <RootCauseModal
        open={aiMode() === 'rootcause'}
        projectID={projectID()}
        query={filterQuery()}
        onClose={() => setAiMode(null)}
        onOpenSettings={goAISettings}
      />

      <ReportModal
        open={aiMode() === 'report'}
        projectID={projectID()}
        query={filterQuery()}
        onClose={() => setAiMode(null)}
        onOpenSettings={goAISettings}
      />

      <LogAnalysisAIPanel
        open={logAIPanelOpen()}
        projectID={projectID()}
        projectName={currentProject()?.name}
        onClose={() => setLogAIPanelOpen(false)}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget())}
        title="删除查询快照"
        message={`确认删除查询快照“${deleteTarget()?.name ?? ''}”？此操作不可撤销。`}
        confirmText="删除快照"
        danger
        loading={deletingSnapshot()}
        onConfirm={() => void confirmDeleteSnapshot()}
        onCancel={() => {
          if (!deletingSnapshot()) setDeleteTarget(null)
        }}
      />
    </>
  )
}
