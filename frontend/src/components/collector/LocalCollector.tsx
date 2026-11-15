import { For, Show, createMemo, createSignal, onMount, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import {
  localDiscovery,
  localDiscoveryError,
  localDiscoveryLoading,
  notify,
  refreshLocalDiscovery,
} from '../../stores/app'
import type { LocalCollectRequest, MiddlewarePreset, ParseConfig } from '../../types'
import { formatBytes, truncate } from '../../utils'
import {
  Badge,
  Button,
  Field,
  Icon,
  SectionLabel,
  Spinner,
  TextArea,
  Toggle,
} from '../ui'
import { FilePreview } from './FilePreview'
import { TaskProgressInline } from './TaskProgressInline'
import { categoryLabel, parseModeLabel, splitLocalPaths } from './shared'

export interface LocalCollectorProps {
  projectId: string
  parse: () => ParseConfig
  windows: boolean
  onSamples: (lines: string[], source: string) => void
  onTemplate: (templateId: string) => void
}

/** 本地文件 / 目录采集配置。 */
export function LocalCollector(props: LocalCollectorProps): JSX.Element {
  const [pathsText, setPathsText] = createSignal('')
  const [excludeText, setExcludeText] = createSignal('')
  const [recursive, setRecursive] = createSignal(true)
  const [previewPath, setPreviewPath] = createSignal('')
  const [commonPaths, setCommonPaths] = createSignal<Record<string, string[]>>({})
  const [presets, setPresets] = createSignal<MiddlewarePreset[]>([])
  const [shortcutsLoading, setShortcutsLoading] = createSignal(true)
  const [presetsLoading, setPresetsLoading] = createSignal(true)
  const [shortcutsError, setShortcutsError] = createSignal('')
  const [presetsError, setPresetsError] = createSignal('')
  const [appliedPreset, setAppliedPreset] = createSignal('')
  const [picking, setPicking] = createSignal<'files' | 'dir' | ''>('')
  const [starting, setStarting] = createSignal(false)
  const [lastTaskId, setLastTaskId] = createSignal<string | undefined>(undefined)

  const pathCount = createMemo(() => splitLocalPaths(pathsText()).length)
  const shortcutGroups = createMemo(() =>
    Object.entries(commonPaths())
      .map(
        ([category, paths]) =>
          [
            category,
            (Array.isArray(paths) ? paths : []).filter((path) => !/\.evtx$/i.test(path)),
          ] as const,
      )
      .filter(([, paths]) => paths.length > 0),
  )
  const hasEvtxShortcuts = createMemo(() =>
    Object.values(commonPaths()).some(
      (paths) => Array.isArray(paths) && paths.some((path) => /\.evtx$/i.test(path)),
    ),
  )
  const discoveredFiles = createMemo(() =>
    (localDiscovery()?.items ?? [])
      .filter((item) => item.readable && (item.type === 'file' || item.type === 'evtx'))
      .slice(0, 12),
  )

  onMount(() => {
    if (!localDiscovery()) void refreshLocalDiscovery(false)
    void (async () => {
      try {
        const result = await api.collect.commonPaths()
        if (result && typeof result === 'object') {
          setCommonPaths(result)
        }
      } catch (err) {
        setShortcutsError(errorText(err))
      } finally {
        setShortcutsLoading(false)
      }
    })()

    void (async () => {
      try {
        const result = await api.collect.presets()
        setPresets(Array.isArray(result) ? result : [])
      } catch (err) {
        setPresetsError(errorText(err))
      } finally {
        setPresetsLoading(false)
      }
    })()
  })

  function addPaths(input: string[]) {
    const incoming = input.map((value) => value.trim()).filter(Boolean)
    if (!incoming.length) return
    const current = splitLocalPaths(pathsText())
    for (const path of incoming) {
      if (!current.includes(path)) current.push(path)
    }
    setPathsText(current.join('\n'))
    if (!previewPath()) {
      const candidate = incoming.find((path) => !/[*?]/.test(path))
      if (candidate) setPreviewPath(candidate)
    }
  }

  async function pickFiles() {
    if (picking()) return
    setPicking('files')
    try {
      const files = await api.system.selectFiles()
      if (Array.isArray(files) && files.length > 0) {
        addPaths(files)
        if (!previewPath()) setPreviewPath(files[0])
        notify('info', '已添加文件', `${files.length} 个文件路径已加入列表。`)
      }
    } catch (err) {
      notify('error', '选择文件失败', errorText(err))
    } finally {
      setPicking('')
    }
  }

  async function pickDirectory() {
    if (picking()) return
    setPicking('dir')
    try {
      const directory = await api.system.selectDirectory('选择日志目录')
      if (directory) {
        addPaths([directory])
        notify('info', '已添加目录', directory)
      }
    } catch (err) {
      notify('error', '选择目录失败', errorText(err))
    } finally {
      setPicking('')
    }
  }

  function applyPreset(preset: MiddlewarePreset) {
    const platformPaths = props.windows ? preset.windows_paths : preset.linux_paths
    const selectedPaths = Array.isArray(platformPaths) ? platformPaths : []
    if (selectedPaths.length > 0) addPaths(selectedPaths)
    if (preset.template_id) props.onTemplate(preset.template_id)
    setAppliedPreset(preset.name)
    notify(
      'info',
      `已套用预设：${preset.name}`,
      `${
        selectedPaths.length > 0
          ? `追加 ${selectedPaths.length} 个${props.windows ? ' Windows' : ' Linux'}路径`
          : '当前平台没有预置路径'
      }${preset.template_id ? ` · 模板 ${preset.template_id}` : ''}`,
    )
  }

  function presetPathCount(preset: MiddlewarePreset): number {
    const paths = props.windows ? preset.windows_paths : preset.linux_paths
    return Array.isArray(paths) ? paths.length : 0
  }

  async function start() {
    const paths = splitLocalPaths(pathsText())
    if (!props.projectId) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return
    }
    if (!paths.length) {
      notify('warn', '缺少日志路径', '请填写文件或目录路径，或使用快捷路径。')
      return
    }
    const excludes = splitLocalPaths(excludeText())
    const evtxPaths = paths.filter((path) => /\.evtx$/i.test(path))
    const textPaths = paths.filter((path) => !/\.evtx$/i.test(path))
    setStarting(true)
    const started: string[] = []
    const failures: string[] = []
    try {
      if (evtxPaths.length > 0) {
        try {
          const task = await api.collect.startLocal({
            project_id: props.projectId,
            file_paths: evtxPaths,
            recursive: false,
            source_type: 'evtx',
            mode: 'one_time',
            parse: { mode: 'native_evtx', template_id: 'windows-evtx' },
          })
          if (task?.id) setLastTaskId(task.id)
          started.push(`EVTX ${evtxPaths.length} 个`)
        } catch (err) {
          failures.push(`EVTX：${errorText(err)}`)
        }
      }
      if (textPaths.length > 0) {
        try {
          const request: LocalCollectRequest = {
            project_id: props.projectId,
            file_paths: textPaths,
            recursive: recursive(),
            source_type: 'file',
            mode: 'one_time',
            exclude_paths: excludes.length > 0 ? excludes : undefined,
            parse: props.parse(),
          }
          const task = await api.collect.startLocal(request)
          if (task?.id) setLastTaskId(task.id)
          started.push(`文本 ${textPaths.length} 个`)
        } catch (err) {
          failures.push(`文本：${errorText(err)}`)
        }
      }

      if (started.length > 0) {
        notify(
          'success',
          '采集已启动',
          `${started.join(' · ')}，进度见右侧任务队列。`,
        )
      }
      if (failures.length > 0) {
        notify(
          'error',
          started.length > 0 ? '部分采集未启动' : '启动采集失败',
          failures.join('；'),
          9000,
        )
      }
    } finally {
      setStarting(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '14px' }}>
      <Field
        label="日志路径"
        hint={`${pathCount()} 个路径 · 支持通配符`}
      >
        <TextArea
          mono
          rows={4}
          value={pathsText()}
          placeholder={
            props.windows
              ? 'C:\\inetpub\\logs\\LogFiles\\W3SVC1\\*.log\nC:\\ProgramData\\MyApp\\logs'
              : '/var/log/nginx/access.log\n/var/log/*.log'
          }
          onInput={(value) => setPathsText(value)}
        />
      </Field>

      <div class="row wrap" style={{ gap: '8px' }}>
        <Button
          size="sm"
          loading={picking() === 'files'}
          disabled={picking() !== ''}
          onClick={() => void pickFiles()}
        >
          <Show when={picking() !== 'files'}>
            <Icon name="file" size={11} />
          </Show>
          选择文件
        </Button>
        <Button
          size="sm"
          loading={picking() === 'dir'}
          disabled={picking() !== ''}
          onClick={() => void pickDirectory()}
        >
          <Show when={picking() !== 'dir'}>
            <Icon name="folder" size={11} />
          </Show>
          选择目录
        </Button>
        <span class="mono-sm faint">支持多选；路径可用换行或分号分隔</span>
      </div>

      <div class="form-row">
        <div class="field">
          <span class="label">扫描选项</span>
          <div class="row" style={{ gap: '8px', 'min-height': '32px' }}>
            <Toggle
              checked={recursive()}
              label="递归扫描子目录"
              onChange={(checked) => setRecursive(checked)}
            />
            <div>
              <div style={{ 'font-size': '11.5px' }}>递归扫描子目录</div>
              <div class="mono-sm faint">目录内所有匹配文件</div>
            </div>
          </div>
        </div>
        <Field label="排除路径" hint="换行或分号分隔">
          <TextArea
            mono
            rows={2}
            value={excludeText()}
            placeholder={'**/*.tmp\n/var/log/old/**'}
            onInput={(value) => setExcludeText(value)}
          />
        </Field>
      </div>

      <div>
        <SectionLabel>快捷路径</SectionLabel>
        <Show when={hasEvtxShortcuts()}>
          <div class="mono-sm faint" style={{ 'margin-bottom': '7px' }}>
            事件文件（.evtx）会在开始采集时自动按原生 EVTX 解析；Setup / Security 等受保护通道请使用上方「Windows 事件日志」页采集。
          </div>
        </Show>
        <Show
          when={!shortcutsError()}
          fallback={<div class="danger-note mono-sm">{shortcutsError()}</div>}
        >
          <Show
            when={!shortcutsLoading() && shortcutGroups().length === 0}
            fallback={
              <Show
                when={!shortcutsLoading()}
                fallback={
                  <div class="row" style={{ gap: '6px' }}>
                    <Spinner />
                    <span class="mono-sm faint">正在读取常见日志路径…</span>
                  </div>
                }
              >
                <div class="stack" style={{ gap: '9px' }}>
                  <For each={shortcutGroups()}>
                    {([category, paths]) => (
                      <div class="stack" style={{ gap: '5px' }}>
                        <div class="row" style={{ gap: '6px' }}>
                          <span class="mono-sm faint">{categoryLabel(category)}</span>
                          <span class="mono-sm faint">· {paths.length}</span>
                        </div>
                        <div class="row wrap" style={{ gap: '5px' }}>
                          <For each={paths}>
                            {(path) => (
                              <button
                                type="button"
                                class="chip"
                                title={path}
                                onClick={() => addPaths([path])}
                              >
                                <Icon name="plus" size={9} />
                                {truncate(path, 52)}
                              </button>
                            )}
                          </For>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            }
          >
            <div class="faint mono-sm">当前平台暂无常见路径，可直接选择文件或目录。</div>
          </Show>
        </Show>
      </div>

      <div>
        <SectionLabel>启动时发现的本机日志</SectionLabel>
        <Show when={localDiscoveryError()}>
          <div class="danger-note mono-sm">{localDiscoveryError()}</div>
        </Show>
        <Show when={localDiscoveryLoading() && discoveredFiles().length === 0}>
          <div class="row" style={{ gap: '6px' }}>
            <Spinner />
            <span class="mono-sm faint">正在扫描本机日志路径…</span>
          </div>
        </Show>
        <Show when={!localDiscoveryLoading() && !localDiscoveryError() && discoveredFiles().length === 0}>
          <div class="faint mono-sm">未发现可直接读取的常见日志文件。</div>
        </Show>
        <Show when={discoveredFiles().length > 0}>
          <div class="row wrap" style={{ gap: '5px' }}>
            <For each={discoveredFiles()}>
              {(item) => (
                <button
                  type="button"
                  class="chip"
                  title={`${item.path}${item.template_id ? `\n建议模板：${item.template_id}` : ''}`}
                  onClick={() => {
                    addPaths([item.path])
                    if (item.template_id) props.onTemplate(item.template_id)
                  }}
                >
                  <Icon name={item.type === 'evtx' ? 'database' : 'file'} size={11} />
                  <span class="truncate" style={{ 'max-width': '190px' }}>
                    {item.name}
                  </span>
                  <span class="faint">{formatBytes(item.size_bytes)}</span>
                </button>
              )}
            </For>
          </div>
          <div class="row between" style={{ gap: '8px', 'margin-top': '6px' }}>
            <span class="mono-sm faint">
              共 {localDiscovery()?.items?.length ?? 0} 项发现结果，点击加入路径；采集完成后自动登记为项目日志源
            </span>
            <Button
              size="sm"
              variant="ghost"
              loading={localDiscoveryLoading()}
              onClick={() => void refreshLocalDiscovery(true)}
            >
              <Icon name="refresh" size={11} />
              重新扫描
            </Button>
          </div>
        </Show>
      </div>

      <div>
        <SectionLabel>中间件预设</SectionLabel>
        <Show when={presetsError()}>
          <div class="danger-note mono-sm">{presetsError()}</div>
        </Show>
        <Show when={!presetsError() && presetsLoading() && presets().length === 0}>
          <div class="row" style={{ gap: '6px' }}>
            <Spinner />
            <span class="mono-sm faint">正在加载中间件预设…</span>
          </div>
        </Show>
        <Show when={!presetsError() && !presetsLoading() && presets().length === 0}>
          <div class="faint mono-sm">暂无中间件预设。</div>
        </Show>
        <Show when={presets().length > 0}>
          <div class="preset-list">
            <For each={presets()}>
              {(preset) => (
                <button
                  type="button"
                  class={`preset-option ${appliedPreset() === preset.name ? 'active' : ''}`}
                  title={preset.description}
                  onClick={() => applyPreset(preset)}
                >
                  <Icon name="layers" size={11} />
                  <div class="grow" style={{ 'min-width': 0 }}>
                    <div class="row between" style={{ gap: '8px' }}>
                      <span class="truncate">{preset.name}</span>
                      <span class="count">
                        {presetPathCount(preset) > 0
                          ? `${presetPathCount(preset)} 路径`
                          : '当前平台无预置路径'}
                      </span>
                    </div>
                    <div class="mono-sm faint truncate">{preset.description}</div>
                    <div class="row wrap" style={{ gap: '4px', 'margin-top': '2px' }}>
                      <Badge tone="muted">{categoryLabel(preset.category)}</Badge>
                      <Show when={preset.multiline}>
                        <Badge tone="sky">多行</Badge>
                      </Show>
                      <Show when={preset.template_id}>
                        <span class="mono-sm faint truncate">{preset.template_id}</span>
                      </Show>
                    </div>
                  </div>
                </button>
              )}
            </For>
          </div>
        </Show>
      </div>

      <div class="divider" style={{ margin: '0' }} />

      <div class="preview-block">
        <FilePreview
          label="文件预览"
          hint="最多 100 行 · 可作解析样本"
          path={previewPath()}
          onPathChange={(value) => setPreviewPath(value)}
          onLoad={(lines, path) => props.onSamples(lines, path)}
          placeholder="输入要预览的单个文件路径"
        />
      </div>

      <div class="row between wrap" style={{ gap: '10px' }}>
        <span class="mono-sm faint">
          路径 {pathCount()} 个 · 递归 {recursive() ? '开启' : '关闭'} · 解析{' '}
          {parseModeLabel(props.parse().mode)}
        </span>
        <Button
          variant="primary"
          loading={starting()}
          disabled={pathCount() === 0}
          onClick={() => void start()}
        >
          <Show when={!starting()}>
            <Icon name="play" size={12} />
          </Show>
          开始采集
        </Button>
      </div>

      <Show when={lastTaskId()}>
        <TaskProgressInline taskId={lastTaskId()} label="本地文件采集" />
      </Show>
    </div>
  )
}
