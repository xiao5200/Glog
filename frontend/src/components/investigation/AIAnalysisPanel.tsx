import { For, Show } from 'solid-js'
import type { AIAnalysisResult, TimelineEvent } from '../../types'
import { formatTime, severityTone } from '../../utils'
import { Badge, Icon } from '../ui'

export interface AIAnalysisPanelProps {
  analysis: AIAnalysisResult
  onOpenLog?: (docID: number) => void
}

/** 解析研判事件中的日志 Doc ID（十进制字符串），忽略非数字与非法值。 */
function parseDocIds(event: TimelineEvent): number[] {
  const ids: number[] = []
  for (const raw of event.log_entry_ids ?? []) {
    const text = String(raw ?? '').trim()
    if (!/^\d+$/.test(text)) continue
    const id = Number(text)
    if (!Number.isSafeInteger(id) || id < 0) continue
    if (!ids.includes(id)) ids.push(id)
  }
  return ids
}

export function AIAnalysisPanel(props: AIAnalysisPanelProps) {
  const findings = () => props.analysis.findings ?? []
  const suggestions = () => props.analysis.suggestions ?? []
  const phases = () => props.analysis.attack_phases ?? []
  const timeline = () => props.analysis.timeline ?? []

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          AI 研判分析
        </div>
        <div class="row wrap">
          <Badge tone={severityTone(props.analysis.risk_level)}>
            风险 {props.analysis.risk_level || 'unknown'}
          </Badge>
          <span class="mono-sm faint">{formatTime(props.analysis.generated_at)}</span>
        </div>
      </div>

      <div class="panel-body stack" style={{ gap: '14px' }}>
        <Show
          when={findings().length > 0}
          fallback={<div class="faint" style={{ 'font-size': '11px' }}>模型未返回关键发现。</div>}
        >
          <div>
            <div class="section-label">关键发现</div>
            <ul style={{ margin: 0, padding: '0 0 0 16px', display: 'grid', gap: '5px' }}>
              <For each={findings()}>
                {(item) => (
                  <li style={{ 'font-size': '11.5px', color: 'var(--text-muted)', 'line-height': '1.6' }}>
                    {item}
                  </li>
                )}
              </For>
            </ul>
          </div>
        </Show>

        <Show when={suggestions().length > 0}>
          <div>
            <div class="section-label">处置建议</div>
            <ul style={{ margin: 0, padding: '0 0 0 16px', display: 'grid', gap: '5px' }}>
              <For each={suggestions()}>
                {(item) => (
                  <li style={{ 'font-size': '11.5px', color: 'var(--text-muted)', 'line-height': '1.6' }}>
                    {item}
                  </li>
                )}
              </For>
            </ul>
          </div>
        </Show>

        <Show when={phases().length > 0}>
          <div>
            <div class="section-label">攻击阶段</div>
            <div style={{ display: 'grid', gap: '8px' }}>
              <For each={phases()}>
                {(phase) => (
                  <div
                    style={{
                      border: '1px solid var(--border)',
                      'border-left': '2px solid var(--accent-dim)',
                      background: 'var(--surface-soft)',
                      padding: '9px 11px',
                    }}
                  >
                    <div class="row between wrap">
                      <span
                        style={{
                          'font-family': 'var(--font-display)',
                          'font-weight': 600,
                          'letter-spacing': '0.05em',
                          'font-size': '12px',
                        }}
                      >
                        {phase.phase || '未命名阶段'}
                      </span>
                      <Show when={phase.evidence?.length}>
                        <span class="mono-sm faint">证据 {phase.evidence.length}</span>
                      </Show>
                    </div>
                    <div
                      style={{
                        'margin-top': '4px',
                        'font-size': '11.5px',
                        color: 'var(--text-muted)',
                        'line-height': '1.65',
                      }}
                    >
                      {phase.description || '暂无阶段描述'}
                    </div>
                    <Show when={phase.evidence?.length}>
                      <div class="row wrap" style={{ 'margin-top': '7px', gap: '4px' }}>
                        <For each={phase.evidence ?? []}>
                          {(evidence) => (
                            <span class="tag" title={evidence}>
                              <Icon name="hash" size={9} />
                              {evidence.length > 48 ? `${evidence.slice(0, 48)}…` : evidence}
                            </span>
                          )}
                        </For>
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>

        <Show when={timeline().length > 0}>
          <div>
            <div class="section-label">研判时间线</div>
            <div style={{ display: 'grid', gap: '7px' }}>
              <For each={timeline()}>
                {(event) => (
                  <div
                    style={{
                      display: 'grid',
                      'grid-template-columns': '118px minmax(0, 1fr)',
                      gap: '9px',
                      'border-left': '1px solid var(--border-strong)',
                      'padding-left': '10px',
                    }}
                  >
                    <span class="mono-sm faint" style={{ 'white-space': 'nowrap' }}>
                      {formatTime(event.timestamp, false)}
                    </span>
                    <div>
                      <div class="row wrap" style={{ gap: '6px' }}>
                        <Badge tone={severityTone(event.severity)}>{event.severity || 'info'}</Badge>
                        <Show when={event.source}>
                          <span class="mono-sm faint">{event.source}</span>
                        </Show>
                      </div>
                      <div style={{ 'font-size': '11.5px', 'line-height': '1.55', 'margin-top': '2px' }}>
                        {event.event}
                      </div>
                      <Show when={event.log_entry_ids?.length}>
                        <div class="row wrap" style={{ 'margin-top': '4px', gap: '4px' }}>
                          <span class="mono-sm faint">
                            {event.log_entry_ids.length} 条关联日志
                          </span>
                          <Show when={props.onOpenLog}>
                            {(open) => (
                              <For each={parseDocIds(event)}>
                                {(docId) => (
                                  <button
                                    type="button"
                                    class="chip"
                                    style={{ padding: '1px 7px', 'font-size': '10px' }}
                                    title={`查看 Doc #${docId} 日志详情`}
                                    onClick={() => open()(docId)}
                                  >
                                    <Icon name="hash" size={9} />
                                    Doc #{docId}
                                  </button>
                                )}
                              </For>
                            )}
                          </Show>
                        </div>
                      </Show>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </div>
    </div>
  )
}
