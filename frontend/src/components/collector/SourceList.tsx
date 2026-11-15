import { For, Show, createMemo, createSignal, type JSX } from 'solid-js'
import type { LogSource, ParseConfig } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatBytes, formatNumber, formatRange, formatTime } from '../../utils'
import { Badge, Button, EmptyState, Icon, IconButton, Modal, Spinner } from '../ui'
import {
  fileBaseName,
  parseModeLabel,
  sourceTypeLabel,
  sourceTypeTone,
} from './shared'

export interface SourceListProps {
  projectId: string
  sources: LogSource[]
  loading?: boolean
  error?: string
  onRefresh?: () => void
  onTaskStarted?: () => void
}

/** 当前项目的日志源列表与详情弹窗。 */
export function SourceList(props: SourceListProps): JSX.Element {
  const [selected, setSelected] = createSignal<LogSource | null>(null)
  const [recollecting, setRecollecting] = createSignal(false)

  const canRecollect = (source: LogSource | null): boolean =>
    Boolean(source && (source.type === 'local' || source.type === 'import') && source.path)

  async function recollect(source: LogSource) {
    if (!canRecollect(source) || recollecting()) return
    setRecollecting(true)
    try {
      const parse: ParseConfig = {
        mode: source.parse_mode === 'ai' ? 'ai' : 'builtin',
        template_id: source.template_id || undefined,
        ai_provider: source.ai_provider || undefined,
      }
      if (source.type === 'import' && /\.evtx$/i.test(source.path)) {
        await api.collect.importFile({
          project_id: props.projectId,
          file_path: source.path,
          parse: { mode: 'native_evtx', template_id: 'windows-evtx' },
        })
      } else {
        const paths = source.path
          .split(/[;\n]+/)
          .map((value) => value.trim())
          .filter(Boolean)
        if (paths.length === 0) {
          notify('warn', '无法再次采集', '该日志源没有可用的本地路径。')
          return
        }
        await api.collect.startLocal({
          project_id: props.projectId,
          file_paths: paths,
          recursive: source.type === 'local',
          parse,
        })
      }
      notify('success', '已再次启动采集', '任务进度可在右侧队列查看，完成后日志源统计会刷新。')
      setSelected(null)
      props.onTaskStarted?.()
    } catch (err) {
      notify('error', '再次采集失败', errorText(err))
    } finally {
      setRecollecting(false)
    }
  }

  const sortedSources = createMemo(() =>
    props.sources
      .slice()
      .sort((a, b) => {
        const timeA = Date.parse(a.created_at || '')
        const timeB = Date.parse(b.created_at || '')
        return (Number.isNaN(timeB) ? 0 : timeB) - (Number.isNaN(timeA) ? 0 : timeA)
      }),
  )

  function sourceTitle(source: LogSource): string {
    if (source.host) return `${source.host} · ${sourceTypeLabel(source.type)}`
    return fileBaseName(source.path) || sourceTypeLabel(source.type)
  }

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          日志源
          <Badge tone="muted">{props.sources.length}</Badge>
        </div>
        <div class="row" style={{ gap: '4px' }}>
          <Show when={props.loading}>
            <Spinner />
          </Show>
          <IconButton
            icon="refresh"
            title="刷新日志源"
            disabled={props.loading}
            onClick={() => props.onRefresh?.()}
          />
        </div>
      </div>

      <Show when={props.error}>
        <div style={{ padding: '10px 12px' }}>
          <div class="danger-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span class="mono-sm">{props.error}</span>
              <Button size="sm" variant="ghost" onClick={() => props.onRefresh?.()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show
        when={sortedSources().length > 0}
        fallback={
          <div class="panel-body tight">
            <EmptyState
              icon="database"
              title="暂无日志源"
              text="完成一次采集或文件导入后，日志源会显示在这里。"
            />
          </div>
        }
      >
        <div class="panel-body tight scrollable" style={{ 'max-height': '360px' }}>
          <For each={sortedSources()}>
            {(source) => (
              <button
                type="button"
                class="list-row"
                title={source.path}
                onClick={() => setSelected(source)}
              >
                <div class="grow" style={{ 'min-width': 0 }}>
                  <div class="row wrap" style={{ gap: '5px' }}>
                    <Badge tone={sourceTypeTone(source.type)}>
                      {sourceTypeLabel(source.type)}
                    </Badge>
                    <span class="title truncate">{sourceTitle(source)}</span>
                  </div>
                  <div class="meta truncate" title={source.path}>
                    {source.path || '—'}
                  </div>
                  <div class="row wrap" style={{ gap: '5px', 'margin-top': '3px' }}>
                    <span class="mono-sm faint">
                      {formatNumber(source.log_count)} 条
                    </span>
                    <span class="mono-sm faint">· {formatBytes(source.size_bytes)}</span>
                    <span class="mono-sm faint">
                      · 模板 {source.template_id || '自动'}
                    </span>
                    <span class="mono-sm faint">
                      · {parseModeLabel(source.parse_mode)}
                    </span>
                  </div>
                  <div class="mono-sm faint truncate">
                    {source.time_range
                      ? formatRange(source.time_range.start, source.time_range.end)
                      : '时间范围未知'}
                  </div>
                </div>
                <Icon name="chevron-right" size={12} />
              </button>
            )}
          </For>
        </div>
      </Show>

      <Modal
        open={Boolean(selected())}
        title="日志源详情"
        subtitle={selected()?.id}
        size="lg"
        onClose={() => setSelected(null)}
        footer={
          <>
            <Show when={canRecollect(selected())}>
              <Button
                size="sm"
                variant="primary"
                loading={recollecting()}
                onClick={() => selected() && void recollect(selected()!)}
              >
                <Icon name="refresh" size={13} />
                再次采集
              </Button>
            </Show>
            <Button variant="ghost" onClick={() => setSelected(null)}>
              关闭
            </Button>
          </>
        }
      >
        <Show when={selected()} fallback={<div class="faint mono-sm">未选择日志源。</div>}>
          {(source) => (
            <div class="stack" style={{ gap: '14px' }}>
              <div class="row wrap" style={{ gap: '6px' }}>
                <Badge tone={sourceTypeTone(source().type)}>
                  {sourceTypeLabel(source().type)}
                </Badge>
                <Badge tone="sky">{parseModeLabel(source().parse_mode)}</Badge>
                <Show when={source().host}>
                  <Badge tone="violet">{source().host}</Badge>
                </Show>
              </div>

              <dl class="kv">
                <dt>源 ID</dt>
                <dd>{source().id}</dd>
                <dt>类型</dt>
                <dd>{sourceTypeLabel(source().type)}</dd>
                <dt>主机</dt>
                <dd>{source().host || '本机'}</dd>
                <dt>路径</dt>
                <dd style={{ 'word-break': 'break-all' }}>{source().path || '—'}</dd>
                <dt>解析模式</dt>
                <dd>{parseModeLabel(source().parse_mode)}</dd>
                <dt>模板</dt>
                <dd>{source().template_id || '自动匹配'}</dd>
                <Show when={source().ai_provider}>
                  <dt>AI 提供商</dt>
                  <dd>{source().ai_provider}</dd>
                </Show>
                <dt>日志条数</dt>
                <dd>{formatNumber(source().log_count)}</dd>
                <dt>大小</dt>
                <dd>{formatBytes(source().size_bytes)}</dd>
                <dt>时间范围</dt>
                <dd>
                  {source().time_range
                    ? formatRange(source().time_range?.start, source().time_range?.end)
                    : '未知'}
                </dd>
                <dt>创建时间</dt>
                <dd>{formatTime(source().created_at)}</dd>
              </dl>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  )
}
