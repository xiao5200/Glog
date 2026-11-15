import { For, Show, createMemo } from 'solid-js'
import type { DetectionMatch, DetectionResult, LogEntry } from '../../types'
import {
  formatDuration,
  formatInt,
  formatTimeShort,
  riskTone,
  severityTone,
  truncate,
} from '../../utils'
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  LevelBadge,
  StatTile,
} from '../ui'

export interface DetectionResultsPanelProps {
  result: DetectionResult | null
  onOpenLog: (docId: number, entry?: LogEntry, note?: string) => void
  onGoRules: () => void
}

interface EnrichedMatch extends DetectionMatch {
  rule_name: string
}

function severityColor(severity: string | undefined): string {
  const tone = severityTone(severity)
  if (tone === 'ember') return 'var(--ember)'
  if (tone === 'amber') return 'var(--amber)'
  if (tone === 'sky') return 'var(--sky)'
  return 'var(--accent)'
}

export function DetectionResultsPanel(props: DetectionResultsPanelProps) {
  const allMatches = createMemo<EnrichedMatch[]>(() => {
    const result = props.result
    if (!result) return []
    return (result.results ?? []).flatMap((rule) =>
      (rule.matches ?? []).map((match) => ({
        ...match,
        rule_name: match.rule_name || rule.rule_name || rule.rule_id,
      })),
    )
  })

  const ruleResults = createMemo(() => props.result?.results ?? [])

  return (
    <Show
      when={props.result}
      fallback={
        <div class="panel">
          <div class="panel-body">
            <EmptyState
              icon="crosshair"
              title="暂无检测结果"
              text="在“检测规则”中选择规则并运行检测，命中结果将在这里分组展示。"
              action={
                <Button variant="primary" size="sm" onClick={props.onGoRules}>
                  <Icon name="shield" size={12} />
                  前往检测规则
                </Button>
              }
            />
          </div>
        </div>
      }
    >
      {(result) => (
        <div class="stack">
          <div class="stat-grid">
            <StatTile
              label="总命中"
              value={formatInt(result().total_matches)}
              note={`${ruleResults().length} 条规则参与本次检测`}
              tone={result().total_matches > 0 ? 'ember' : 'mint'}
            />
            <StatTile
              label="扫描日志"
              value={formatInt(result().scanned)}
              note={`任务 ${result().task_id || '—'}`}
            />
            <StatTile
              label="耗时"
              value={formatDuration(result().took)}
              note={`完成于 ${formatTimeShort(result().finished_at)}`}
              tone="sky"
            />
          </div>

          <div class="panel">
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                按规则汇总
              </div>
              <span class="mono-sm faint">
                {formatTimeShort(result().started_at)} → {formatTimeShort(result().finished_at)}
              </span>
            </div>
            <div class="panel-body">
              <Show
                when={ruleResults().length > 0}
                fallback={<div class="faint" style={{ 'font-size': '11px' }}>本次检测未返回规则摘要。</div>}
              >
                <div
                  style={{
                    display: 'grid',
                    'grid-template-columns': 'repeat(auto-fill, minmax(250px, 1fr))',
                    gap: '10px',
                  }}
                >
                  <For each={ruleResults()}>
                    {(rule) => (
                      <div
                        style={{
                          border: '1px solid var(--border)',
                          'border-left': `2px solid ${severityColor(rule.severity)}`,
                          background: 'var(--surface-soft)',
                          padding: '10px 12px',
                        }}
                      >
                        <div class="row between" style={{ gap: '8px' }}>
                          <span
                            class="truncate"
                            style={{
                              'font-family': 'var(--font-display)',
                              'font-weight': 600,
                              'font-size': '12px',
                            }}
                            title={rule.rule_name}
                          >
                            {rule.rule_name || rule.rule_id}
                          </span>
                          <Badge tone={severityTone(rule.severity)}>
                            {rule.severity || 'unknown'}
                          </Badge>
                        </div>
                        <div class="row" style={{ 'margin-top': '7px', 'align-items': 'baseline' }}>
                          <span
                            style={{
                              'font-family': 'var(--font-display)',
                              'font-size': '24px',
                              'font-weight': 600,
                              'line-height': 1,
                              color: rule.count > 0 ? 'var(--text)' : 'var(--text-faint)',
                            }}
                          >
                            {formatInt(rule.count)}
                          </span>
                          <span class="mono-sm faint">命中</span>
                          <Show when={rule.summaries?.length}>
                            <span class="mono-sm faint">· {rule.summaries?.length} 个分组</span>
                          </Show>
                        </div>
                        <Show when={rule.error}>
                          <div class="danger-note" style={{ 'margin-top': '8px' }}>
                            {rule.error}
                          </div>
                        </Show>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          </div>

          <div class="panel">
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                命中日志
                <Badge tone={allMatches().length > 0 ? 'ember' : 'muted'}>
                  {allMatches().length}
                </Badge>
              </div>
              <span class="mono-sm faint">点击命中行查看日志详情</span>
            </div>

            <Show
              when={allMatches().length > 0}
              fallback={
                <EmptyState
                  icon="shield-check"
                  title="没有命中日志"
                  text={
                    result().total_matches > 0
                      ? '检测结果未携带命中明细，请重新运行检测。'
                      : `已扫描 ${formatInt(result().scanned)} 条日志，所有规则均未命中。`
                  }
                  action={
                    <Button size="sm" onClick={props.onGoRules}>
                      <Icon name="play" size={11} />
                      重新选择规则
                    </Button>
                  }
                />
              }
            >
              <div class="scrollable" style={{ 'max-height': '520px' }}>
                <table class="data-table">
                  <thead>
                    <tr>
                      <th style={{ width: '136px' }}>时间</th>
                      <th style={{ width: '190px' }}>规则</th>
                      <th style={{ width: '80px' }}>级别</th>
                      <th style={{ width: '112px' }}>主机</th>
                      <th>消息</th>
                      <th style={{ width: '130px' }}>Group Key</th>
                      <th style={{ width: '80px' }}>风险</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={allMatches()}>
                      {(match) => (
                        <tr
                          style={{ cursor: 'pointer' }}
                          title="打开日志详情"
                          onClick={() =>
                            props.onOpenLog(
                              match.doc_id,
                              undefined,
                              `规则 ${match.rule_name} · ${match.group_key || '无分组'}`,
                            )
                          }
                        >
                          <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                            {formatTimeShort(match.timestamp)}
                          </td>
                          <td class="truncate" title={match.rule_name}>
                            {match.rule_name}
                          </td>
                          <td>
                            <LevelBadge level={match.level} />
                          </td>
                          <td class="mono-sm truncate" title={match.source_host}>
                            {match.source_host || '—'}
                          </td>
                          <td title={match.message}>{truncate(match.message, 160)}</td>
                          <td class="mono-sm truncate" title={match.group_key}>
                            {match.group_key || '—'}
                          </td>
                          <td>
                            <Badge tone={riskTone(match.risk_score)}>
                              {match.risk_score ?? 0}
                            </Badge>
                          </td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>
            </Show>
          </div>
        </div>
      )}
    </Show>
  )
}
