import { For, Show, createMemo, createSignal, onMount, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { navigate, notify } from '../../stores/app'
import type {
  ParseConfig,
  SSHCollectRequest,
  SSHHost,
  SSHLogSearchRequest,
  SSHRemoteLogFile,
} from '../../types'
import { formatBytes, formatDuration, formatTime, formatTimeShort } from '../../utils'
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Field,
  Icon,
  IconButton,
  SectionLabel,
  Select,
  Spinner,
  TextArea,
  TextInput,
} from '../ui'
import { TaskProgressInline } from './TaskProgressInline'
import {
  datetimeLocalToISO,
  isInvalidDateTimeLocal,
  parseModeLabel,
  splitRemotePaths,
  sshCompressions,
  sshStrategies,
} from './shared'

export interface SSHCollectorProps {
  projectId: string
  parse: () => ParseConfig
}

/** 一键采集系统日志时远端扫描的上限。 */
const SYSTEM_LOG_SCAN_LIMIT = 300
/** 远程日志查找的结果上限。 */
const SEARCH_RESULT_LIMIT = 200

const searchScopeOptions = [
  { value: 'var-log', label: '/var/log（常用）' },
  { value: 'root', label: '/（全盘，较慢）' },
  { value: 'custom', label: '自定义目录' },
]

/** 转义为字面量正则，避免关键词中的元字符被 grep -E 当成表达式。 */
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

interface StartOverrides {
  paths?: string[]
  strategy?: string
  filterPattern?: string
  parse?: ParseConfig
  successTitle?: string
  successMessage?: string
}

/** SSH 远程只读采集配置。 */
export function SSHCollector(props: SSHCollectorProps): JSX.Element {
  const [hosts, setHosts] = createSignal<SSHHost[]>([])
  const [hostsLoading, setHostsLoading] = createSignal(true)
  const [hostsError, setHostsError] = createSignal('')
  const [hostID, setHostID] = createSignal('')
  const [remotePathsText, setRemotePathsText] = createSignal('')
  const [strategy, setStrategy] = createSignal('compress')
  const [filterPattern, setFilterPattern] = createSignal('')
  const [compression, setCompression] = createSignal('gzip')
  const [concurrencyText, setConcurrencyText] = createSignal('8')
  const [sinceText, setSinceText] = createSignal('')
  const [untilText, setUntilText] = createSignal('')
  const [unit, setUnit] = createSignal('')
  const [testing, setTesting] = createSignal(false)
  const [starting, setStarting] = createSignal(false)
  const [lastTaskId, setLastTaskId] = createSignal<string | undefined>(undefined)

  // 一键采集系统日志
  const [discovering, setDiscovering] = createSignal(false)

  // 远程日志查找
  const [searchScope, setSearchScope] = createSignal('var-log')
  const [searchCustomDir, setSearchCustomDir] = createSignal('/var/log')
  const [searchKeyword, setSearchKeyword] = createSignal('')
  const [searchContent, setSearchContent] = createSignal(false)
  const [includeRotated, setIncludeRotated] = createSignal(false)
  const [onlyMatched, setOnlyMatched] = createSignal(false)
  const [searching, setSearching] = createSignal(false)
  const [pulling, setPulling] = createSignal(false)
  const [searchRan, setSearchRan] = createSignal(false)
  const [searchFiles, setSearchFiles] = createSignal<SSHRemoteLogFile[]>([])
  const [searchTruncated, setSearchTruncated] = createSignal(false)
  const [searchTookMs, setSearchTookMs] = createSignal(0)
  const [selectedPaths, setSelectedPaths] = createSignal<Set<string>>(new Set<string>())

  const hostOptions = createMemo(() =>
    hosts().map((host) => ({
      value: host.id,
      label: `${host.name || host.host} · ${host.username}@${host.host}:${host.port}`,
    })),
  )
  const selectedHost = createMemo(() => hosts().find((host) => host.id === hostID()))
  const strategyInfo = createMemo(
    () => sshStrategies.find((item) => item.value === strategy()) ?? sshStrategies[0],
  )
  const remotePathCount = createMemo(() => splitRemotePaths(remotePathsText()).length)
  const selectedCount = createMemo(() => selectedPaths().size)
  const contentSearchAvailable = createMemo(() => searchKeyword().trim().length > 0)

  onMount(() => {
    void loadHosts(false)
  })

  async function loadHosts(manual: boolean) {
    setHostsLoading(true)
    setHostsError('')
    try {
      const result = await api.ssh.hosts()
      const list = Array.isArray(result) ? result : []
      setHosts(list)
      if (!list.some((host) => host.id === hostID())) {
        setHostID(list[0]?.id ?? '')
        resetSearchResults()
      }
      if (manual) notify('success', 'SSH 主机列表已刷新', `共 ${list.length} 台主机。`)
    } catch (err) {
      const message = errorText(err)
      setHostsError(message)
      if (manual) notify('error', '加载 SSH 主机失败', message)
    } finally {
      setHostsLoading(false)
    }
  }

  function resetSearchResults() {
    setSearchRan(false)
    setSearchFiles([])
    setSearchTruncated(false)
    setSearchTookMs(0)
    setSelectedPaths(new Set<string>())
  }

  function selectHost(value: string) {
    if (value === hostID()) return
    setHostID(value)
    resetSearchResults()
  }

  async function testConnection() {
    const id = hostID()
    if (!id) {
      notify('warn', '未选择 SSH 主机', '请先选择一个 SSH 主机。')
      return
    }
    setTesting(true)
    try {
      await api.ssh.test(id)
      notify(
        'success',
        'SSH 连接成功',
        selectedHost()
          ? `${selectedHost()?.username}@${selectedHost()?.host}:${selectedHost()?.port}`
          : undefined,
      )
    } catch (err) {
      notify('error', 'SSH 连接失败', errorText(err))
    } finally {
      setTesting(false)
    }
  }

  /** 混合系统日志（syslog/auth.log 等）不沿用当前表单模板，避免固定模板误解析。 */
  function mixedLogParse(): ParseConfig {
    return { mode: 'builtin', sample_size: props.parse().sample_size }
  }

  function buildSearchRequest(maxResults: number): SSHLogSearchRequest {
    const keyword = searchKeyword().trim()
    const scope = searchScope()
    const roots =
      scope === 'custom'
        ? [searchCustomDir().trim() || '/var/log']
        : scope === 'var-log'
          ? ['/var/log']
          : undefined
    return {
      host_id: hostID(),
      roots,
      full_scan: scope === 'root',
      keyword,
      search_content: searchContent() && keyword.length > 0,
      include_rotated: includeRotated(),
      max_results: maxResults,
    }
  }

  async function collectSystemLogs() {
    if (!props.projectId) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return
    }
    const id = hostID()
    if (!id) {
      notify('warn', '未选择 SSH 主机', '请先选择一个 SSH 主机。')
      return
    }
    if (discovering()) return
    setDiscovering(true)
    try {
      const result = await api.ssh.searchLogs({
        host_id: id,
        roots: ['/var/log'],
        full_scan: false,
        keyword: '',
        search_content: false,
        include_rotated: false,
        max_results: SYSTEM_LOG_SCAN_LIMIT,
      })
      const files = Array.isArray(result?.files) ? result.files : []
      const paths = Array.from(new Set(files.map((file) => file.path).filter(Boolean)))
      if (paths.length === 0) {
        notify('warn', '未发现系统日志', '远端 /var/log 下没有找到可采集的文本日志文件。')
        return
      }
      setRemotePathsText(paths.join('\n'))
      setStrategy('plain')
      const truncated = Boolean(result?.truncated)
      await startWith({
        paths,
        strategy: 'plain',
        parse: mixedLogParse(),
        successTitle: '系统日志采集已启动',
        successMessage: `发现 ${paths.length} 个日志文件，已使用 plain 策略提交采集任务。${
          truncated
            ? `扫描结果超过 ${SYSTEM_LOG_SCAN_LIMIT} 条，本次仅采集前 ${SYSTEM_LOG_SCAN_LIMIT} 个文件。`
            : ''
        }`,
      })
    } catch (err) {
      notify('error', '扫描系统日志失败', errorText(err))
    } finally {
      setDiscovering(false)
    }
  }

  async function findLogs() {
    const id = hostID()
    if (!id) {
      notify('warn', '未选择 SSH 主机', '请先选择一个 SSH 主机。')
      return
    }
    if (searching()) return
    if (searchScope() === 'custom' && !searchCustomDir().trim()) {
      notify('warn', '缺少自定义目录', '请填写要查找的远端目录，例如 /var/log。')
      return
    }
    setSearching(true)
    try {
      const result = await api.ssh.searchLogs(buildSearchRequest(SEARCH_RESULT_LIMIT))
      const files = Array.isArray(result?.files) ? result.files : []
      setSearchFiles(files)
      setSearchTruncated(Boolean(result?.truncated))
      setSearchTookMs(Number(result?.took_ms) || 0)
      setSelectedPaths(new Set<string>())
      setSearchRan(true)
      if (files.length === 0) {
        notify('info', '未找到远端日志', '请调整目录、关键词，或勾选包含轮转/压缩日志后重试。')
      }
    } catch (err) {
      notify('error', '查找远端日志失败', errorText(err))
    } finally {
      setSearching(false)
    }
  }

  function toggleSelected(path: string, checked: boolean) {
    setSelectedPaths((prev) => {
      const next = new Set(prev)
      if (checked) next.add(path)
      else next.delete(path)
      return next
    })
  }

  function selectAllSearchFiles() {
    setSelectedPaths(new Set(searchFiles().map((file) => file.path)))
  }

  function clearSelectedPaths() {
    setSelectedPaths(new Set<string>())
  }

  async function pullSelected() {
    const paths = Array.from(selectedPaths())
    if (paths.length === 0) {
      notify('warn', '未选择日志', '请先勾选要拉取的远端日志文件。')
      return
    }
    const keyword = searchKeyword().trim()
    const onlyMatchedLines = searchContent() && keyword.length > 0 && onlyMatched()
    setPulling(true)
    try {
      await startWith({
        paths,
        strategy: onlyMatchedLines ? 'filter' : 'plain',
        filterPattern: onlyMatchedLines ? escapeRegExp(keyword) : undefined,
        parse: mixedLogParse(),
        successTitle: '远程日志拉取已启动',
        successMessage: `${paths.length} 个文件 · ${
          onlyMatchedLines ? `filter 策略（仅保留匹配 “${keyword}” 的行）` : 'plain 策略'
        }`,
      })
    } finally {
      setPulling(false)
    }
  }

  async function startWith(overrides: StartOverrides = {}): Promise<boolean> {
    if (!props.projectId) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return false
    }
    const id = hostID()
    if (!id) {
      notify('warn', '未选择 SSH 主机', '请先选择一个 SSH 主机。')
      return false
    }
    const effectiveStrategy = overrides.strategy ?? strategy()
    const remotePaths = overrides.paths ?? splitRemotePaths(remotePathsText())
    if (!remotePaths.length) {
      notify('warn', '缺少远程路径', '请填写要采集的远程文件或目录路径。')
      return false
    }
    const effectiveFilter = (overrides.filterPattern ?? filterPattern()).trim()
    if (effectiveStrategy === 'filter' && !effectiveFilter) {
      notify('warn', '缺少过滤正则', 'filter 策略需要填写过滤正则表达式。')
      return false
    }
    const concurrency = Number(concurrencyText())
    if (!Number.isFinite(concurrency) || concurrency < 1) {
      notify('warn', '并发数无效', '并发数必须是大于 0 的整数。')
      return false
    }
    if (isInvalidDateTimeLocal(sinceText()) || isInvalidDateTimeLocal(untilText())) {
      notify('error', '时间格式无效', '请检查 journalctl 的 since / until 时间。')
      return false
    }
    const since =
      effectiveStrategy === 'journalctl' ? datetimeLocalToISO(sinceText()) : undefined
    const until =
      effectiveStrategy === 'journalctl' ? datetimeLocalToISO(untilText()) : undefined
    if (since && until && new Date(since).getTime() > new Date(until).getTime()) {
      notify('error', '时间范围无效', 'since 不能晚于 until。')
      return false
    }

    setStarting(true)
    try {
      const request: SSHCollectRequest = {
        project_id: props.projectId,
        host_id: id,
        remote_paths: remotePaths,
        strategy: effectiveStrategy,
        filter_pattern: effectiveStrategy === 'filter' ? effectiveFilter : undefined,
        compression: compression(),
        concurrency: Math.min(64, Math.floor(concurrency)),
        since,
        until,
        unit: effectiveStrategy === 'journalctl' ? unit().trim() || undefined : undefined,
        parse: overrides.parse ?? props.parse(),
      }
      const task = await api.collect.startSSH(request)
      if (task?.id) setLastTaskId(task.id)
      notify(
        'success',
        overrides.successTitle ?? 'SSH 采集已启动',
        overrides.successMessage ??
          (task?.message || `任务 ${task?.id ?? ''} 已提交，进度见右侧任务队列。`),
      )
      return true
    } catch (err) {
      notify('error', '启动 SSH 采集失败', errorText(err))
      return false
    } finally {
      setStarting(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '14px' }}>
      <div class="success-note">
        SSH 采集为只读操作：仅执行读取、压缩、正则过滤与 journalctl 导出命令，不修改或删除远程文件。
      </div>

      <Show when={hostsLoading() && hosts().length === 0}>
        <div class="row" style={{ gap: '6px' }}>
          <Spinner />
          <span class="mono-sm faint">正在加载 SSH 主机…</span>
        </div>
      </Show>

      <Show when={hostsError()}>
        <div class="danger-note">
          <div class="row between wrap" style={{ gap: '8px' }}>
            <span class="mono-sm">{hostsError()}</span>
            <Button size="sm" variant="ghost" onClick={() => void loadHosts(true)}>
              重试
            </Button>
          </div>
        </div>
      </Show>

      <Show when={!hostsLoading() && !hostsError() && hosts().length === 0}>
        <EmptyState
          icon="server"
          title="尚未配置 SSH 主机"
          text="SSH 远程采集需要先保存主机连接信息。请前往系统设置添加主机。"
          action={
            <Button variant="primary" onClick={() => navigate('settings')}>
              <Icon name="settings" size={12} />
              前往系统设置
            </Button>
          }
        />
      </Show>

      <Show when={hosts().length > 0}>
        <div class="stack" style={{ gap: '14px' }}>
          <Field label="SSH 主机" hint={`${hosts().length} 台可用`}>
            <div class="row" style={{ gap: '6px' }}>
              <div class="grow">
                <Select
                  value={hostID()}
                  options={hostOptions()}
                  onInput={(value) => selectHost(value)}
                />
              </div>
              <IconButton
                icon="refresh"
                title="刷新 SSH 主机列表"
                disabled={hostsLoading()}
                onClick={() => void loadHosts(true)}
              />
              <Button
                size="sm"
                loading={testing()}
                disabled={!hostID()}
                onClick={() => void testConnection()}
              >
                <Show when={!testing()}>
                  <Icon name="zap" size={11} />
                </Show>
                测试连接
              </Button>
            </div>
          </Field>

          <Show when={selectedHost()}>
            {(host) => (
              <div class="row wrap" style={{ gap: '5px' }}>
                <Badge tone="violet">{host().auth_method === 'key' ? '密钥认证' : '密码认证'}</Badge>
                <span class="mono-sm faint">
                  {host().username}@{host().host}:{host().port}
                </span>
                <Show when={host().proxy_jump}>
                  <span class="mono-sm faint">· 跳板 {host().proxy_jump}</span>
                </Show>
              </div>
            )}
          </Show>

          <Field label="远程路径" hint="换行或逗号分隔">
            <TextArea
              mono
              rows={4}
              value={remotePathsText()}
              placeholder={'/var/log/nginx/access.log\n/var/log/myapp/*.log'}
              onInput={(value) => setRemotePathsText(value)}
            />
          </Field>

          <div class="row wrap" style={{ gap: '8px' }}>
            <Button
              size="sm"
              loading={discovering()}
              disabled={!hostID() || starting()}
              onClick={() => void collectSystemLogs()}
            >
              <Show when={!discovering()}>
                <Icon name="cpu" size={11} />
              </Show>
              一键采集系统日志
            </Button>
            <span class="mono-sm faint">
              扫描 /var/log 文本日志（不含轮转/压缩），自动使用 plain 策略并立即开始采集
            </span>
          </div>

          <div>
            <SectionLabel>拉取策略</SectionLabel>
            <div class="form-row">
              <Field label="策略">
                <Select
                  value={strategy()}
                  options={sshStrategies.map((item) => ({
                    value: item.value,
                    label: item.label,
                  }))}
                  onInput={(value) => setStrategy(value)}
                />
              </Field>
              <Field label="压缩方式">
                <Select
                  value={compression()}
                  options={sshCompressions}
                  onInput={(value) => setCompression(value)}
                />
              </Field>
              <Field label="并发数" hint="1-64">
                <TextInput
                  type="number"
                  value={concurrencyText()}
                  onInput={(value) => setConcurrencyText(value)}
                />
              </Field>
            </div>
            <div class="detail-note" style={{ 'margin-top': '8px' }}>
              {strategyInfo().description}
            </div>
          </div>

          <Show when={strategy() === 'filter'}>
            <Field label="过滤正则" hint="服务器端 grep -E">
              <TextInput
                mono
                value={filterPattern()}
                placeholder="error|exception|timeout"
                onInput={(value) => setFilterPattern(value)}
              />
            </Field>
          </Show>

          <Show when={strategy() === 'journalctl'}>
            <div class="form-row">
              <Field label="since" hint="按本地时区解释">
                <TextInput
                  type="datetime-local"
                  value={sinceText()}
                  onInput={(value) => setSinceText(value)}
                />
              </Field>
              <Field label="until" hint="按本地时区解释">
                <TextInput
                  type="datetime-local"
                  value={untilText()}
                  onInput={(value) => setUntilText(value)}
                />
              </Field>
              <Field label="unit" hint="可选，如 nginx.service">
                <TextInput
                  mono
                  value={unit()}
                  placeholder="nginx.service"
                  onInput={(value) => setUnit(value)}
                />
              </Field>
            </div>
          </Show>

          <div class="row between wrap" style={{ gap: '10px' }}>
            <span class="mono-sm faint">
              {remotePathCount()} 个路径 · {strategy()} · 解析 {parseModeLabel(props.parse().mode)}
            </span>
            <Button
              variant="primary"
              loading={starting()}
              disabled={remotePathCount() === 0 || !hostID()}
              onClick={() => void startWith()}
            >
              <Show when={!starting()}>
                <Icon name="play" size={12} />
              </Show>
              开始采集
            </Button>
          </div>

          <Show when={lastTaskId()}>
            <TaskProgressInline taskId={lastTaskId()} label="SSH 远程采集" />
          </Show>

          <div class="divider" style={{ margin: '0' }} />

          <div class="stack" style={{ gap: '10px' }}>
            <SectionLabel>远程日志查找</SectionLabel>

            <div class="form-row">
              <Field label="查找范围">
                <Select
                  value={searchScope()}
                  options={searchScopeOptions}
                  onInput={(value) => setSearchScope(value)}
                />
              </Field>
              <Show when={searchScope() === 'custom'}>
                <Field label="自定义目录" hint="远端绝对路径">
                  <TextInput
                    mono
                    value={searchCustomDir()}
                    placeholder="/var/log"
                    onInput={(value) => setSearchCustomDir(value)}
                  />
                </Field>
              </Show>
              <Field label="关键词" hint="可留空，仅列出日志文件">
                <TextInput
                  mono
                  value={searchKeyword()}
                  placeholder="error / nginx / auth.log"
                  onInput={(value) => setSearchKeyword(value)}
                  onEnter={() => void findLogs()}
                />
              </Field>
            </div>

            <div class="row between wrap" style={{ gap: '10px' }}>
              <div class="row wrap" style={{ gap: '14px' }}>
                <Checkbox
                  checked={searchContent()}
                  disabled={!contentSearchAvailable()}
                  label="搜索日志内容"
                  onChange={(checked) => {
                    setSearchContent(checked)
                    if (!checked) setOnlyMatched(false)
                  }}
                />
                <Checkbox
                  checked={includeRotated()}
                  label="包含轮转/压缩日志"
                  onChange={(checked) => setIncludeRotated(checked)}
                />
              </div>
              <Button loading={searching()} disabled={!hostID()} onClick={() => void findLogs()}>
                <Show when={!searching()}>
                  <Icon name="search" size={12} />
                </Show>
                查找日志
              </Button>
            </div>

            <Show when={!contentSearchAvailable()}>
              <div class="mono-sm faint">
                未填写关键词时仅按文件名/路径列出日志；填写关键词后可以搜索日志内容。
              </div>
            </Show>

            <Show when={searchRan()}>
              <div class="stack" style={{ gap: '7px' }}>
                <div class="row between wrap" style={{ gap: '8px' }}>
                  <span class="mono-sm faint">
                    共 {searchFiles().length} 个文件 · 已选 {selectedCount()} 个 · 用时{' '}
                    {formatDuration(searchTookMs())}
                  </span>
                  <div class="row" style={{ gap: '6px' }}>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={searchFiles().length === 0}
                      onClick={selectAllSearchFiles}
                    >
                      全选
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={selectedCount() === 0}
                      onClick={clearSelectedPaths}
                    >
                      清空
                    </Button>
                  </div>
                </div>

                <Show when={searchTruncated()}>
                  <div class="detail-note">
                    结果超过上限，仅返回前 {SEARCH_RESULT_LIMIT} 条；请缩小目录范围或补充关键词后重新查找。
                  </div>
                </Show>

                <Show when={searchFiles().length === 0}>
                  <div class="faint mono-sm">未找到匹配的远端日志文件。</div>
                </Show>

                <Show when={searchFiles().length > 0}>
                  <div
                    class="scrollable scroll-thin"
                    style={{
                      'max-height': '240px',
                      border: '1px solid var(--border)',
                      'border-radius': 'var(--r-md)',
                      background: 'var(--surface-soft)',
                    }}
                  >
                    <For each={searchFiles()}>
                      {(file) => (
                        <label
                          class="row"
                          style={{
                            gap: '8px',
                            padding: '5px 10px',
                            'border-bottom': '1px solid var(--divider)',
                            cursor: 'pointer',
                          }}
                        >
                          <input
                            type="checkbox"
                            class="checkbox"
                            checked={selectedPaths().has(file.path)}
                            onChange={(e) => toggleSelected(file.path, e.currentTarget.checked)}
                          />
                          <span class="mono-sm grow truncate" title={file.path}>
                            {file.path}
                          </span>
                          <span class="mono-sm faint numeric" style={{ 'flex-shrink': 0 }}>
                            {formatBytes(file.size)}
                          </span>
                          <span
                            class="mono-sm faint numeric"
                            style={{ 'flex-shrink': 0 }}
                            title={formatTime(file.mod_time)}
                          >
                            {formatTimeShort(file.mod_time)}
                          </span>
                        </label>
                      )}
                    </For>
                  </div>

                  <div class="row between wrap" style={{ gap: '8px' }}>
                    <Show when={contentSearchAvailable() && searchContent()}>
                      <Checkbox
                        checked={onlyMatched()}
                        label="仅拉取匹配行（filter 策略）"
                        onChange={(checked) => setOnlyMatched(checked)}
                      />
                    </Show>
                    <span class="mono-sm faint" style={{ 'margin-left': 'auto' }}>
                      已选 {selectedCount()} 个
                    </span>
                    <Button
                      variant="primary"
                      loading={pulling()}
                      disabled={selectedCount() === 0 || starting()}
                      onClick={() => void pullSelected()}
                    >
                      <Show when={!pulling()}>
                        <Icon name="download" size={12} />
                      </Show>
                      拉取选中日志
                    </Button>
                  </div>
                </Show>
              </div>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}
