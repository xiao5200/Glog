import { For, Show, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { DetectionRule } from '../../types'
import { formatInt, formatTime, severityTone, truncate } from '../../utils'
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  Modal,
  Toggle,
} from '../ui'
import { CopyableValue, SettingsPanel } from './shared'

function mitreList(rule: DetectionRule): string[] {
  return [...(rule.mitre_tactics ?? []), ...(rule.mitre_techniques ?? [])]
}

export function RuleSettings() {
  const [rules, setRules] = createSignal<DetectionRule[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [toggling, setToggling] = createSignal<string | null>(null)
  const [detail, setDetail] = createSignal<DetectionRule | null>(null)

  async function loadRules(reportError = true): Promise<void> {
    setLoading(true)
    try {
      const list = await api.threat.rules()
      setRules(Array.isArray(list) ? list : [])
      setLoadError('')
    } catch (err) {
      setLoadError(errorText(err))
      if (reportError) notify('error', '加载检测规则失败', errorText(err))
    } finally {
      setLoading(false)
    }
  }

  onMount(() => {
    void loadRules(true)
  })

  async function toggleRule(rule: DetectionRule, enabled: boolean): Promise<void> {
    if (toggling()) return
    setToggling(rule.id)
    try {
      if (enabled) await api.threat.enableRule(rule.id)
      else await api.threat.disableRule(rule.id)
      setRules((prev) =>
        prev.map((item) =>
          item.id === rule.id ? { ...item, enabled } : item,
        ),
      )
      notify(
        'success',
        enabled ? '规则已启用' : '规则已停用',
        rule.name,
      )
    } catch (err) {
      notify('error', '更新规则状态失败', errorText(err))
    } finally {
      setToggling(null)
    }
  }

  return (
    <div class="stack">
      <SettingsPanel
        title="检测规则"
        note="规则以 YAML 描述，由本地检测引擎执行。启用状态可通过后端持久化；规则内容不在线编辑。"
        actions={
          <Button
            size="sm"
            variant="ghost"
            loading={loading()}
            onClick={() => void loadRules(true)}
          >
            <Icon name="refresh" size={12} />
            刷新规则
          </Button>
        }
      >
        <Show when={loading()}>
          <div class="loading-line" />
        </Show>
        <Show when={loadError()}>
          <div class="danger-note">
            无法从后端读取检测规则：{loadError()}。请确认 Wails 后端已启动后重试。
          </div>
        </Show>

        <Show
          when={rules().length > 0}
          fallback={
            <EmptyState
              icon="shield-check"
              title="暂无检测规则"
              text="内置规则位于 configs/rules/builtin，自定义规则可放入数据目录的 rules 目录后重新加载。"
              action={
                <Button variant="ghost" onClick={() => void loadRules(true)}>
                  <Icon name="refresh" size={12} />
                  重新加载
                </Button>
              }
            />
          }
        >
          <div style={{ 'overflow-x': 'auto' }}>
            <table class="data-table">
              <thead>
                <tr>
                  <th>规则</th>
                  <th>严重性</th>
                  <th>启用</th>
                  <th>标签</th>
                  <th>MITRE ATT&amp;CK</th>
                  <th>命中</th>
                  <th>最近命中</th>
                  <th>来源 / 文件</th>
                  <th style={{ width: '80px' }}>操作</th>
                </tr>
              </thead>
              <tbody>
                <For each={rules()}>
                  {(rule) => (
                    <tr>
                      <td>
                        <div style={{ 'font-weight': 600 }}>{rule.name}</div>
                        <div
                          class="mono-sm faint truncate"
                          title={rule.id}
                          style={{ 'max-width': '240px' }}
                        >
                          {rule.id}
                        </div>
                      </td>
                      <td>
                        <Badge tone={severityTone(rule.severity)}>
                          {String(rule.severity || 'unknown').toUpperCase()}
                        </Badge>
                      </td>
                      <td>
                        <Toggle
                          checked={rule.enabled}
                          label={`启用规则 ${rule.name}`}
                          disabled={toggling() !== null}
                          onChange={(checked) => void toggleRule(rule, checked)}
                        />
                      </td>
                      <td>
                        <Show
                          when={(rule.tags?.length ?? 0) > 0}
                          fallback={<span class="faint">—</span>}
                        >
                          <div class="row wrap" style={{ gap: '3px' }}>
                            <For each={(rule.tags ?? []).slice(0, 3)}>
                              {(tag) => <span class="tag">{tag}</span>}
                            </For>
                            <Show when={(rule.tags?.length ?? 0) > 3}>
                              <span class="tag">
                                +{(rule.tags?.length ?? 0) - 3}
                              </span>
                            </Show>
                          </div>
                        </Show>
                      </td>
                      <td>
                        <Show
                          when={mitreList(rule).length > 0}
                          fallback={<span class="faint">—</span>}
                        >
                          <div class="row wrap" style={{ gap: '3px' }}>
                            <For each={mitreList(rule).slice(0, 2)}>
                              {(item) => <span class="tag mono-sm">{item}</span>}
                            </For>
                            <Show when={mitreList(rule).length > 2}>
                              <span class="tag">
                                +{mitreList(rule).length - 2}
                              </span>
                            </Show>
                          </div>
                        </Show>
                      </td>
                      <td class="numeric mono-sm">
                        {formatInt(rule.match_count)}
                      </td>
                      <td class="mono-sm faint">
                        {formatTime(rule.last_matched_at)}
                      </td>
                      <td>
                        <Badge tone={rule.builtin ? 'muted' : 'sky'}>
                          {rule.builtin ? '内置' : '自定义'}
                        </Badge>
                        <div
                          class="mono-sm faint truncate"
                          title={rule.file_path}
                          style={{ 'max-width': '220px', 'margin-top': '3px' }}
                        >
                          {rule.file_path ? truncate(rule.file_path, 40) : '—'}
                        </div>
                      </td>
                      <td>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setDetail(rule)}
                        >
                          详情
                        </Button>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>

        <div class="detail-note">
          规则 YAML 位于内置资源或数据目录（data/rules）。GLog 不提供在线规则编辑；修改 YAML 后请重新加载应用，或点击刷新以同步启用状态与命中统计。
        </div>
      </SettingsPanel>

      <Modal
        open={Boolean(detail())}
        title="检测规则详情"
        subtitle={detail()?.id}
        onClose={() => setDetail(null)}
        footer={
          <Button variant="ghost" onClick={() => setDetail(null)}>
            关闭
          </Button>
        }
      >
        <Show when={detail()}>
          {(rule) => (
            <div class="form-stack">
              <div class="row wrap between">
                <div>
                  <div
                    style={{
                      'font-family': 'var(--font-display)',
                      'font-size': '17px',
                      'font-weight': 600,
                    }}
                  >
                    {rule().name}
                  </div>
                  <div class="mono-sm faint">{rule().id}</div>
                </div>
                <div class="row wrap">
                  <Badge tone={severityTone(rule().severity)}>
                    {String(rule().severity || 'unknown').toUpperCase()}
                  </Badge>
                  <Badge tone={rule().builtin ? 'muted' : 'sky'}>
                    {rule().builtin ? '内置规则' : '自定义规则'}
                  </Badge>
                  <Badge tone={rule().enabled ? 'mint' : 'muted'}>
                    {rule().enabled ? '已启用' : '已停用'}
                  </Badge>
                </div>
              </div>

              <p class="muted" style={{ 'font-size': '12px', 'line-height': '1.7' }}>
                {rule().description || '该规则暂无描述。'}
              </p>

              <dl class="kv">
                <dt>命中次数</dt>
                <dd>{formatInt(rule().match_count)}</dd>
                <dt>最近命中</dt>
                <dd>{formatTime(rule().last_matched_at)}</dd>
                <dt>标签</dt>
                <dd>
                  <Show
                    when={(rule().tags?.length ?? 0) > 0}
                    fallback={<span class="faint">—</span>}
                  >
                    <div class="row wrap" style={{ gap: '4px' }}>
                      <For each={rule().tags ?? []}>
                        {(tag) => <span class="tag">{tag}</span>}
                      </For>
                    </div>
                  </Show>
                </dd>
                <dt>MITRE 战术</dt>
                <dd>
                  {(rule().mitre_tactics ?? []).join(' / ') || '—'}
                </dd>
                <dt>MITRE 技术</dt>
                <dd>
                  {(rule().mitre_techniques ?? []).join(' / ') || '—'}
                </dd>
                <dt>规则文件</dt>
                <dd>
                  <CopyableValue
                    value={rule().file_path ?? ''}
                    label="规则文件路径"
                  />
                </dd>
              </dl>

              <div class="detail-note">
                检测在本地完成，不会上传日志。规则定义与检测结果仍可能存在误报，请结合原始日志人工复核。
              </div>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  )
}
