import { JSX, Show, createEffect, createMemo, createSignal, onCleanup } from 'solid-js'
import type { EChartsOption } from 'echarts'
import type { AggregateResult, Query, TimeSeriesPoint } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { Button, EmptyState, IconButton, Select, Spinner } from '../ui'
import { formatInt, formatTimeShort, truncate } from '../../utils'
import { EChart, baseChartOption, chartPalette } from './EChart'

export interface LogChartsProps {
  projectID: string
  query: Query
  reloadKey: number
  onCollapse: () => void
}

interface CountItem {
  key: string
  count: number
}

const CHART_HEIGHT = '176px'

const DIMENSION_OPTIONS: Array<{ value: string; label: string }> = [
  { value: 'event_type', label: '事件类型' },
  { value: 'source_ip', label: '来源 IP' },
  { value: 'destination_ip', label: '目的 IP' },
  { value: 'event_id', label: '事件 ID' },
  { value: 'source_host', label: '主机' },
  { value: 'level', label: '级别' },
]

function aggregateQuery(query: Query): Query {
  const clone: Query = { ...query }
  delete clone.offset
  delete clone.limit
  delete clone.sort_by
  delete clone.sort_order
  return clone
}

function countItems(result: AggregateResult | null | undefined): CountItem[] {
  if (!result) return []
  if (result.distribution?.length) {
    return result.distribution.map((item) => ({ key: item.key, count: item.count }))
  }
  if (result.top_items?.length) {
    return result.top_items.map((item) => ({ key: item.key, count: item.count }))
  }
  return []
}

function levelColor(level: string): string {
  switch (String(level ?? '').toUpperCase()) {
    case 'INFO':
      return chartPalette.sky
    case 'WARN':
    case 'WARNING':
      return chartPalette.amber
    case 'ERROR':
      return chartPalette.ember
    default:
      return chartPalette.mint
  }
}

function horizontalBarOption(items: CountItem[], fallback: string, maxLabel = 22): EChartsOption {
  const names = items.map((item) => truncate(item.key || fallback, maxLabel))
  const values = items.map((item) => Number(item.count) || 0)
  return {
    ...baseChartOption(),
    animationDuration: 360,
    grid: { left: 4, right: 18, top: 8, bottom: 4, containLabel: true },
    legend: { show: false },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: chartPalette.tooltipBg,
      borderColor: 'var(--border-strong)',
      borderWidth: 1,
      textStyle: {
        color: '#1d2939',
        fontSize: 10,
        fontFamily: 'IBM Plex Mono, monospace',
      },
    },
    xAxis: {
      type: 'value',
      minInterval: 1,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: chartPalette.faint, fontSize: 9 },
      splitLine: { lineStyle: { color: chartPalette.grid, type: 'dashed' } },
    },
    yAxis: {
      type: 'category',
      data: names.slice().reverse(),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: chartPalette.text, fontSize: 9 },
    },
    series: [
      {
        name: '日志量',
        type: 'bar',
        data: values.slice().reverse(),
        barWidth: '55%',
        itemStyle: {
          color: chartPalette.mintDim,
          borderRadius: [0, 2, 2, 0],
        },
        emphasis: { itemStyle: { color: chartPalette.mint } },
      },
    ],
  }
}

function ChartSkeleton(): JSX.Element {
  return (
    <div
      style={{
        height: CHART_HEIGHT,
        display: 'flex',
        'flex-direction': 'column',
        gap: '8px',
        padding: '14px 12px',
      }}
    >
      <span
        style={{
          display: 'block',
          height: '10px',
          width: '44%',
          background: 'var(--border)',
          animation: 'pulseSoft 1.4s ease-in-out infinite',
        }}
      />
      <div class="row" style={{ gap: '5px', 'align-items': 'flex-end', flex: 1 }}>
        {[34, 52, 28, 66, 44, 74, 38, 58, 46].map((height) => (
          <span
            style={{
              display: 'block',
              flex: 1,
              height: `${height}%`,
              background: 'var(--border)',
              animation: 'pulseSoft 1.4s ease-in-out infinite',
            }}
          />
        ))}
      </div>
    </div>
  )
}

function ChartError(props: { message: string; onRetry: () => void }): JSX.Element {
  return (
    <div
      style={{
        height: CHART_HEIGHT,
        display: 'grid',
        'place-items': 'center',
        padding: '0 8px',
      }}
    >
      <div class="danger-note" style={{ width: '100%' }}>
        <div class="row between" style={{ gap: '8px' }}>
          <span class="grow truncate" title={props.message} style={{ 'font-size': '10px' }}>
            聚合失败：{props.message}
          </span>
          <Button size="sm" variant="ghost" onClick={props.onRetry}>
            重试
          </Button>
        </div>
      </div>
    </div>
  )
}

function ChartEmpty(): JSX.Element {
  return (
    <div
      style={{
        height: CHART_HEIGHT,
        display: 'grid',
        'place-items': 'center',
        overflow: 'auto',
      }}
    >
      <EmptyState icon="chart" title="暂无聚合数据" />
    </div>
  )
}

function ChartSection(props: {
  title: string
  note?: string
  loading: boolean
  error: string | null
  empty: boolean
  option: EChartsOption
  onRetry: () => void
  flex?: string
  minWidth?: string
}): JSX.Element {
  return (
    <div
      style={{
        flex: props.flex ?? '1 1 0',
        'min-width': props.minWidth ?? '270px',
        display: 'flex',
        'flex-direction': 'column',
        'min-height': 0,
        background: 'var(--bg-raised)',
      }}
    >
      <div
        class="row between"
        style={{
          padding: '4px 9px',
          'border-bottom': '1px solid var(--border)',
          gap: '6px',
          'flex': 'none',
          'min-height': '24px',
        }}
      >
        <span
          class="mono-sm"
          style={{ color: 'var(--text-muted)', 'font-size': '9px', 'letter-spacing': '0.08em' }}
        >
          {props.title}
        </span>
        <span class="row" style={{ gap: '5px' }}>
          <Show when={props.note}>
            <span class="mono-sm faint" style={{ 'font-size': '8.5px' }}>
              {props.note}
            </span>
          </Show>
          <Show when={props.loading}>
            <Spinner />
          </Show>
        </span>
      </div>
      <div style={{ flex: 1, 'min-height': 0, 'overflow': 'hidden' }}>
        <Show when={!props.loading} fallback={<ChartSkeleton />}>
          <Show
            when={!props.error}
            fallback={
              <ChartError message={props.error ?? '未知错误'} onRetry={props.onRetry} />
            }
          >
            <Show when={!props.empty} fallback={<ChartEmpty />}>
              <EChart option={props.option} height="100%" />
            </Show>
          </Show>
        </Show>
      </div>
    </div>
  )
}

export function LogCharts(props: LogChartsProps): JSX.Element {
  const [timeline, setTimeline] = createSignal<TimeSeriesPoint[]>([])
  const [levelItems, setLevelItems] = createSignal<CountItem[]>([])
  const [hostItems, setHostItems] = createSignal<CountItem[]>([])
  const [dimension, setDimension] = createSignal('event_type')
  const [dimensionItems, setDimensionItems] = createSignal<CountItem[]>([])
  const [timelineError, setTimelineError] = createSignal<string | null>(null)
  const [levelError, setLevelError] = createSignal<string | null>(null)
  const [hostError, setHostError] = createSignal<string | null>(null)
  const [dimensionError, setDimensionError] = createSignal<string | null>(null)
  const [loading, setLoading] = createSignal(false)

  let requestSeq = 0
  let debounceTimer: number | undefined

  function clearTimer() {
    if (debounceTimer) {
      window.clearTimeout(debounceTimer)
      debounceTimer = undefined
    }
  }

  function clearData() {
    setTimeline([])
    setLevelItems([])
    setHostItems([])
    setDimensionItems([])
    setTimelineError(null)
    setLevelError(null)
    setHostError(null)
    setDimensionError(null)
  }

  async function fetchCharts(projectID: string, query: Query, seq: number) {
    const request: Query = aggregateQuery(query)
    const dimensionField = dimension()
    const results = await Promise.allSettled([
      api.query.aggregate(projectID, {
        query: request,
        aggregate_by: ['hour'],
        time_bucket: 'auto',
        top_n: 10,
      }),
      api.query.aggregate(projectID, {
        query: request,
        aggregate_by: ['level'],
        top_n: 10,
      }),
      api.query.aggregate(projectID, {
        query: request,
        aggregate_by: ['source_host'],
        top_n: 10,
      }),
      api.query.aggregate(projectID, {
        query: request,
        aggregate_by: [dimensionField],
        top_n: 10,
      }),
    ])
    if (seq !== requestSeq) return

    const failed: string[] = []
    let firstError = ''

    const [timelineResult, levelResult, hostResult, dimensionResult] = results

    if (timelineResult.status === 'fulfilled') {
      setTimeline(Array.isArray(timelineResult.value?.time_series) ? timelineResult.value.time_series : [])
    } else {
      const message = errorText(timelineResult.reason)
      setTimelineError(message)
      failed.push('时间线')
      if (!firstError) firstError = message
    }

    if (levelResult.status === 'fulfilled') {
      const items = countItems(levelResult.value)
        .slice()
        .sort((a, b) => b.count - a.count)
      setLevelItems(items)
    } else {
      const message = errorText(levelResult.reason)
      setLevelError(message)
      failed.push('级别分布')
      if (!firstError) firstError = message
    }

    if (hostResult.status === 'fulfilled') {
      const items = countItems(hostResult.value)
        .slice()
        .sort((a, b) => b.count - a.count)
        .slice(0, 10)
      setHostItems(items)
    } else {
      const message = errorText(hostResult.reason)
      setHostError(message)
      failed.push('主机 TOP')
      if (!firstError) firstError = message
    }

    if (dimensionResult.status === 'fulfilled') {
      const items = countItems(dimensionResult.value)
        .slice()
        .sort((a, b) => b.count - a.count)
        .slice(0, 10)
      setDimensionItems(items)
    } else {
      const message = errorText(dimensionResult.reason)
      setDimensionError(message)
      failed.push('维度聚合')
      if (!firstError) firstError = message
    }

    setLoading(false)

    if (failed.length > 0) {
      notify(
        'error',
        '部分聚合图表加载失败',
        `${failed.join('、')}：${firstError}`,
        6000,
      )
    }
  }

  function scheduleFetch() {
    clearTimer()
    const projectID = props.projectID
    if (!projectID) {
      requestSeq++
      clearData()
      setLoading(false)
      return
    }
    const seq = ++requestSeq
    clearData()
    setLoading(true)
    const query = props.query
    debounceTimer = window.setTimeout(() => {
      debounceTimer = undefined
      void fetchCharts(projectID, query, seq)
    }, 300)
  }

  function refreshNow() {
    clearTimer()
    const projectID = props.projectID
    if (!projectID) {
      requestSeq++
      clearData()
      setLoading(false)
      return
    }
    const seq = ++requestSeq
    clearData()
    setLoading(true)
    void fetchCharts(projectID, props.query, seq)
  }

  createEffect(() => {
    void props.projectID
    void props.query
    void props.reloadKey
    void dimension()
    scheduleFetch()
  })

  onCleanup(() => {
    requestSeq++
    clearTimer()
  })

  const timelineTotal = createMemo(() =>
    timeline().reduce((acc, point) => acc + (Number(point.count) || 0), 0),
  )

  const levelTotal = createMemo(() =>
    levelItems().reduce((acc, item) => acc + (Number(item.count) || 0), 0),
  )

  const hostTotal = createMemo(() =>
    hostItems().reduce((acc, item) => acc + (Number(item.count) || 0), 0),
  )

  const dimensionTotal = createMemo(() =>
    dimensionItems().reduce((acc, item) => acc + (Number(item.count) || 0), 0),
  )

  const dimensionLabel = createMemo(
    () => DIMENSION_OPTIONS.find((item) => item.value === dimension())?.label ?? dimension(),
  )

  const timelineOption = createMemo<EChartsOption>(() => {
    const points = timeline()
    return {
      ...baseChartOption(),
      animationDuration: 300,
      grid: { left: 38, right: 14, top: 14, bottom: 20, containLabel: true },
      legend: { show: false },
      tooltip: {
        trigger: 'axis',
        backgroundColor: chartPalette.tooltipBg,
        borderColor: 'var(--border-strong)',
        borderWidth: 1,
        textStyle: {
          color: '#1d2939',
          fontSize: 10,
          fontFamily: 'IBM Plex Mono, monospace',
        },
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: points.map((point) => formatTimeShort(point.timestamp)),
        axisLine: { lineStyle: { color: chartPalette.axis } },
        axisTick: { show: false },
        axisLabel: { color: chartPalette.faint, fontSize: 9, hideOverlap: true },
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: chartPalette.faint, fontSize: 9 },
        splitLine: { lineStyle: { color: chartPalette.grid, type: 'dashed' } },
      },
      series: [
        {
          name: '日志量',
          type: 'line',
          smooth: true,
          showSymbol: false,
          symbolSize: 4,
          data: points.map((point) => Number(point.count) || 0),
          lineStyle: { color: chartPalette.mint, width: 1.6 },
          itemStyle: { color: chartPalette.mint },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'var(--accent)' },
                { offset: 1, color: 'rgba(37, 99, 235, 0.04)' },
              ],
            },
          },
          emphasis: { focus: 'series' },
        },
      ],
    }
  })

  const levelOption = createMemo<EChartsOption>(() => {
    const items = levelItems()
    return {
      ...baseChartOption(),
      animationDuration: 360,
      tooltip: {
        trigger: 'item',
        backgroundColor: chartPalette.tooltipBg,
        borderColor: 'var(--border-strong)',
        borderWidth: 1,
        textStyle: {
          color: '#1d2939',
          fontSize: 10,
          fontFamily: 'IBM Plex Mono, monospace',
        },
      },
      legend: {
        bottom: 0,
        left: 'center',
        textStyle: {
          color: chartPalette.text,
          fontSize: 9,
          fontFamily: 'IBM Plex Mono, monospace',
        },
        itemWidth: 8,
        itemHeight: 8,
        itemGap: 8,
      },
      graphic: [
        {
          type: 'text',
          left: 'center',
          top: '34%',
          silent: true,
          style: {
            text: formatInt(levelTotal()),
            fill: chartPalette.mint,
            fontSize: 15,
            fontWeight: 600,
            fontFamily: 'IBM Plex Mono, Consolas, monospace',
            textAlign: 'center',
          },
        },
        {
          type: 'text',
          left: 'center',
          top: '49%',
          silent: true,
          style: {
            text: '总日志',
            fill: chartPalette.faint,
            fontSize: 9,
            fontFamily: 'IBM Plex Mono, Consolas, monospace',
            textAlign: 'center',
          },
        },
      ],
      series: [
        {
          name: '日志级别',
          type: 'pie',
          radius: ['46%', '66%'],
          center: ['50%', '42%'],
          avoidLabelOverlap: true,
          itemStyle: {
            borderColor: '#ffffff',
            borderWidth: 1,
          },
          label: { show: false },
          labelLine: { show: false },
          emphasis: {
            itemStyle: { shadowBlur: 8, shadowColor: 'rgba(16, 24, 40, 0.12)' },
          },
          data: items.map((item) => ({
            name: item.key || 'UNKNOWN',
            value: Number(item.count) || 0,
            itemStyle: { color: levelColor(item.key) },
          })),
        },
      ],
    }
  })

  const hostOption = createMemo<EChartsOption>(() =>
    horizontalBarOption(hostItems(), '未知主机', 22),
  )

  const dimensionOption = createMemo<EChartsOption>(() =>
    horizontalBarOption(dimensionItems(), '未知维度', 24),
  )

  return (
    <section
      class="panel"
      style={{
        flex: 'none',
        height: '278px',
        display: 'flex',
        'flex-direction': 'column',
        'min-height': 0,
      }}
    >
      <style>{`
        .logcharts-dimension-select {
          width: 88px;
          height: 22px;
          min-height: 22px;
          padding: 1px 18px 1px 6px;
          font-size: 10px;
          border-radius: 5px;
          background-position: calc(100% - 11px) 8px, calc(100% - 6px) 8px;
        }
      `}</style>
      <div
        class="panel-head"
        style={{ 'min-height': '32px', padding: '5px 10px', 'flex': 'none' }}
      >
        <div class="panel-title">
          <span class="tick" />
          聚合图表
        </div>
        <div class="row" style={{ gap: '6px' }}>
          <div class="row" style={{ gap: '4px' }}>
            <span class="mono-sm faint" style={{ 'font-size': '9px' }}>
              维度
            </span>
            <Select
              value={dimension()}
              options={DIMENSION_OPTIONS}
              onInput={(value) => setDimension(value || 'event_type')}
              disabled={loading()}
              class="logcharts-dimension-select"
            />
          </div>
          <span class="mono-sm faint" style={{ 'font-size': '9px' }}>
            {loading() ? '聚合中…' : '当前查询'}
          </span>
          <IconButton
            icon="refresh"
            title="刷新聚合图表"
            disabled={loading()}
            onClick={refreshNow}
          />
          <IconButton icon="x" title="收起图表面板" onClick={props.onCollapse} />
        </div>
      </div>

      <div
        class="scrollable scroll-thin"
        style={{
          flex: 1,
          'min-height': 0,
          display: 'flex',
          gap: '1px',
          background: 'var(--border)',
          'overflow-x': 'auto',
          'overflow-y': 'hidden',
        }}
      >
        <ChartSection
          title="时间线 · 按小时"
          note={timeline().length > 0 ? `${formatInt(timelineTotal())} 条` : undefined}
          loading={loading()}
          error={timelineError()}
          empty={timeline().length === 0}
          option={timelineOption()}
          onRetry={refreshNow}
          flex="1.7 1 0"
          minWidth="440px"
        />
        <ChartSection
          title="级别分布"
          note={levelTotal() > 0 ? `${formatInt(levelTotal())} 条` : undefined}
          loading={loading()}
          error={levelError()}
          empty={levelItems().length === 0}
          option={levelOption()}
          onRetry={refreshNow}
          minWidth="260px"
        />
        <ChartSection
          title="主机 TOP 10"
          note={hostTotal() > 0 ? `${formatInt(hostTotal())} 条` : undefined}
          loading={loading()}
          error={hostError()}
          empty={hostItems().length === 0}
          option={hostOption()}
          onRetry={refreshNow}
          minWidth="280px"
        />
        <ChartSection
          title={`维度聚合 · ${dimensionLabel()}`}
          note={dimensionTotal() > 0 ? `${formatInt(dimensionTotal())} 条` : undefined}
          loading={loading()}
          error={dimensionError()}
          empty={dimensionItems().length === 0}
          option={dimensionOption()}
          onRetry={refreshNow}
          minWidth="300px"
        />
      </div>
    </section>
  )
}
