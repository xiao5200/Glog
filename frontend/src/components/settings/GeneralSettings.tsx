import { For, Show, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { appConfig, notify, setAppConfig, systemInfo } from '../../stores/app'
import type { AppConfig, SystemInfo } from '../../types'
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Icon,
  PageBusy,
  Select,
  TextInput,
} from '../ui'
import {
  CopyableValue,
  SettingsPanel,
  cloneJson,
  toInt,
  withCurrentOption,
} from './shared'

const SYSTEM_FIELDS: Array<{ key: keyof SystemInfo; label: string }> = [
  { key: 'os', label: '操作系统' },
  { key: 'arch', label: '架构' },
  { key: 'version', label: '版本' },
  { key: 'data_path', label: '数据目录' },
  { key: 'projects_path', label: '项目目录' },
  { key: 'temp_path', label: '临时目录' },
  { key: 'log_path', label: '日志目录' },
  { key: 'db_path', label: '数据库文件' },
  { key: 'executable', label: '可执行文件' },
]

const SSH_COMPRESSION_OPTIONS = [
  { value: 'gzip', label: 'gzip' },
  { value: 'zstd', label: 'zstd' },
  { value: 'none', label: 'none（不压缩）' },
]

const COMPRESSION_OPTIONS = [
  { value: 'lz4', label: 'lz4（推荐）' },
  { value: 'zstd', label: 'zstd' },
  { value: 'gzip', label: 'gzip' },
  { value: 'none', label: 'none（不压缩）' },
]

const THEME_OPTIONS = [
  { value: 'dark', label: '深色（默认）' },
  { value: 'light', label: '浅色' },
  { value: 'system', label: '跟随系统' },
]

const LANGUAGE_OPTIONS = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'en-US', label: 'English' },
]

export function GeneralSettings() {
  const initialConfig = appConfig()
  const [info, setInfo] = createSignal<SystemInfo | null>(systemInfo())
  const [config, setConfig] = createSignal<AppConfig | null>(
    initialConfig ? cloneJson(initialConfig) : null,
  )
  const [loadingInfo, setLoadingInfo] = createSignal(false)
  const [loadingConfig, setLoadingConfig] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [openingData, setOpeningData] = createSignal(false)
  const [errors, setErrors] = createSignal<Record<string, string>>({})

  const currentInfo = (): SystemInfo | null => info() ?? systemInfo()

  async function reloadInfo(reportError = false): Promise<void> {
    setLoadingInfo(true)
    try {
      const fresh = await api.system.info()
      setInfo(fresh)
    } catch (err) {
      if (reportError || !currentInfo()) {
        notify('error', '读取系统信息失败', errorText(err))
      }
    } finally {
      setLoadingInfo(false)
    }
  }

  async function reloadConfig(reportError = false): Promise<void> {
    setLoadingConfig(true)
    try {
      const fresh = await api.system.config()
      const copy = cloneJson(fresh)
      setConfig(copy)
      setAppConfig(copy)
      setErrors({})
    } catch (err) {
      if (reportError || !config()) {
        notify('error', '读取应用配置失败', errorText(err))
      }
    } finally {
      setLoadingConfig(false)
    }
  }

  onMount(() => {
    void reloadInfo(false)
    void reloadConfig(false)
  })

  function patchSection(mutator: (cfg: AppConfig) => AppConfig): void {
    setConfig((prev) => (prev ? mutator(cloneJson(prev)) : prev))
  }

  function patchPerformance(patch: Partial<AppConfig['performance']>): void {
    patchSection((cfg) => ({ ...cfg, performance: { ...cfg.performance, ...patch } }))
  }

  function patchCollector(patch: Partial<AppConfig['collector']>): void {
    patchSection((cfg) => ({ ...cfg, collector: { ...cfg.collector, ...patch } }))
  }

  function patchIndexer(patch: Partial<AppConfig['indexer']>): void {
    patchSection((cfg) => ({ ...cfg, indexer: { ...cfg.indexer, ...patch } }))
  }

  function patchParser(patch: Partial<AppConfig['parser']>): void {
    patchSection((cfg) => ({ ...cfg, parser: { ...cfg.parser, ...patch } }))
  }

  function patchUI(patch: Partial<AppConfig['ui']>): void {
    patchSection((cfg) => ({ ...cfg, ui: { ...cfg.ui, ...patch } }))
  }

  function validate(cfg: AppConfig): Record<string, string> {
    const next: Record<string, string> = {}
    const integer = (
      key: string,
      value: number,
      min: number,
      max: number,
      label: string,
    ): void => {
      if (!Number.isInteger(value) || value < min || value > max) {
        next[key] = `${label}需为 ${min}–${max} 的整数`
      }
    }
    integer('parser_workers', cfg.performance.parser_workers, 1, 128, '解析线程数')
    integer('indexer_workers', cfg.performance.indexer_workers, 1, 64, '索引线程数')
    integer('ai_task_workers', cfg.performance.ai_task_workers, 1, 64, 'AI 任务线程数')
    integer('query_timeout_seconds', cfg.performance.query_timeout_seconds, 1, 3600, '查询超时')
    integer('ssh_timeout_seconds', cfg.collector.ssh_timeout_seconds, 1, 3600, 'SSH 超时')
    integer('ssh_concurrency', cfg.collector.ssh_concurrency, 1, 128, 'SSH 并发数')
    integer('index_batch_size', cfg.indexer.index_batch_size, 1, 1_000_000, '索引批次大小')
    integer(
      'index_flush_interval_seconds',
      cfg.indexer.index_flush_interval_seconds,
      1,
      3600,
      '索引刷新间隔',
    )
    integer('multiline_timeout_ms', cfg.parser.multiline_timeout_ms, 100, 600_000, '多行超时')
    integer('multiline_max_lines', cfg.parser.multiline_max_lines, 1, 100_000, '多行最大行数')
    integer('log_page_size', cfg.ui.log_page_size, 10, 10_000, '日志分页大小')
    if (!cfg.collector.ssh_compression.trim()) {
      next.ssh_compression = '请选择 SSH 压缩方式'
    }
    if (!cfg.indexer.compression_algorithm.trim()) {
      next.compression_algorithm = '请选择索引压缩算法'
    }
    if (!cfg.ui.theme.trim()) next.theme = '请选择主题'
    if (!cfg.ui.language.trim()) next.language = '请选择语言'
    return next
  }

  async function saveConfig(): Promise<void> {
    const cfg = config()
    if (!cfg) return
    const nextErrors = validate(cfg)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      notify('warn', '配置校验失败', '请检查标红字段后重新保存。')
      return
    }
    setSaving(true)
    try {
      await api.system.updateConfig(cfg)
      setAppConfig(cloneJson(cfg))
      notify(
        'success',
        '配置已保存',
        '部分线程、缓存与索引参数需要重启 GLog 后生效。',
      )
    } catch (err) {
      notify('error', '保存配置失败', errorText(err))
    } finally {
      setSaving(false)
    }
  }

  async function openDataDir(): Promise<void> {
    const path = currentInfo()?.data_path
    if (!path) {
      notify('warn', '数据目录未知', '尚未获取系统信息，请先刷新。')
      return
    }
    setOpeningData(true)
    try {
      await api.system.showInExplorer(path)
    } catch (err) {
      notify('error', '打开数据目录失败', errorText(err))
    } finally {
      setOpeningData(false)
    }
  }

  return (
    <div class="stack">
      <SettingsPanel
        title="运行环境"
        note="只读系统信息。复制路径前请确认目标环境，避免将本机路径带入案件材料。"
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={loadingInfo()}
              onClick={() => void reloadInfo(true)}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
            <Button
              size="sm"
              loading={openingData()}
              disabled={!currentInfo()?.data_path}
              onClick={() => void openDataDir()}
            >
              <Icon name="folder" size={12} />
              在资源管理器中打开
            </Button>
          </>
        }
      >
        <Show
          when={currentInfo()}
          fallback={
            <Show
              when={loadingInfo()}
              fallback={
                <EmptyState
                  icon="cpu"
                  title="系统信息不可用"
                  text="无法从后端读取运行环境信息。"
                  action={
                    <Button variant="primary" onClick={() => void reloadInfo(true)}>
                      重新加载
                    </Button>
                  }
                />
              }
            >
              <PageBusy visible text="读取系统信息…" />
            </Show>
          }
        >
          {(sys) => (
            <div class="stack">
              <div class="row wrap">
                <Badge tone="mint">
                  {sys().os} / {sys().arch}
                </Badge>
                <Badge tone="sky">v{sys().version || '—'}</Badge>
                <Badge tone="muted">GO · WAILS</Badge>
              </div>
              <div style={{ 'overflow-x': 'auto' }}>
                <table class="data-table">
                  <thead>
                    <tr>
                      <th style={{ width: '142px' }}>项目</th>
                      <th>值</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={SYSTEM_FIELDS}>
                      {(field) => (
                        <tr>
                          <td
                            class="faint mono-sm"
                            style={{ 'white-space': 'nowrap' }}
                          >
                            {field.label}
                          </td>
                          <td>
                            <CopyableValue
                              value={sys()[field.key]}
                              label={field.label}
                            />
                          </td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </Show>
      </SettingsPanel>

      <Show
        when={config()}
        fallback={
          <div class="panel">
            <div class="panel-body stack">
              <Show
                when={loadingConfig()}
                fallback={
                  <EmptyState
                    icon="settings"
                    title="应用配置不可用"
                    text="无法从后端读取 AppConfig。设置页其余功能仍可使用。"
                    action={
                      <Button variant="primary" onClick={() => void reloadConfig(true)}>
                        重新加载
                      </Button>
                    }
                  />
                }
              >
                <PageBusy visible text="读取应用配置…" />
              </Show>
            </div>
          </div>
        }
      >
        {(cfg) => (
          <div class="stack">
            <SettingsPanel
              title="性能"
              note="解析、索引与 AI 任务的并行度。数值越高吞吐越大，但会占用更多 CPU 与内存。"
            >
              <div class="form-row">
                <Field
                  label="解析线程数"
                  hint="parser_workers"
                  error={errors().parser_workers}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().performance.parser_workers)}
                    onInput={(v) => patchPerformance({ parser_workers: toInt(v) })}
                  />
                </Field>
                <Field
                  label="索引线程数"
                  hint="indexer_workers"
                  error={errors().indexer_workers}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().performance.indexer_workers)}
                    onInput={(v) => patchPerformance({ indexer_workers: toInt(v) })}
                  />
                </Field>
                <Field
                  label="AI 任务线程数"
                  hint="ai_task_workers"
                  error={errors().ai_task_workers}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().performance.ai_task_workers)}
                    onInput={(v) => patchPerformance({ ai_task_workers: toInt(v) })}
                  />
                </Field>
                <Field
                  label="查询超时（秒）"
                  hint="query_timeout_seconds"
                  error={errors().query_timeout_seconds}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().performance.query_timeout_seconds)}
                    onInput={(v) =>
                      patchPerformance({ query_timeout_seconds: toInt(v) })
                    }
                  />
                </Field>
              </div>
            </SettingsPanel>

            <SettingsPanel
              title="采集"
              note="SSH 采集参数。超时与并发的调整会直接影响远程日志读取的稳定性。"
            >
              <div class="form-row">
                <Field
                  label="SSH 超时（秒）"
                  hint="ssh_timeout_seconds"
                  error={errors().ssh_timeout_seconds}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().collector.ssh_timeout_seconds)}
                    onInput={(v) =>
                      patchCollector({ ssh_timeout_seconds: toInt(v) })
                    }
                  />
                </Field>
                <Field
                  label="SSH 并发数"
                  hint="ssh_concurrency"
                  error={errors().ssh_concurrency}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().collector.ssh_concurrency)}
                    onInput={(v) => patchCollector({ ssh_concurrency: toInt(v) })}
                  />
                </Field>
                <Field
                  label="SSH 压缩"
                  hint="ssh_compression"
                  error={errors().ssh_compression}
                >
                  <Select
                    value={cfg().collector.ssh_compression}
                    options={withCurrentOption(
                      SSH_COMPRESSION_OPTIONS,
                      cfg().collector.ssh_compression,
                    )}
                    onInput={(v) => patchCollector({ ssh_compression: v })}
                  />
                </Field>
              </div>
            </SettingsPanel>

            <SettingsPanel
              title="索引"
              note="索引写入批次与压缩策略。批量越大写入越快，但异常退出时丢失的缓冲也更多。"
            >
              <div class="form-row">
                <Field
                  label="索引批次大小"
                  hint="index_batch_size"
                  error={errors().index_batch_size}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().indexer.index_batch_size)}
                    onInput={(v) => patchIndexer({ index_batch_size: toInt(v) })}
                  />
                </Field>
                <Field
                  label="刷新间隔（秒）"
                  hint="index_flush_interval_seconds"
                  error={errors().index_flush_interval_seconds}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().indexer.index_flush_interval_seconds)}
                    onInput={(v) =>
                      patchIndexer({ index_flush_interval_seconds: toInt(v) })
                    }
                  />
                </Field>
                <Field
                  label="压缩算法"
                  hint="compression_algorithm"
                  error={errors().compression_algorithm}
                >
                  <Select
                    value={cfg().indexer.compression_algorithm}
                    options={withCurrentOption(
                      COMPRESSION_OPTIONS,
                      cfg().indexer.compression_algorithm,
                    )}
                    onInput={(v) =>
                      patchIndexer({ compression_algorithm: v })
                    }
                  />
                </Field>
              </div>
            </SettingsPanel>

            <SettingsPanel
              title="解析"
              note="多行日志（如 Java 堆栈）合并的等待窗口与最大行数。"
            >
              <div class="form-row">
                <Field
                  label="多行超时（毫秒）"
                  hint="multiline_timeout_ms"
                  error={errors().multiline_timeout_ms}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().parser.multiline_timeout_ms)}
                    onInput={(v) =>
                      patchParser({ multiline_timeout_ms: toInt(v) })
                    }
                  />
                </Field>
                <Field
                  label="多行最大行数"
                  hint="multiline_max_lines"
                  error={errors().multiline_max_lines}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().parser.multiline_max_lines)}
                    onInput={(v) =>
                      patchParser({ multiline_max_lines: toInt(v) })
                    }
                  />
                </Field>
              </div>
            </SettingsPanel>

            <SettingsPanel
              title="界面"
              note="界面偏好立即写入配置文件；主题与语言将在下次启动时完整应用。"
            >
              <div class="form-row">
                <Field label="主题" hint="theme" error={errors().theme}>
                  <Select
                    value={cfg().ui.theme}
                    options={withCurrentOption(THEME_OPTIONS, cfg().ui.theme)}
                    onInput={(v) => patchUI({ theme: v })}
                  />
                </Field>
                <Field label="语言" hint="language" error={errors().language}>
                  <Select
                    value={cfg().ui.language}
                    options={withCurrentOption(
                      LANGUAGE_OPTIONS,
                      cfg().ui.language,
                    )}
                    onInput={(v) => patchUI({ language: v })}
                  />
                </Field>
                <Field
                  label="日志分页大小"
                  hint="log_page_size"
                  error={errors().log_page_size}
                >
                  <TextInput
                    type="number"
                    value={String(cfg().ui.log_page_size)}
                    onInput={(v) => patchUI({ log_page_size: toInt(v) })}
                  />
                </Field>
              </div>
            </SettingsPanel>

            <div class="panel">
              <div class="panel-body row between wrap" style={{ gap: '14px' }}>
                <div class="stack" style={{ gap: '4px' }}>
                  <span class="mono-sm faint">
                    配置写入数据目录下的 config.toml
                  </span>
                  <span class="mono-sm faint">
                    线程数、缓存、索引批次与解析超时等参数需重启 GLog 后生效。
                  </span>
                </div>
                <div class="row">
                  <Button
                    variant="ghost"
                    onClick={() => void reloadConfig(true)}
                    disabled={saving()}
                  >
                    <Icon name="refresh" size={12} />
                    还原已保存
                  </Button>
                  <Button
                    variant="primary"
                    loading={saving()}
                    onClick={() => void saveConfig()}
                  >
                    <Icon name="save" size={12} />
                    保存配置
                  </Button>
                </div>
              </div>
            </div>
          </div>
        )}
      </Show>
    </div>
  )
}
