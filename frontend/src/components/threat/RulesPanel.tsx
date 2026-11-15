import { For, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { DetectionResult, DetectionRule } from '../../types'
import { api, errorText } from '../../api'
import { notify, systemInfo } from '../../stores/app'
import { formatTimeShort, severityTone } from '../../utils'
import {
  Badge,
  Button,
  CodeBlock,
  EmptyState,
  Icon,
  IconButton,
  Modal,
  PageBusy,
  SectionLabel,
  Toggle,
} from '../ui'

export interface RulesPanelProps {
  projectId: string
  onResult: (result: DetectionResult) => void
}

function mitreItems(rule: DetectionRule): string[] {
  return Array.from(
    new Set([...(rule.mitre_tactics ?? []), ...(rule.mitre_techniques ?? [])].filter(Boolean)),
  )
}

export function RulesPanel(props: RulesPanelProps) {
  const [rules, setRules] = createSignal<DetectionRule[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')

  const [selected, setSelected] = createSignal<Set<string>>(new Set())
  const [toggleBusy, setToggleBusy] = createSignal<Set<string>>(new Set())

  const [running, setRunning] = createSignal(false)
  const [runError, setRunError] = createSignal('')

  const [detailRule, setDetailRule] = createSignal<DetectionRule | null>(null)

  let loadSeq = 0

  const loadRules = async () => {
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.threat.rules()
      if (seq !== loadSeq) return
      const safe = Array.isArray(list) ? list : []
      setRules(safe)
      setSelected((prev) => {
        const available = new Set(safe.map((rule) => rule.id))
        return new Set(Array.from(prev).filter((id) => available.has(id)))
      })
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载检测规则失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const projectID = props.projectId
    if (!projectID) return
    setSelected(new Set<string>())
    setRunError('')
    setDetailRule(null)
    void loadRules()
  })

  const allSelected = createMemo(() => {
    const list = rules()
    return list.length > 0 && list.every((rule) => selected().has(rule.id))
  })

  const toggleAll = () => {
    if (allSelected()) {
      setSelected(new Set<string>())
      return
    }
    setSelected(new Set(rules().map((rule) => rule.id)))
  }

  const toggleSelected = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const setRuleToggleBusy = (id: string, busy: boolean) => {
    setToggleBusy((prev) => {
      const next = new Set(prev)
      if (busy) next.add(id)
      else next.delete(id)
      return next
    })
  }

  const toggleRuleEnabled = async (rule: DetectionRule, enabled: boolean) => {
    if (toggleBusy().has(rule.id)) return
    setRuleToggleBusy(rule.id, true)
    try {
      if (enabled) await api.threat.enableRule(rule.id)
      else await api.threat.disableRule(rule.id)
      setRules((prev) =>
        prev.map((item) => (item.id === rule.id ? { ...item, enabled } : item)),
      )
      notify('success', enabled ? '规则已启用' : '规则已禁用', rule.name)
    } catch (err) {
      notify('error', '更新规则状态失败', errorText(err))
    } finally {
      setRuleToggleBusy(rule.id, false)
    }
  }

  const runSelected = async () => {
    if (running()) return
    const ids = Array.from(selected())
    if (ids.length === 0) {
      notify('warn', '请选择检测规则', '至少勾选一条规则后才能运行检测。')
      return
    }
    setRunning(true)
    setRunError('')
    try {
      const result = await api.threat.runDetection({
        project_id: props.projectId,
        rule_ids: ids,
      })
      notify(
        'success',
        '检测完成',
        `命中 ${result?.total_matches ?? 0} 条，扫描 ${result?.scanned ?? 0} 条日志`,
      )
      props.onResult(result)
    } catch (err) {
      const message = errorText(err)
      setRunError(message)
      notify('error', '检测运行失败', message)
    } finally {
      setRunning(false)
    }
  }

  const tagPreview = (rule: DetectionRule) => {
    const tags = rule.tags ?? []
    return { list: tags.slice(0, 3), rest: Math.max(0, tags.length - 3) }
  }

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          检测规则
          <Badge tone="muted">{rules().length}</Badge>
        </div>
        <div class="row wrap" style={{ gap: '6px' }}>
          <span class="mono-sm faint">已选 {selected().size}</span>
          <Button size="sm" variant="ghost" onClick={toggleAll} disabled={rules().length === 0}>
            {allSelected() ? '取消全选' : '全选'}
          </Button>
          <IconButton
            icon="refresh"
            title="刷新规则"
            disabled={loading() || running()}
            onClick={() => void loadRules()}
          />
          <Button
            variant="primary"
            size="sm"
            loading={running()}
            disabled={selected().size === 0}
            onClick={() => void runSelected()}
          >
            <Icon name="play" size={11} />
            运行检测
          </Button>
        </div>
      </div>

      <Show when={running()}>
        <div class="panel-body" style={{ 'padding-top': '8px', 'padding-bottom': '2px' }}>
          <PageBusy visible text="检测规则运行中，正在扫描项目日志…" />
        </div>
      </Show>

      <Show when={runError()}>
        <div class="panel-body" style={{ 'padding-top': '4px', 'padding-bottom': '6px' }}>
          <div class="danger-note">检测运行失败：{runError()}</div>
        </div>
      </Show>

      <Show when={loadError()}>
        <div class="panel-body">
          <div class="danger-note">
            <div class="row between wrap">
              <span>{loadError()}</span>
              <Button size="sm" variant="ghost" onClick={() => void loadRules()}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={loading()}>
        <div class="panel-body">
          <PageBusy visible text="加载检测规则…" />
        </div>
      </Show>

      <Show when={!loading() && !loadError()}>
        <Show
          when={rules().length > 0}
          fallback={
            <EmptyState
              icon="shield"
              title="暂无检测规则"
              text="内置规则应位于 configs/rules/builtin；也可将规则文件放入数据目录的 rules 子目录后刷新。"
              action={
                <Button size="sm" onClick={() => void loadRules()}>
                  <Icon name="refresh" size={12} />
                  重新加载
                </Button>
              }
            />
          }
        >
          <div class="scrollable" style={{ 'max-height': 'calc(100vh - 330px)', 'min-height': '300px' }}>
            <table class="data-table">
              <thead>
                <tr>
                  <th style={{ width: '34px' }}>
                    <input
                      type="checkbox"
                      class="checkbox"
                      checked={allSelected()}
                      aria-label="全选检测规则"
                      onChange={toggleAll}
                    />
                  </th>
                  <th>规则名称</th>
                  <th style={{ width: '88px' }}>严重级别</th>
                  <th style={{ width: '64px' }}>启用</th>
                  <th style={{ width: '170px' }}>标签</th>
                  <th style={{ width: '210px' }}>MITRE ATT&CK</th>
                  <th style={{ width: '78px' }}>匹配次数</th>
                  <th style={{ width: '126px' }}>最近匹配</th>
                  <th style={{ width: '42px' }} />
                </tr>
              </thead>
              <tbody>
                <For each={rules()}>
                  {(rule) => (
                    <tr>
                      <td>
                        <input
                          type="checkbox"
                          class="checkbox"
                          checked={selected().has(rule.id)}
                          aria-label={`选择规则 ${rule.name}`}
                          onChange={() => toggleSelected(rule.id)}
                        />
                      </td>
                      <td>
                        <div class="row" style={{ gap: '6px' }}>
                          <button
                            type="button"
                            title="查看规则详情"
                            style={{
                              background: 'transparent',
                              border: 'none',
                              padding: 0,
                              color: 'var(--text)',
                              'text-align': 'left',
                              'font-family': 'var(--font-display)',
                              'font-weight': 600,
                              'font-size': '12px',
                              cursor: 'pointer',
                            }}
                            onClick={() => setDetailRule(rule)}
                          >
                            {rule.name || rule.id}
                          </button>
                          <Show when={rule.builtin}>
                            <Badge tone="violet">BUILTIN</Badge>
                          </Show>
                        </div>
                        <div class="mono-sm faint" style={{ 'margin-top': '2px' }}>
                          {rule.id}
                        </div>
                      </td>
                      <td>
                        <Badge tone={severityTone(rule.severity)}>
                          {rule.severity || 'unknown'}
                        </Badge>
                      </td>
                      <td>
                        <Toggle
                          checked={rule.enabled}
                          label={`${rule.enabled ? '禁用' : '启用'}规则 ${rule.name}`}
                          disabled={toggleBusy().has(rule.id)}
                          onChange={(checked) => void toggleRuleEnabled(rule, checked)}
                        />
                      </td>
                      <td>
                        <div class="row wrap" style={{ gap: '3px' }}>
                          <For each={tagPreview(rule).list}>
                            {(tag) => <span class="tag">{tag}</span>}
                          </For>
                          <Show when={tagPreview(rule).rest > 0}>
                            <span class="tag">+{tagPreview(rule).rest}</span>
                          </Show>
                          <Show when={(rule.tags ?? []).length === 0}>
                            <span class="faint">—</span>
                          </Show>
                        </div>
                      </td>
                      <td class="mono-sm" title={mitreItems(rule).join(' · ')}>
                        {mitreItems(rule).length > 0 ? (
                          <span class="truncate" style={{ display: 'block', 'max-width': '200px' }}>
                            {mitreItems(rule).join(' · ')}
                          </span>
                        ) : (
                          <span class="faint">—</span>
                        )}
                      </td>
                      <td class="numeric">{rule.match_count ?? 0}</td>
                      <td class="mono-sm" style={{ 'white-space': 'nowrap' }}>
                        {rule.last_matched_at ? formatTimeShort(rule.last_matched_at) : '—'}
                      </td>
                      <td>
                        <IconButton
                          icon="eye"
                          title="规则详情"
                          onClick={() => setDetailRule(rule)}
                        />
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </Show>

      <Modal
        open={Boolean(detailRule())}
        title="检测规则详情"
        subtitle={detailRule()?.id}
        size="lg"
        onClose={() => setDetailRule(null)}
        footer={
          <Button variant="ghost" onClick={() => setDetailRule(null)}>
            关闭
          </Button>
        }
      >
        <Show when={detailRule()}>
          {(rule) => (
            <div class="stack">
              <div class="row wrap" style={{ gap: '6px' }}>
                <Badge tone={severityTone(rule().severity)}>
                  {rule().severity || 'unknown'}
                </Badge>
                <Badge tone={rule().enabled ? 'mint' : 'muted'}>
                  {rule().enabled ? '已启用' : '已禁用'}
                </Badge>
                <Show when={rule().builtin}>
                  <Badge tone="violet">BUILTIN</Badge>
                </Show>
                <span class="mono-sm faint">匹配 {rule().match_count ?? 0} 次</span>
                <Show when={rule().last_matched_at}>
                  <span class="mono-sm faint">
                    最近 {formatTimeShort(rule().last_matched_at)}
                  </span>
                </Show>
              </div>

              <div>
                <div
                  style={{
                    'font-family': 'var(--font-display)',
                    'font-size': '18px',
                    'font-weight': 600,
                    'letter-spacing': '0.02em',
                  }}
                >
                  {rule().name || rule().id}
                </div>
                <p
                  class="muted"
                  style={{ 'font-size': '12px', 'line-height': '1.7', 'margin-top': '6px' }}
                >
                  {rule().description || '暂无规则描述。'}
                </p>
              </div>

              <div>
                <SectionLabel>MITRE ATT&CK</SectionLabel>
                <Show
                  when={mitreItems(rule()).length > 0}
                  fallback={<span class="faint" style={{ 'font-size': '11px' }}>未标注战术或技术。</span>}
                >
                  <div class="row wrap" style={{ gap: '4px' }}>
                    <For each={rule().mitre_tactics ?? []}>
                      {(item) => <span class="tag">TACTIC · {item}</span>}
                    </For>
                    <For each={rule().mitre_techniques ?? []}>
                      {(item) => <span class="tag">TECH · {item}</span>}
                    </For>
                  </div>
                </Show>
              </div>

              <div>
                <SectionLabel>标签</SectionLabel>
                <Show
                  when={(rule().tags ?? []).length > 0}
                  fallback={<span class="faint" style={{ 'font-size': '11px' }}>无标签。</span>}
                >
                  <div class="row wrap" style={{ gap: '4px' }}>
                    <For each={rule().tags ?? []}>
                      {(tag) => <span class="tag">{tag}</span>}
                    </For>
                  </div>
                </Show>
              </div>

              <div>
                <SectionLabel>规则文件</SectionLabel>
                <CodeBlock text={rule().file_path || '（规则未提供文件路径）'} maxHeight="90px" />
              </div>

              <div class="detail-note">
                内置规则位于应用目录 configs/rules/builtin；自定义规则位于数据目录{' '}
                {systemInfo()?.data_path ?? '（数据目录）'} 的 rules 子目录。修改规则文件后重新加载规则生效。
              </div>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  )
}
