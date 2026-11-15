import { For, JSX, Show, createMemo, createSignal } from 'solid-js'
import { api, errorText } from '../../api'
import {
  currentProject,
  localDiscovery,
  localDiscoveryError,
  localDiscoveryLoading,
  navigate,
  notify,
  refreshLocalDiscovery,
} from '../../stores/app'
import type { DetectedLog, ParseConfig } from '../../types'
import { Badge, Button, Icon, Spinner } from '../ui'
import { formatBytes, formatTime } from '../../utils'

function detectedTypeLabel(item: DetectedLog): string {
  switch (String(item.type ?? '').toLowerCase()) {
    case 'eventlog':
      return '事件日志'
    case 'journal':
      return 'journald'
    case 'evtx':
      return 'EVTX'
    default:
      return '文件'
  }
}

function detectedPath(item: DetectedLog): string {
  return String(item.path ?? '').trim() || String(item.channel ?? '').trim() || '—'
}

interface CollectPlan {
  filesByTemplate: Map<string, string[]>
  evtxPaths: string[]
  channels: string[]
  journal: boolean
}

function buildCollectPlan(items: DetectedLog[]): CollectPlan {
  const filesByTemplate = new Map<string, string[]>()
  const evtx = new Set<string>()
  const channels = new Set<string>()
  const seenPaths = new Set<string>()
  let journal = false

  for (const item of items) {
    if (!item.readable) continue
    const path = String(item.path ?? '').trim()
    const type = String(item.type ?? '').toLowerCase()
    if (type === 'eventlog') {
      const channel = String(item.channel ?? '').trim()
      if (channel) channels.add(channel)
      continue
    }
    if (type === 'journal') {
      journal = true
      continue
    }
    if (type === 'evtx' || /\.evtx$/i.test(path)) {
      if (path) evtx.add(path)
      continue
    }
    if (type === 'file' || !type) {
      if (!path || seenPaths.has(path)) continue
      seenPaths.add(path)
      const template = String(item.template_id ?? '').trim() || 'auto'
      const paths = filesByTemplate.get(template) ?? []
      paths.push(path)
      filesByTemplate.set(template, paths)
    }
  }

  return {
    filesByTemplate,
    evtxPaths: Array.from(evtx),
    channels: Array.from(channels),
    journal,
  }
}

/**
 * 项目总览上的本机日志发现摘要。
 * 仅展示数量与类型，完整路径、模板建议和采集操作在“数据采集”页完成。
 */
export function LocalLogDiscovery(): JSX.Element {
  const [collectingAll, setCollectingAll] = createSignal(false)
  const [collectError, setCollectError] = createSignal('')

  const items = createMemo(() => localDiscovery()?.items ?? [])
  const collectPlan = createMemo(() => buildCollectPlan(items()))
  const collectTaskCount = createMemo(() => {
    const plan = collectPlan()
    return (
      plan.filesByTemplate.size +
      (plan.evtxPaths.length > 0 ? 1 : 0) +
      (plan.channels.length > 0 ? 1 : 0) +
      (plan.journal ? 1 : 0)
    )
  })

  const counts = createMemo(() => {
    const list = items()
    return {
      total: list.length,
      readable: list.filter((item) => item.readable).length,
      eventlog: list.filter((item) => item.type === 'eventlog').length,
      journal: list.filter((item) => item.type === 'journal').length,
      files: list.filter((item) => item.type === 'file' || item.type === 'evtx').length,
    }
  })

  const summary = () => {
    const value = counts()
    if (localDiscoveryLoading() && value.total === 0) return '正在扫描本机常见日志路径…'
    if (localDiscoveryError()) return '扫描失败，可在下方重试'
    if (value.total === 0) return '未发现常见日志，可前往数据采集页手动选择'
    return `发现 ${value.total} 个本地日志来源，其中 ${value.readable} 个可读；可在数据采集页将其采集为项目日志源。`
  }

  function goCollect() {
    if (!currentProject()) {
      notify('warn', '请先创建或打开项目', '本机日志采集需要在项目内进行。')
      return
    }
    navigate('collect')
  }

  const parseForTemplate = (template: string): ParseConfig =>
    template === 'auto'
      ? { mode: 'builtin' }
      : { mode: 'builtin', template_id: template }

  async function collectDiscovered() {
    if (collectingAll()) return
    const project = currentProject()
    if (!project) {
      notify('warn', '请先创建或打开项目', '一键采集会把发现的日志加入当前项目。')
      return
    }
    const plan = collectPlan()
    const requests: Array<{ label: string; start: () => Promise<unknown> }> = []

    for (const [template, paths] of plan.filesByTemplate) {
      const unique = Array.from(new Set(paths))
      if (unique.length === 0) continue
      requests.push({
        label:
          template === 'auto'
            ? `文件（自动识别，${unique.length} 个）`
            : `文件（${template}，${unique.length} 个）`,
        start: () =>
          api.collect.startLocal({
            project_id: project.id,
            file_paths: unique,
            recursive: false,
            parse: parseForTemplate(template),
          }),
      })
    }
    if (plan.evtxPaths.length > 0) {
      const paths = Array.from(new Set(plan.evtxPaths))
      requests.push({
        label: `EVTX 文件（${paths.length} 个）`,
        start: () =>
          api.collect.startLocal({
            project_id: project.id,
            file_paths: paths,
            recursive: false,
            parse: { mode: 'native_evtx' },
          }),
      })
    }
    if (plan.channels.length > 0) {
      requests.push({
        label: `Windows 事件日志（${plan.channels.join(', ')}）`,
        start: () =>
          api.collect.startWindowsEvent({
            project_id: project.id,
            channels: plan.channels,
            parse: { mode: 'native_evtx' },
          }),
      })
    }
    if (plan.journal) {
      requests.push({
        label: 'systemd journal',
        start: () =>
          api.collect.startJournal({
            project_id: project.id,
            follow: false,
            parse: { mode: 'builtin', template_id: 'journald-json' },
          }),
      })
    }

    if (requests.length === 0) {
      notify('warn', '没有可采集的已发现日志', '请先重新扫描，或确认发现项可读。')
      return
    }

    setCollectingAll(true)
    setCollectError('')
    const errors: string[] = []
    let started = 0
    for (const request of requests) {
      try {
        await request.start()
        started += 1
      } catch (err) {
        errors.push(`${request.label}: ${errorText(err)}`)
      }
    }
    setCollectingAll(false)

    if (started > 0) {
      notify(
        'success',
        '已启动本机日志采集',
        `${started} 个任务已加入当前项目队列${
          errors.length > 0 ? `；${errors.length} 个任务启动失败` : ''
        }`,
        6000,
      )
      if (errors.length > 0) setCollectError(errors.join('；'))
      navigate('collect')
      return
    }
    const message = errors.join('；') || '没有可启动的采集任务。'
    setCollectError(message)
    notify('error', '启动采集失败', message, 8000)
  }

  return (
    <section class="panel discovery-summary-panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          本机日志发现
        </div>
        <div class="row" style={{ gap: '10px' }}>
          <Show when={localDiscovery()}>
            {(discovery) => <span class="mono-sm faint">{formatTime(discovery().scanned_at)}</span>}
          </Show>
          <Button
            size="sm"
            variant="ghost"
            loading={localDiscoveryLoading()}
            onClick={() => void refreshLocalDiscovery(true)}
          >
            <Icon name="refresh" size={13} />
            重新扫描
          </Button>
        </div>
      </div>

      <div class="panel-body discovery-summary-body">
        <div class="row wrap" style={{ gap: '10px', 'align-items': 'center', 'min-width': 0 }}>
          <span class="discovery-summary-icon">
            <Show when={localDiscoveryLoading()} fallback={<Icon name="cpu" size={18} />}>
              <Spinner />
            </Show>
          </span>
          <div class="grow" style={{ 'min-width': '220px' }}>
            <div class="discovery-summary-text">{summary()}</div>
            <Show when={counts().total > 0}>
              <div class="row wrap" style={{ gap: '6px', 'margin-top': '6px' }}>
                <Badge tone="mint">可读 {counts().readable}</Badge>
                <Show when={counts().eventlog > 0}>
                  <Badge tone="sky">事件日志 {counts().eventlog}</Badge>
                </Show>
                <Show when={counts().journal > 0}>
                  <Badge tone="violet">journald {counts().journal}</Badge>
                </Show>
                <Show when={counts().files > 0}>
                  <Badge tone="muted">文件 {counts().files}</Badge>
                </Show>
              </div>
            </Show>
          </div>
          <Button
            size="sm"
            variant="primary"
            loading={collectingAll()}
            disabled={!currentProject() || counts().readable === 0 || collectingAll()}
            title={
              currentProject()
                ? `将 ${counts().readable} 个可读发现项加入当前项目，可能启动 ${collectTaskCount()} 个采集任务`
                : '请先打开一个项目'
            }
            onClick={() => void collectDiscovered()}
          >
            <Icon name="download" size={13} />
            一键获取本机日志
          </Button>
          <Button size="sm" variant="ghost" onClick={goCollect} disabled={counts().total === 0}>
            <Icon name="upload" size={13} />
            前往数据采集
          </Button>
        </div>

        <Show when={!currentProject() && counts().total > 0}>
          <div class="detail-note" style={{ 'margin-top': '8px' }}>
            一键采集需要先打开一个项目；打开项目后按钮会自动启用。
          </div>
        </Show>

        <Show when={localDiscoveryError()}>
          <div class="danger-note" style={{ 'margin-top': '10px' }}>
            {localDiscoveryError()}
          </div>
        </Show>

        <Show when={collectError()}>
          <div class="danger-note" style={{ 'margin-top': '10px' }}>
            部分采集任务启动失败：{collectError()}
          </div>
        </Show>

        <Show when={items().length > 0}>
          <div style={{ 'margin-top': '10px' }}>
            <div class="row between wrap" style={{ gap: '8px', 'align-items': 'center' }}>
              <span class="section-label" style={{ margin: 0 }}>
                已发现日志路径
              </span>
              <span class="mono-sm faint">
                共 {items().length} 项 · 可读 {counts().readable} · 一键采集任务{' '}
                {collectTaskCount()} 个
              </span>
            </div>
            <div
              class="scrollable scroll-thin"
              style={{
                'max-height': '240px',
                'margin-top': '6px',
                border: '1px solid var(--border)',
                'border-radius': 'var(--r-md)',
              }}
            >
              <table class="data-table" style={{ 'min-width': '760px' }}>
                <thead>
                  <tr>
                    <th style={{ width: '84px' }}>类型</th>
                    <th>路径</th>
                    <th style={{ width: '86px' }}>大小</th>
                    <th style={{ width: '130px' }}>建议模板</th>
                    <th style={{ width: '70px' }}>状态</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={items()}>
                    {(item) => (
                      <tr style={{ opacity: item.readable ? 1 : 0.58 }}>
                        <td>
                          <Badge
                            tone={
                              item.type === 'eventlog'
                                ? 'sky'
                                : item.type === 'journal'
                                  ? 'violet'
                                  : item.type === 'evtx'
                                    ? 'amber'
                                    : 'muted'
                            }
                          >
                            {detectedTypeLabel(item)}
                          </Badge>
                        </td>
                        <td>
                          <div
                            class="mono-sm"
                            style={{ 'word-break': 'break-all', color: 'var(--text)' }}
                            title={detectedPath(item)}
                          >
                            {detectedPath(item)}
                          </div>
                          <Show when={item.name && item.name !== detectedPath(item)}>
                            <div class="mono-sm faint" style={{ 'font-size': '9.5px' }}>
                              {item.name}
                            </div>
                          </Show>
                        </td>
                        <td class="mono-sm faint" style={{ 'white-space': 'nowrap' }}>
                          {item.size_bytes > 0 ? formatBytes(item.size_bytes) : '—'}
                        </td>
                        <td class="mono-sm faint" style={{ 'word-break': 'break-all' }}>
                          {item.template_id || '自动检测'}
                        </td>
                        <td>
                          <Badge tone={item.readable ? 'mint' : 'ember'}>
                            {item.readable ? '可读' : '不可读'}
                          </Badge>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
            <Show when={localDiscovery()?.truncated}>
              <div class="mono-sm faint" style={{ 'margin-top': '4px' }}>
                发现结果已按数量上限截断，可重新扫描或前往数据采集页查看。
              </div>
            </Show>
          </div>
        </Show>

        <div class="mono-sm faint" style={{ 'margin-top': '8px' }}>
          扫描范围：系统日志目录、Web/数据库常见路径、用户目录与事件日志通道；本机发现仅为全局扫描建议，不会自动加入任何项目，也不会展示或上传日志内容。
          一键采集会把所有可读发现项加入当前项目；重复点击会再次采集并可能产生重复日志。受保护事件通道（如 Security）可能需要管理员权限，失败原因会显示在数据采集页任务队列中。
        </div>
      </div>
    </section>
  )
}
