import { For, Show } from 'solid-js'
import type { LogEntry } from '../../types'
import { errorText } from '../../api'
import { notify } from '../../stores/app'
import { copyText, fieldLabel, formatTime, friendlyLogMessage, safeJson } from '../../utils'
import {
  Badge,
  Button,
  CodeBlock,
  Drawer,
  EmptyState,
  Icon,
  LevelBadge,
  PageBusy,
} from '../ui'

export interface LogEntryDrawerProps {
  open: boolean
  entry?: LogEntry | null
  loading?: boolean
  error?: string
  note?: string
  title?: string
  onClose: () => void
  onRetry?: () => void
}

export function LogEntryDrawer(props: LogEntryDrawerProps) {
  const copyRaw = async (entry: LogEntry) => {
    try {
      await copyText(entry.raw_log ?? '')
      notify('success', '已复制原始日志', `Doc #${entry.doc_id}`)
    } catch (err) {
      notify('error', '复制失败', errorText(err))
    }
  }

  return (
    <Drawer
      open={props.open}
      wide
      title={props.title ?? '日志详情'}
      subtitle={
        props.entry
          ? `${formatTime(props.entry.timestamp)} · ${props.entry.source_host || props.entry.source || '未知来源'}`
          : '正在读取日志上下文'
      }
      onClose={props.onClose}
      footer={
        <>
          <Button variant="ghost" onClick={props.onClose}>
            关闭
          </Button>
          <Show when={props.entry}>
            {(entry) => (
              <Button onClick={() => void copyRaw(entry())}>
                <Icon name="copy" size={12} />
                复制原始日志
              </Button>
            )}
          </Show>
        </>
      }
    >
      <Show when={props.note}>
        <div class="detail-note" style={{ 'margin-bottom': '12px' }}>
          {props.note}
        </div>
      </Show>

      <Show when={props.loading}>
        <PageBusy visible text="加载日志详情…" />
      </Show>

      <Show when={props.error}>
        <div class="stack">
          <div class="danger-note">{props.error}</div>
          <Show when={props.onRetry}>
            <div>
              <Button size="sm" onClick={() => props.onRetry?.()}>
                <Icon name="refresh" size={12} />
                重试
              </Button>
            </div>
          </Show>
        </div>
      </Show>

      <Show when={!props.loading && !props.error && !props.entry}>
        <EmptyState icon="file" title="暂无日志详情" text="未能读取该条日志，请稍后重试。" />
      </Show>

      <Show when={props.entry}>
        {(entry) => (
          <div class="stack">
            <div class="row wrap">
              <LevelBadge level={entry().level} />
              <Show when={entry().risk_score != null}>
                <Badge tone="amber">风险分 {entry().risk_score}</Badge>
              </Show>
              <Show when={entry().is_anomaly}>
                <Badge tone="ember">异常</Badge>
              </Show>
              <Show when={entry().is_evidence}>
                <Badge tone="violet">证据</Badge>
              </Show>
              <Show when={entry().tags?.length}>
                <For each={entry().tags ?? []}>
                  {(tag) => <span class="tag">#{tag}</span>}
                </For>
              </Show>
            </div>

            <dl class="kv">
              <dt>Doc ID</dt>
              <dd>{entry().doc_id}</dd>
              <dt>日志 ID</dt>
              <dd style={{ 'word-break': 'break-all' }}>{entry().id || '—'}</dd>
              <dt>时间</dt>
              <dd>{formatTime(entry().timestamp)}</dd>
              <dt>级别</dt>
              <dd>{entry().level || 'UNKNOWN'}</dd>
              <dt>来源</dt>
              <dd>{entry().source || '—'}</dd>
              <dt>主机</dt>
              <dd>{entry().source_host || entry().computer || '—'}</dd>
              <dt>文件</dt>
              <dd style={{ 'word-break': 'break-all' }}>{entry().source_file || '—'}</dd>
              <dt>文件偏移</dt>
              <dd>{entry().source_offset ?? '—'}</dd>
              <Show when={entry().event_id != null}>
                <dt>事件 ID</dt>
                <dd>{entry().event_id}</dd>
              </Show>
              <Show when={entry().provider}>
                <dt>提供程序</dt>
                <dd style={{ 'word-break': 'break-all' }}>{entry().provider}</dd>
              </Show>
              <Show when={entry().computer}>
                <dt>计算机</dt>
                <dd>{entry().computer}</dd>
              </Show>
              <Show when={entry().user_id}>
                <dt>用户</dt>
                <dd>{entry().user_id}</dd>
              </Show>
            </dl>

            <div>
              <div class="section-label">消息</div>
              <div style={{ 'font-size': '12px', 'line-height': '1.7', color: 'var(--text)' }}>
                {friendlyLogMessage(entry().message) || '—'}
              </div>
            </div>

            <Show when={entry().fields && Object.keys(entry().fields ?? {}).length > 0}>
              <div>
                <div class="section-label">解析字段</div>
                <div class="scrollable" style={{ 'max-height': '240px', overflow: 'auto' }}>
                  <table class="data-table">
                    <thead>
                      <tr>
                        <th style={{ width: '36%' }}>字段</th>
                        <th>值</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={Object.entries(entry().fields ?? {})}>
                        {([key, value]) => (
                          <tr>
                            <td style={{ 'word-break': 'break-all' }}>
                              <div style={{ color: 'var(--text)' }}>{fieldLabel(key)}</div>
                              <Show when={fieldLabel(key) !== key}>
                                <div class="mono-sm faint" style={{ 'font-size': '9.5px' }}>
                                  {key}
                                </div>
                              </Show>
                            </td>
                            <td class="mono-sm" style={{ 'word-break': 'break-all', 'white-space': 'pre-wrap' }}>
                              {typeof value === 'object' ? safeJson(value, 4000) : String(value)}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </div>
            </Show>

            <div>
              <div class="section-label">原始日志</div>
              <CodeBlock text={entry().raw_log || '（无原始日志）'} maxHeight="380px" />
            </div>
          </div>
        )}
      </Show>
    </Drawer>
  )
}
