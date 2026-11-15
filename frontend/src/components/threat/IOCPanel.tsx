import { For, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { IOCItem, IOCSearchRequest, IOCSearchResult, LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatDuration, formatInt, formatTimeShort, truncate } from '../../utils'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Icon,
  IconButton,
  LevelBadge,
  PageBusy,
  TextArea,
  TextInput,
} from '../ui'

export interface IOCPanelProps {
  projectId: string
  onOpenLog: (docId: number, entry?: LogEntry, note?: string) => void
}

interface ImportSummary {
  files: number
  imported: number
  skipped: number
  errors: string[]
  formats: string[]
}

type ThreatTone = 'mint' | 'amber' | 'ember' | 'sky' | 'muted'

function threatTone(level: string | undefined): ThreatTone {
  const value = String(level ?? '').toLowerCase()
  if (value === 'critical' || value === 'high' || value === 'malicious') return 'ember'
  if (value === 'medium' || value === 'suspicious' || value === 'warn') return 'amber'
  if (value === 'low') return 'sky'
  if (value === 'info' || value === 'benign') return 'mint'
  return 'muted'
}

function toIso(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}

export function IOCPanel(props: IOCPanelProps) {
  const [iocs, setIocs] = createSignal<IOCItem[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')

  const [filter, setFilter] = createSignal('')
  const [selected, setSelected] = createSignal<Set<string>>(new Set())

  const [importing, setImporting] = createSignal(false)
  const [importSummary, setImportSummary] = createSignal<ImportSummary | null>(null)

  const [deleteTarget, setDeleteTarget] = createSignal<IOCItem | null>(null)
  const [deleting, setDeleting] = createSignal(false)

  const [manual, setManual] = createSignal('')
  const [startTime, setStartTime] = createSignal('')
  const [endTime, setEndTime] = createSignal('')
  const [searchBusy, setSearchBusy] = createSignal(false)
  const [searchError, setSearchError] = createSignal('')
  const [searchResult, setSearchResult] = createSignal<IOCSearchResult | null>(null)

  let loadSeq = 0

  const loadIOCs = async () => {
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.threat.iocs()
      if (seq !== loadSeq) return
      const safe = Array.isArray(list) ? list : []
      setIocs(safe)
      setSelected((prev) => {
        const available = new Set(safe.map((item) => item.id))
        return new Set(Array.from(prev).filter((id) => available.has(id)))
      })
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载 IOC 失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const projectID = props.projectId
    if (!projectID) return
    setFilter('')
    setSelected(new Set<string>())
    setImportSummary(null)
    setSearchResult(null)
    setSearchError('')
    void loadIOCs()
  })

  const visibleIocs = createMemo(() => {
    const query = filter().trim().toLowerCase()
    if (!query) return iocs()
    return iocs().filter((item) =>
      [item.value, item.type, item.description, item.source, item.threat_level]
        .map((value) => String(value ?? '').toLowerCase())
        .join('\n')
        .includes(query),
    )
  })

  const allVisibleSelected = createMemo(() => {
    const list = visibleIocs()
    return list.length > 0 && list.every((item) => selected().has(item.id))
  })

  const toggleOne = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleAllVisible = () => {
    const list = visibleIocs()
    setSelected((prev) => {
      const next = new Set(prev)
      const every = list.length > 0 && list.every((item) => next.has(item.id))
      for (const item of list) {
        if (every) next.delete(item.id)
        else next.add(item.id)
      }
      return next
    })
  }

  const manualValues = createMemo(() =>
    manual()
      .split(/[\s,;，；]+/)
      .map((value) => value.trim())
      .filter(Boolean),
  )

  const searchValues = createMemo(() =>
    Array.from(
      new Set([
        ...iocs()
          .filter((item) => selected().has(item.id))
          .map((item) => item.value),
        ...manualValues(),
      ]),
    ),
  )

  const previewErrors = createMemo(() => (importSummary()?.errors ?? []).slice(0, 5))

  const pickAndImport = async () => {
    let files: string[] = []
    try {
      files = await api.system.selectFiles()
    } catch (err) {
      notify('error', '选择 IOC 文件失败', errorText(err))
      return
    }
    if (!files || files.length === 0) return

    setImporting(true)
    setImportSummary(null)
    let imported = 0
    let skipped = 0
    const errors: string[] = []
    const formats = new Set<string>()

    for (const file of files) {
      try {
        const result = await api.threat.importIOC(file)
        imported += Number(result?.imported ?? 0)
        skipped += Number(result?.skipped ?? 0)
        if (result?.format) formats.add(result.format)
        for (const item of result?.errors ?? []) {
          errors.push(`${file}: ${item}`)
        }
      } catch (err) {
        errors.push(`${file}: ${errorText(err)}`)
      }
    }

    setImportSummary({
      files: files.length,
      imported,
      skipped,
      errors,
      formats: Array.from(formats),
    })
    await loadIOCs()

    if (errors.length > 0) {
      notify(
        'warn',
        'IOC 导入完成（含错误）',
        `导入 ${imported} 条，跳过 ${skipped} 条，错误 ${errors.length} 条`,
        7000,
      )
    } else {
      notify('success', 'IOC 导入完成', `导入 ${imported} 条，跳过 ${skipped} 条`)
    }
    setImporting(false)
  }

  const confirmDelete = async () => {
    const target = deleteTarget()
    if (!target) return
    setDeleting(true)
    try {
      await api.threat.removeIOC(target.id)
      setSelected((prev) => {
        const next = new Set(prev)
        next.delete(target.id)
        return next
      })
      setDeleteTarget(null)
      notify('success', 'IOC 已删除', `${target.type} · ${target.value}`)
      await loadIOCs()
    } catch (err) {
      notify('error', '删除 IOC 失败', errorText(err))
    } finally {
      setDeleting(false)
    }
  }

  const runSearch = async () => {
    if (searchBusy()) return
    const values = searchValues()
    if (values.length === 0) {
      notify('warn', '没有可检索的 IOC', '请勾选指标，或在手动值中输入至少一个 IOC。')
      return
    }

    const start = toIso(startTime())
    const end = toIso(endTime())
    if (start && end && new Date(start).getTime() > new Date(end).getTime()) {
      notify('warn', '时间范围无效', '起始时间不能晚于结束时间。')
      return
    }

    const request: IOCSearchRequest = {
      project_id: props.projectId,
      values,
    }
    if (start || end) {
      const query: { start_time?: string; end_time?: string } = {}
      if (start) query.start_time = start
      if (end) query.end_time = end
      request.query = query
    }

    setSearchBusy(true)
    setSearchError('')
    setSearchResult(null)
    try {
      const result = await api.threat.searchIOC(request)
      setSearchResult(result)
      notify(
        'success',
        'IOC 检索完成',
        `命中 ${formatInt(result?.total)} 条，扫描 ${formatInt(result?.scanned)} 条`,
      )
    } catch (err) {
      const message = errorText(err)
      setSearchError(message)
      notify('error', 'IOC 检索失败', message)
    } finally {
      setSearchBusy(false)
    }
  }

  return (
    <div class="stack">
      <div class="panel">
        <div class="panel-head">
          <div class="panel-title">
            <span class="tick" />
            IOC 指标库
            <Badge tone="muted">{iocs().length}</Badge>
          </div>
          <div class="row">
            <Button
              variant="primary"
              size="sm"
              loading={importing()}
              onClick={() => void pickAndImport()}
            >
              <Icon name="upload" size={12} />
              导入 IOC
            </Button>
            <IconButton
              icon="refresh"
              title="刷新 IOC"
              disabled={loading() || importing()}
              onClick={() => void loadIOCs()}
            />
          </div>
        </div>

        <div
          class="row wrap"
          style={{
            padding: '10px 12px',
            'border-bottom': '1px solid var(--border)',
            gap: '8px',
          }}
        >
          <div class="search-box grow" style={{ 'min-width': '220px' }}>
            <span class="search-prefix">
              <Icon name="search" size={12} />
            </span>
            <input
              value={filter()}
              placeholder="过滤类型、值、来源或威胁等级…"
              aria-label="过滤 IOC"
              onInput={(e) => setFilter(e.currentTarget.value)}
            />
            <Show when={filter()}>
              <button
                type="button"
                class="icon-btn"
                style={{ width: '22px', height: '22px' }}
                title="清空过滤"
                onClick={() => setFilter('')}
              >
                <Icon name="x" size={11} />
              </button>
            </Show>
          </div>
          <span class="mono-sm faint">已选 {selected().size}</span>
          <Button size="sm" variant="ghost" onClick={toggleAllVisible}>
            {allVisibleSelected() ? '取消全选' : '全选当前'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={selected().size === 0}
            onClick={() => setSelected(new Set<string>())}
          >
            清空选择
          </Button>
        </div>

        <Show when={importing()}>
          <div class="panel-body" style={{ 'padding-top': '6px', 'padding-bottom': '4px' }}>
            <PageBusy visible text="正在导入 IOC 文件…" />
          </div>
        </Show>

        <Show when={importSummary()}>
          {(summary) => (
            <div class="panel-body" style={{ 'padding-top': '6px', 'padding-bottom': '6px' }}>
              <div class="success-note">
                <div class="row wrap" style={{ gap: '6px' }}>
                  <Badge tone="mint">导入 {summary().imported}</Badge>
                  <Badge tone="amber">跳过 {summary().skipped}</Badge>
                  <Badge tone="muted">文件 {summary().files}</Badge>
                  <For each={summary().formats}>
                    {(format) => <span class="tag">{format}</span>}
                  </For>
                  <span class="mono-sm faint">错误 {summary().errors.length}</span>
                </div>
                <Show when={summary().errors.length > 0}>
                  <div
                    class="mono-sm ember"
                    style={{ 'margin-top': '6px', 'max-height': '82px', overflow: 'auto' }}
                  >
                    <For each={previewErrors()}>{(message) => <div>{message}</div>}</For>
                    <Show when={summary().errors.length > previewErrors().length}>
                      <div class="faint">
                        另有 {summary().errors.length - previewErrors().length} 条错误未显示
                      </div>
                    </Show>
                  </div>
                </Show>
              </div>
            </div>
          )}
        </Show>

        <Show when={loadError()}>
          <div class="panel-body" style={{ 'padding-top': '4px', 'padding-bottom': '8px' }}>
            <div class="danger-note">
              <div class="row between wrap">
                <span>{loadError()}</span>
                <Button size="sm" variant="ghost" onClick={() => void loadIOCs()}>
                  重试
                </Button>
              </div>
            </div>
          </div>
        </Show>

        <Show when={loading()}>
          <div class="panel-body" style={{ 'padding-top': '2px', 'padding-bottom': '6px' }}>
            <PageBusy visible text="加载 IOC 指标…" />
          </div>
        </Show>

        <Show when={!loading() && !loadError()}>
          <Show
            when={visibleIocs().length > 0}
            fallback={
              <EmptyState
                icon="crosshair"
                title={iocs().length === 0 ? 'IOC 指标库为空' : '没有匹配的指标'}
                text={
                  iocs().length === 0
                    ? '支持 CSV、JSON、TXT 指标文件，导入后即可对项目日志执行检索。'
                    : '调整过滤条件，或清空搜索关键字。'
                }
                action={
                  iocs().length === 0 ? (
                    <Button
                      variant="primary"
                      size="sm"
                      loading={importing()}
                      onClick={() => void pickAndImport()}
                    >
                      <Icon name="upload" size={12} />
                      导入 IOC
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div class="scrollable" style={{ 'max-height': '430px' }}>
              <table class="data-table">
                <thead>
                  <tr>
                    <th style={{ width: '34px' }}>
                      <input
                        type="checkbox"
                        class="checkbox"
                        checked={allVisibleSelected()}
                        aria-label="全选当前 IOC"
                        onChange={toggleAllVisible}
                      />
                    </th>
                    <th style={{ width: '82px' }}>类型</th>
                    <th>指标值</th>
                    <th style={{ width: '100px' }}>威胁等级</th>
                    <th>描述</th>
                    <th style={{ width: '120px' }}>来源</th>
                    <th style={{ width: '126px' }}>创建时间</th>
                    <th style={{ width: '44px' }} />
                  </tr>
                </thead>
                <tbody>
                  <For each={visibleIocs()}>
                    {(item) => (
                      <tr>
                        <td>
                          <input
                            type="checkbox"
                            class="checkbox"
                            checked={selected().has(item.id)}
                            aria-label={`选择 IOC ${item.value}`}
                            onChange={() => toggleOne(item.id)}
                          />
                        </td>
                        <td>
                          <Badge tone="muted">{item.type || '未知'}</Badge>
                        </td>
                        <td class="mono-sm" style={{ 'word-break': 'break-all' }}>
                          {item.value}
                        </td>
                        <td>
                          <Badge tone={threatTone(item.threat_level)}>
                            {item.threat_level || 'unknown'}
                          </Badge>
                        </td>
                        <td class="muted" title={item.description}>
                          {item.description ? truncate(item.description, 110) : '—'}
                        </td>
                        <td class="mono-sm faint truncate" title={item.source}>
                          {item.source || '—'}
                        </td>
                        <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                          {formatTimeShort(item.created_at)}
                        </td>
                        <td>
                          <IconButton
                            icon="trash"
                            title="删除 IOC"
                            onClick={() => setDeleteTarget(item)}
                          />
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </Show>
      </div>

      <div class="panel">
        <div class="panel-head">
          <div class="panel-title">
            <span class="tick" />
            IOC 检索
          </div>
          <span class="mono-sm faint">在当前项目日志中匹配指标</span>
        </div>

        <div class="panel-body">
          <div class="form-row">
            <Field label="手动 IOC 值" hint="每行一个，支持逗号分隔">
              <TextArea
                value={manual()}
                rows={3}
                mono
                placeholder={'8.8.8.8\nevil.example.com\nHKEY_LOCAL_MACHINE\\\\...'}
                onInput={setManual}
              />
            </Field>
            <div class="form-stack" style={{ gap: '10px' }}>
              <Field label="起始时间" hint="可选">
                <TextInput
                  type="datetime-local"
                  value={startTime()}
                  onInput={setStartTime}
                />
              </Field>
              <Field label="结束时间" hint="可选">
                <TextInput type="datetime-local" value={endTime()} onInput={setEndTime} />
              </Field>
            </div>
          </div>

          <div
            class="row between wrap"
            style={{ 'margin-top': '12px', gap: '10px', 'align-items': 'center' }}
          >
            <span class="mono-sm faint">
              已选 {selected().size} 个指标 · 手动 {manualValues().length} 个 · 共{' '}
              {searchValues().length} 个唯一值
            </span>
            <Button
              variant="primary"
              loading={searchBusy()}
              disabled={searchValues().length === 0}
              onClick={() => void runSearch()}
            >
              <Icon name="crosshair" size={12} />
              检索 IOC
            </Button>
          </div>
        </div>

        <Show when={searchBusy()}>
          <div class="panel-body" style={{ 'padding-top': 0 }}>
            <PageBusy visible text="正在扫描项目日志并匹配 IOC…" />
          </div>
        </Show>

        <Show when={searchError()}>
          <div class="panel-body" style={{ 'padding-top': 0 }}>
            <div class="danger-note">IOC 检索失败：{searchError()}</div>
          </div>
        </Show>

        <Show when={searchResult()}>
          {(result) => (
            <div>
              <div
                class="row wrap"
                style={{
                  gap: '18px',
                  padding: '10px 14px',
                  'border-top': '1px solid var(--border)',
                  'border-bottom': '1px solid var(--border)',
                  background: 'var(--surface-soft)',
                }}
              >
                <span class="metric-inline">
                  <b>{formatInt(result().total)}</b> 命中
                </span>
                <span class="metric-inline">
                  <b>{formatInt(result().scanned)}</b> 扫描日志
                </span>
                <span class="metric-inline">
                  <b>{formatDuration(result().took)}</b> 耗时
                </span>
                <span class="mono-sm faint">点击命中行查看日志详情</span>
              </div>

              <Show
                when={(result().matches ?? []).length > 0}
                fallback={
                  <EmptyState
                    icon="crosshair"
                    title="未发现 IOC 命中"
                    text={`已扫描 ${formatInt(result().scanned)} 条日志，未匹配所选指标。`}
                  />
                }
              >
                <div class="scrollable" style={{ 'max-height': '430px' }}>
                  <table class="data-table">
                    <thead>
                      <tr>
                        <th style={{ width: '190px' }}>IOC</th>
                        <th style={{ width: '110px' }}>命中字段</th>
                        <th style={{ width: '136px' }}>时间</th>
                        <th style={{ width: '78px' }}>级别</th>
                        <th style={{ width: '110px' }}>主机</th>
                        <th>消息</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={result().matches ?? []}>
                        {(match) => (
                          <tr
                            style={{ cursor: 'pointer' }}
                            title="打开日志详情"
                            onClick={() =>
                              props.onOpenLog(
                                match.doc_id,
                                match.entry,
                                `IOC ${match.ioc?.value ?? ''} · 命中字段 ${match.matched_field || '—'}`,
                              )
                            }
                          >
                            <td>
                              <div class="row" style={{ gap: '5px' }}>
                                <Badge tone={threatTone(match.ioc?.threat_level)}>
                                  {match.ioc?.type || 'ioc'}
                                </Badge>
                                <span class="mono-sm truncate" title={match.ioc?.value}>
                                  {match.ioc?.value || '—'}
                                </span>
                              </div>
                            </td>
                            <td class="mono-sm">{match.matched_field || '—'}</td>
                            <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                              {formatTimeShort(match.entry?.timestamp)}
                            </td>
                            <td>
                              <Show when={match.entry} fallback={<span class="faint">—</span>}>
                                <LevelBadge level={match.entry?.level} />
                              </Show>
                            </td>
                            <td class="mono-sm truncate" title={match.entry?.source_host}>
                              {match.entry?.source_host || match.entry?.computer || '—'}
                            </td>
                            <td title={match.entry?.message}>
                              {match.entry?.message
                                ? truncate(match.entry.message, 150)
                                : '点击加载日志详情'}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </div>
          )}
        </Show>
      </div>

      <ConfirmDialog
        open={Boolean(deleteTarget())}
        danger
        title="删除 IOC"
        message={`确定删除指标“${deleteTarget()?.value ?? ''}”？删除后不会影响已生成的取证包或历史检测结果。`}
        confirmText="删除"
        loading={deleting()}
        onConfirm={() => void confirmDelete()}
        onCancel={() => {
          if (!deleting()) setDeleteTarget(null)
        }}
      />
    </div>
  )
}
