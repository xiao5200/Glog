import { For, JSX, Show, createEffect, createMemo, createSignal, on } from 'solid-js'
import type { LogPattern, PatternClusterResult, Query } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import {
  Button,
  EmptyState,
  IconButton,
  Modal,
  PageBusy,
} from '../ui'
import {
  copyText,
  formatDuration,
  formatInt,
  formatTime,
} from '../../utils'
import { percentDisplay } from './logCommon'

export interface ClusterModalProps {
  open: boolean
  projectID: string
  query: Query
  onClose: () => void
}

const TOP_LIMIT = 100

export function ClusterModal(props: ClusterModalProps): JSX.Element {
  const [result, setResult] = createSignal<PatternClusterResult | null>(null)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [sortDesc, setSortDesc] = createSignal(true)
  const [expanded, setExpanded] = createSignal<ReadonlySet<string>>(new Set<string>())

  let requestSeq = 0

  const patternKey = (pattern: LogPattern, index: number) =>
    pattern.id || `${index}:${pattern.template}`

  async function loadClusters() {
    const projectID = props.projectID
    if (!projectID) return
    const seq = ++requestSeq
    setLoading(true)
    setError(null)
    setResult(null)
    setExpanded(new Set<string>())
    try {
      const data = await api.query.cluster(projectID, props.query)
      if (seq !== requestSeq) return
      setResult(data)
    } catch (err) {
      if (seq !== requestSeq) return
      const text = errorText(err)
      setError(text)
      notify('error', '模式聚类失败', text, 7000)
    } finally {
      if (seq === requestSeq) setLoading(false)
    }
  }

  createEffect(
    on(
      () => [props.open, props.projectID] as const,
      ([open]) => {
        if (!open) {
          requestSeq++
          setLoading(false)
          return
        }
        void loadClusters()
      },
    ),
  )

  const allPatterns = createMemo(() => result()?.patterns ?? [])

  const sortedPatterns = createMemo(() => {
    const list = allPatterns().slice()
    list.sort((a, b) => (sortDesc() ? b.count - a.count : a.count - b.count))
    return list
  })

  const visiblePatterns = createMemo(() => sortedPatterns().slice(0, TOP_LIMIT))

  function toggleExpanded(key: string) {
    setExpanded((previous) => {
      const next = new Set(previous)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  async function copyTemplate(pattern: LogPattern) {
    try {
      await copyText(pattern.template)
      notify('success', '模板已复制', pattern.template.slice(0, 80))
    } catch (err) {
      notify('error', '复制失败', errorText(err))
    }
  }

  const summary = () => {
    const data = result()
    if (!data) return ''
    const shown = Math.min(data.patterns?.length ?? 0, TOP_LIMIT)
    const totalPatterns = data.patterns?.length ?? 0
    return `共 ${formatInt(totalPatterns)} 个模式 · 覆盖 ${formatInt(data.total_count)} 条日志 · 耗时 ${formatDuration(data.took)} · 显示 ${shown} 个`
  }

  return (
    <Modal
      open={props.open}
      title="日志模式聚类"
      subtitle="按模板聚合当前查询结果，识别重复模式与参数分布"
      size="lg"
      onClose={props.onClose}
      footer={
        <>
          <span class="mono-sm faint" style={{ 'margin-right': 'auto' }}>
            {loading() ? '聚类计算中…' : summary()}
          </span>
          <Button
            size="sm"
            variant="ghost"
            loading={loading()}
            onClick={() => void loadClusters()}
          >
            重新聚类
          </Button>
          <Button size="sm" variant="primary" onClick={props.onClose}>
            关闭
          </Button>
        </>
      }
    >
      <PageBusy visible={loading()} text="正在对查询结果进行模式聚类…" />

      <Show when={error()}>
        <div class="danger-note" style={{ margin: '4px 0 10px' }}>
          <div class="row between" style={{ gap: '8px' }}>
            <span class="grow">{error()}</span>
            <Button size="sm" variant="ghost" onClick={() => void loadClusters()}>
              重试
            </Button>
          </div>
        </div>
      </Show>

      <Show when={!loading() && !error() && allPatterns().length === 0}>
        <EmptyState
          icon="layers"
          title="没有可聚类的日志"
          text="当前查询没有聚合出模式，请放宽筛选条件或先执行一次查询。"
        />
      </Show>

      <Show when={!loading() && !error() && allPatterns().length > 0}>
        <div class="row between wrap" style={{ gap: '8px', 'margin-bottom': '8px' }}>
          <div class="mono-sm faint" style={{ 'line-height': '1.6' }}>
            {summary()}
          </div>
          <span class="mono-sm faint">点击行展开示例与参数值</span>
        </div>

        <div
          class="scrollable scroll-thin"
          style={{
            'max-height': '58vh',
            overflow: 'auto',
            border: '1px solid var(--border)',
            'border-radius': '3px',
          }}
        >
          <table class="data-table">
            <thead>
              <tr>
                <th>模板</th>
                <th style={{ width: '90px' }}>
                  <button
                    type="button"
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      color: 'inherit',
                      font: 'inherit',
                      cursor: 'pointer',
                    }}
                    title="按出现次数切换排序"
                    onClick={() => setSortDesc((value) => !value)}
                  >
                    数量 {sortDesc() ? '↓' : '↑'}
                  </button>
                </th>
                <th style={{ width: '72px' }}>占比</th>
                <th style={{ width: '142px' }}>首次出现</th>
                <th style={{ width: '142px' }}>最近出现</th>
              </tr>
            </thead>
            <tbody>
              <For each={visiblePatterns()}>
                {(pattern, index) => {
                  const key = () => patternKey(pattern, index())
                  const isOpen = () => expanded().has(key())
                  return (
                    <>
                      <tr
                        style={{ cursor: 'pointer' }}
                        title="点击展开示例与参数值"
                        onClick={() => toggleExpanded(key())}
                      >
                        <td>
                          <div class="row" style={{ gap: '5px', 'align-items': 'flex-start' }}>
                            <span
                              style={{
                                flex: 'none',
                                width: '10px',
                                color: 'var(--accent-dim)',
                                'font-family': 'var(--font-mono)',
                                'font-size': '9px',
                                'line-height': '1.6',
                              }}
                            >
                              {isOpen() ? '▾' : '▸'}
                            </span>
                            <span
                              class="grow mono-sm"
                              style={{ 'word-break': 'break-word', 'line-height': '1.55' }}
                            >
                              {pattern.template}
                            </span>
                            <IconButton
                              icon="copy"
                              title="复制模板"
                              onClick={(event) => {
                                event.stopPropagation()
                                void copyTemplate(pattern)
                              }}
                            />
                          </div>
                        </td>
                        <td class="mono-sm">{formatInt(pattern.count)}</td>
                        <td class="mono-sm muted">{percentDisplay(pattern.percentage)}</td>
                        <td
                          class="mono-sm faint"
                          title={formatTime(pattern.first_seen)}
                          style={{ 'white-space': 'nowrap' }}
                        >
                          {formatTime(pattern.first_seen) || '—'}
                        </td>
                        <td
                          class="mono-sm faint"
                          title={formatTime(pattern.last_seen)}
                          style={{ 'white-space': 'nowrap' }}
                        >
                          {formatTime(pattern.last_seen) || '—'}
                        </td>
                      </tr>
                      <Show when={isOpen()}>
                        <tr>
                          <td
                            colspan="5"
                            style={{
                              background: 'var(--surface-soft)',
                              padding: '8px 10px',
                            }}
                          >
                            <div class="stack" style={{ gap: '8px' }}>
                              <div>
                                <div class="mono-sm faint" style={{ 'margin-bottom': '4px' }}>
                                  示例
                                </div>
                                <Show
                                  when={(pattern.examples?.length ?? 0) > 0}
                                  fallback={<span class="mono-sm faint">无示例</span>}
                                >
                                  <div class="stack" style={{ gap: '4px' }}>
                                    <For each={pattern.examples ?? []}>
                                      {(example) => (
                                        <pre
                                          class="code-block"
                                          style={{
                                            margin: 0,
                                            padding: '5px 8px',
                                            'max-height': '110px',
                                            'font-size': '10px',
                                            'line-height': '1.55',
                                          }}
                                        >
                                          {example}
                                        </pre>
                                      )}
                                    </For>
                                  </div>
                                </Show>
                              </div>

                              <div>
                                <div class="mono-sm faint" style={{ 'margin-bottom': '4px' }}>
                                  参数值
                                </div>
                                <Show
                                  when={Object.keys(pattern.parameters ?? {}).length > 0}
                                  fallback={<span class="mono-sm faint">无参数提取结果</span>}
                                >
                                  <div class="stack" style={{ gap: '5px' }}>
                                    <For each={Object.entries(pattern.parameters ?? {})}>
                                      {([name, values]) => (
                                        <div
                                          class="row wrap"
                                          style={{ gap: '4px', 'align-items': 'center' }}
                                        >
                                          <span
                                            class="mono-sm muted"
                                            style={{ 'min-width': '88px' }}
                                          >
                                            {name}
                                          </span>
                                          <For each={(values ?? []).slice(0, 12)}>
                                            {(value) => (
                                              <span class="tag" title={value}>
                                                {value}
                                              </span>
                                            )}
                                          </For>
                                          <Show when={(values?.length ?? 0) > 12}>
                                            <span class="mono-sm faint">
                                              +{(values?.length ?? 0) - 12}
                                            </span>
                                          </Show>
                                        </div>
                                      )}
                                    </For>
                                  </div>
                                </Show>
                              </div>
                            </div>
                          </td>
                        </tr>
                      </Show>
                    </>
                  )
                }}
              </For>
            </tbody>
          </table>
        </div>

        <Show when={allPatterns().length > TOP_LIMIT}>
          <div class="detail-note" style={{ 'margin-top': '8px' }}>
            模式总数超过 {TOP_LIMIT}，当前仅展示按数量排序后的前 {TOP_LIMIT} 个。
          </div>
        </Show>
      </Show>
    </Modal>
  )
}
