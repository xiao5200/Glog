import { For, JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type { LogEntry } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { Button, EmptyState, LevelBadge, Modal, PageBusy, Segmented } from '../ui'
import { formatTime, formatTimeShort, friendlyLogMessage } from '../../utils'
import { basename } from './logCommon'

export interface ContextModalProps {
  open: boolean
  projectID: string
  docID: number | null
  onClose: () => void
  onSelectEntry: (entry: LogEntry) => void
}

const WINDOW_OPTIONS = [
  { value: '5', label: '±5' },
  { value: '10', label: '±10' },
  { value: '20', label: '±20' },
  { value: '50', label: '±50' },
  { value: '100', label: '±100' },
  { value: '200', label: '±200' },
]

const WINDOW_STORAGE_KEY = 'glog.context_window'
const DEFAULT_WINDOW = 10

function loadWindowSize(): number {
  try {
    const saved = Number(window.localStorage.getItem(WINDOW_STORAGE_KEY))
    if (WINDOW_OPTIONS.some((option) => Number(option.value) === saved)) return saved
  } catch {
    // localStorage 不可用时使用默认值。
  }
  return DEFAULT_WINDOW
}

export function ContextModal(props: ContextModalProps): JSX.Element {
  const [entries, setEntries] = createSignal<LogEntry[]>([])
  const [anchorIndex, setAnchorIndex] = createSignal(-1)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [windowSize, setWindowSize] = createSignal(loadWindowSize())

  let requestSeq = 0

  function changeWindow(value: string) {
    const size = Number(value)
    if (!Number.isFinite(size) || size <= 0) return
    setWindowSize(size)
    try {
      window.localStorage.setItem(WINDOW_STORAGE_KEY, String(size))
    } catch {
      // 忽略持久化失败，仅影响下次打开的默认窗口。
    }
  }

  async function loadContext() {
    const projectID = props.projectID
    const docID = props.docID
    if (!projectID || docID == null) return
    const size = windowSize()
    const seq = ++requestSeq
    setLoading(true)
    setError(null)
    setEntries([])
    setAnchorIndex(-1)
    try {
      const result = await api.query.context(projectID, docID, size, size)
      if (seq !== requestSeq) return
      setEntries(Array.isArray(result?.entries) ? result.entries : [])
      setAnchorIndex(Number.isFinite(result?.anchor_index) ? Number(result.anchor_index) : -1)
    } catch (err) {
      if (seq !== requestSeq) return
      const text = errorText(err)
      setError(text)
      notify('error', '上下文加载失败', text, 7000)
    } finally {
      if (seq === requestSeq) setLoading(false)
    }
  }

  createEffect(
    on(
      () => [props.open, props.projectID, props.docID, windowSize()] as const,
      ([open]) => {
        if (!open) {
          requestSeq++
          setLoading(false)
          return
        }
        void loadContext()
      },
    ),
  )

  const isAnchor = (index: number) => index === anchorIndex()

  return (
    <Modal
      open={props.open}
      title="日志上下文"
      subtitle={`doc #${props.docID ?? '—'} · 前后各 ${windowSize()} 条 · 点击任意行切换详情`}
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <span class="mono-sm faint" style={{ 'margin-right': 'auto' }}>
            高亮行是当前日志锚点
          </span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void loadContext()}
            disabled={loading()}
          >
            重新加载
          </Button>
          <Button size="sm" variant="primary" onClick={props.onClose}>
            关闭
          </Button>
        </>
      }
    >
      <PageBusy visible={loading()} text="正在读取相邻日志…" />

      <div
        class="row"
        style={{
          gap: '10px',
          'align-items': 'center',
          'flex-wrap': 'wrap',
          'margin': '4px 0 10px',
        }}
      >
        <span class="mono-sm faint">上下文窗口</span>
        <Segmented
          options={WINDOW_OPTIONS}
          value={String(windowSize())}
          onChange={changeWindow}
        />
        <span class="mono-sm faint" style={{ 'margin-left': 'auto' }}>
          单侧最多 500 条，选择会记住
        </span>
      </div>

      <Show when={error()}>
        <div class="danger-note" style={{ margin: '4px 0 10px' }}>
          <div class="row between" style={{ gap: '8px' }}>
            <span class="grow">{error()}</span>
            <Button size="sm" variant="ghost" onClick={() => void loadContext()}>
              重试
            </Button>
          </div>
        </div>
      </Show>

      <Show when={!loading() && !error() && entries().length === 0}>
        <EmptyState
          icon="layers"
          title="无上下文日志"
          text="当前日志可能已被删除或索引尚未包含相邻记录。"
        />
      </Show>

      <Show when={entries().length > 0}>
        <Show when={anchorIndex() < 0 || anchorIndex() >= entries().length}>
          <div class="detail-note" style={{ margin: '0 0 8px' }}>
            返回结果中未找到当前日志锚点，以下为查询到的相邻日志。
          </div>
        </Show>
        <div
          class="scrollable scroll-thin"
          style={{
            'max-height': '56vh',
            border: '1px solid var(--border)',
            'border-radius': '3px',
            overflow: 'auto',
          }}
        >
          <For each={entries()}>
            {(entry, index) => {
              const anchor = () => isAnchor(index())
              return (
                <div
                  role="button"
                  tabindex="0"
                  title={`doc #${entry.doc_id} · 点击切换详情`}
                  style={{
                    display: 'grid',
                    'grid-template-columns': '126px 58px minmax(0, 1fr) 76px',
                    gap: '8px',
                    'align-items': 'start',
                    padding: '6px 9px',
                    'border-left': anchor()
                      ? '2px solid var(--accent)'
                      : '2px solid transparent',
                    'border-bottom': '1px solid var(--border)',
                    background: anchor()
                      ? 'var(--accent-ghost)'
                      : index() % 2 === 0
                        ? 'transparent'
                        : 'var(--surface-soft)',
                    cursor: 'pointer',
                  }}
                  onClick={() => props.onSelectEntry(entry)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      props.onSelectEntry(entry)
                    }
                  }}
                >
                  <div style={{ 'min-width': 0 }}>
                    <div class="log-time" title={formatTime(entry.timestamp)}>
                      {formatTimeShort(entry.timestamp)}
                    </div>
                    <div
                      class="mono-sm faint"
                      style={{ 'font-size': '8.5px', 'margin-top': '2px' }}
                    >
                      {anchor() ? 'ANCHOR' : `#${entry.doc_id}`}
                    </div>
                  </div>
                  <LevelBadge level={entry.level} />
                  <div style={{ 'min-width': 0 }}>
                    <div class="truncate" style={{ 'font-size': '11px', color: 'var(--text)' }}>
                      {friendlyLogMessage(entry.message) || '(空消息)'}
                    </div>
                    <div
                      class="mono-sm faint truncate"
                      style={{ 'font-size': '9px', 'margin-top': '2px' }}
                      title={`${entry.source_host || '未知主机'} · ${entry.source_file || ''}`}
                    >
                      {entry.source_host || '未知主机'} · {basename(entry.source_file) || entry.source || 'unknown'}
                    </div>
                  </div>
                  <div
                    class="mono-sm faint truncate"
                    style={{ 'font-size': '9px', 'text-align': 'right' }}
                    title={entry.source_file || ''}
                  >
                    {entry.event_id != null ? `EVT ${entry.event_id}` : '—'}
                  </div>
                </div>
              )
            }}
          </For>
        </div>
      </Show>
    </Modal>
  )
}
