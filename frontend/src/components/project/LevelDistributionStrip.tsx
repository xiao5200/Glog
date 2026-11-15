import { JSX, Show, createMemo } from 'solid-js'
import { LevelBadge } from '../ui'
import { formatNumber, percentWidth, sum } from '../../utils'

export interface LevelSegments {
  info: number
  warn: number
  error: number
  other: number
  total: number
}

/** 将原始 level_distribution 归并为 INFO / WARN / ERROR / 其他。 */
export function getLevelSegments(
  distribution: Record<string, number> | undefined,
  fallbackTotal = 0,
): LevelSegments {
  let info = 0
  let warn = 0
  let error = 0

  for (const [rawLevel, rawCount] of Object.entries(distribution ?? {})) {
    const level = rawLevel.toUpperCase()
    const count = Number(rawCount)
    if (!Number.isFinite(count) || count <= 0) continue
    if (level === 'INFO') {
      info += count
    } else if (level === 'WARN' || level === 'WARNING') {
      warn += count
    } else if (level === 'ERROR' || level === 'CRITICAL' || level === 'FATAL') {
      error += count
    }
  }

  const distributionTotal = sum(distribution)
  const known = info + warn + error
  const other = Math.max(0, distributionTotal - known)
  const total = Math.max(distributionTotal, known, Number(fallbackTotal) || 0)

  return { info, warn, error, other, total }
}

export function ProjectLevelStrip(props: {
  distribution?: Record<string, number>
  fallbackTotal?: number
  height?: number
  showLegend?: boolean
  detailedLegend?: boolean
  class?: string
}): JSX.Element {
  const segments = createMemo(() => getLevelSegments(props.distribution, props.fallbackTotal))
  const hasDistribution = () => {
    const value = segments()
    return value.info + value.warn + value.error + value.other > 0
  }

  const summary = () => {
    const value = segments()
    const parts = [
      `INFO ${formatNumber(value.info)}`,
      `WARN ${formatNumber(value.warn)}`,
      `ERROR ${formatNumber(value.error)}`,
    ]
    if (value.other > 0) parts.push(`其他 ${formatNumber(value.other)}`)
    return hasDistribution() ? `级别分布：${parts.join('；')}` : '暂无级别分布数据'
  }

  return (
    <div class={props.class ?? ''}>
      <div
        class="level-strip"
        style={{ height: `${props.height ?? 3}px` }}
        role="img"
        aria-label={summary()}
        title={summary()}
      >
        <Show when={segments().info > 0}>
          <i class="info" style={{ width: percentWidth(segments().info, segments().total) }} />
        </Show>
        <Show when={segments().warn > 0}>
          <i class="warn" style={{ width: percentWidth(segments().warn, segments().total) }} />
        </Show>
        <Show when={segments().error > 0}>
          <i class="error" style={{ width: percentWidth(segments().error, segments().total) }} />
        </Show>
        <Show when={segments().other > 0}>
          <i class="other" style={{ width: percentWidth(segments().other, segments().total) }} />
        </Show>
      </div>

      <Show when={props.showLegend}>
        <Show
          when={hasDistribution()}
          fallback={<div class="mono-sm faint" style={{ 'margin-top': '7px' }}>暂无级别分布数据</div>}
        >
          <Show
            when={props.detailedLegend}
            fallback={
              <div
                class="row wrap"
                style={{
                  gap: '10px',
                  'margin-top': '7px',
                  'font-family': 'var(--font-mono)',
                  'font-size': '9px',
                }}
              >
                <span class="sky">INFO {formatNumber(segments().info)}</span>
                <span class="amber">WARN {formatNumber(segments().warn)}</span>
                <span class="ember">ERROR {formatNumber(segments().error)}</span>
                <Show when={segments().other > 0}>
                  <span class="faint">OTHER {formatNumber(segments().other)}</span>
                </Show>
              </div>
            }
          >
            <div class="row wrap" style={{ gap: '10px', 'margin-top': '9px' }}>
              <span class="row" style={{ gap: '5px' }}>
                <LevelBadge level="INFO" />
                <span class="mono-sm muted">{formatNumber(segments().info)}</span>
              </span>
              <span class="row" style={{ gap: '5px' }}>
                <LevelBadge level="WARN" />
                <span class="mono-sm muted">{formatNumber(segments().warn)}</span>
              </span>
              <span class="row" style={{ gap: '5px' }}>
                <LevelBadge level="ERROR" />
                <span class="mono-sm muted">{formatNumber(segments().error)}</span>
              </span>
              <Show when={segments().other > 0}>
                <span class="row" style={{ gap: '5px' }}>
                  <span class="tag">其他</span>
                  <span class="mono-sm muted">{formatNumber(segments().other)}</span>
                </span>
              </Show>
            </div>
          </Show>
        </Show>
      </Show>
    </div>
  )
}
