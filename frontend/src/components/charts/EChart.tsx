import { JSX, createEffect, onCleanup, onMount } from 'solid-js'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

export const chartPalette = {
  mint: '#2563eb',
  mintDim: '#1d4ed8',
  amber: '#f79009',
  ember: '#f04438',
  sky: '#0ba5ec',
  violet: '#7a5af8',
  text: '#475467',
  faint: '#98a2b3',
  grid: 'rgba(152, 162, 179, 0.18)',
  axis: '#eaecf0',
  tooltipBg: '#ffffff',
}

export function baseChartOption(): EChartsOption {
  return {
    backgroundColor: 'transparent',
    textStyle: {
      color: chartPalette.text,
      fontFamily: 'IBM Plex Sans, Microsoft YaHei, sans-serif',
      fontSize: 11,
    },
    animationDuration: 320,
    animationEasing: 'cubicOut',
    grid: { left: 44, right: 18, top: 28, bottom: 28, containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: chartPalette.tooltipBg,
      borderColor: '#e4e7ec',
      borderWidth: 1,
      padding: [8, 10],
      textStyle: { color: '#1d2939', fontSize: 11, fontFamily: 'IBM Plex Sans, Microsoft YaHei, sans-serif' },
      extraCssText: 'box-shadow: 0 4px 12px rgba(16, 24, 40, 0.08); border-radius: 8px;',
    },
    xAxis: {
      axisLine: { lineStyle: { color: chartPalette.axis } },
      axisTick: { show: false },
      axisLabel: { color: chartPalette.faint, fontSize: 10 },
      splitLine: { show: false },
    },
    yAxis: {
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: chartPalette.faint, fontSize: 10 },
      splitLine: { lineStyle: { color: chartPalette.grid, type: 'dashed' } },
    },
    legend: {
      textStyle: { color: chartPalette.text, fontSize: 10, fontFamily: 'IBM Plex Sans, Microsoft YaHei, sans-serif' },
      itemWidth: 10,
      itemHeight: 10,
      top: 2,
    },
  }
}

export function EChart(props: {
  option: EChartsOption
  height?: string
  class?: string
  onEvents?: Record<string, (params: unknown) => void>
}): JSX.Element {
  let container: HTMLDivElement | undefined
  let chart: echarts.ECharts | undefined
  let observer: ResizeObserver | undefined

  onMount(() => {
    if (!container) return
    chart = echarts.init(container, undefined, { renderer: 'canvas' })
    chart.setOption(props.option, true)
    if (props.onEvents) {
      for (const [event, handler] of Object.entries(props.onEvents)) {
        chart.on(event, handler)
      }
    }
    observer = new ResizeObserver(() => chart?.resize())
    observer.observe(container)
  })

  createEffect(() => {
    const option = props.option
    if (chart) chart.setOption(option, true)
  })

  onCleanup(() => {
    observer?.disconnect()
    chart?.dispose()
  })

  return (
    <div
      ref={container}
      class={`chart-frame ${props.class ?? ''}`}
      style={{ height: props.height ?? '220px' }}
    />
  )
}
