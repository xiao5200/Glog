import { Match, Show, Switch, createEffect, createSignal } from 'solid-js'
import type { ForensicsRequest, ForensicsResult, Investigation } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import {
  copyText,
  formatBytes,
  formatInt,
  formatTimeShort,
  todayStamp,
} from '../../utils'
import {
  Badge,
  Button,
  Checkbox,
  CodeBlock,
  Field,
  Icon,
  PageBusy,
  Segmented,
  Select,
  StatTile,
  TextArea,
  TextInput,
} from '../ui'

export interface ForensicsPanelProps {
  projectId: string
}

type Scope = 'investigation' | 'query' | 'docs'

const SCOPES = [
  { value: 'investigation', label: '调查工单' },
  { value: 'query', label: '当前查询' },
  { value: 'docs', label: '指定 DocID' },
]

export function ForensicsPanel(props: ForensicsPanelProps) {
  const [scope, setScope] = createSignal<Scope>('investigation')

  const [investigations, setInvestigations] = createSignal<Investigation[]>([])
  const [investigationId, setInvestigationId] = createSignal('')
  const [invLoading, setInvLoading] = createSignal(false)
  const [invError, setInvError] = createSignal('')

  const [queryText, setQueryText] = createSignal('')
  const [docIdsText, setDocIdsText] = createSignal('')

  const [outputPath, setOutputPath] = createSignal(`glog-forensics-${todayStamp()}.zip`)
  const [includeEntries, setIncludeEntries] = createSignal(true)
  const [includeEvidence, setIncludeEvidence] = createSignal(true)

  const [busy, setBusy] = createSignal(false)
  const [error, setError] = createSignal('')
  const [result, setResult] = createSignal<ForensicsResult | null>(null)

  let invSeq = 0

  const loadInvestigations = async () => {
    const projectID = props.projectId
    if (!projectID) return
    const seq = ++invSeq
    setInvLoading(true)
    setInvError('')
    try {
      const list = await api.investigations.list(projectID)
      if (seq !== invSeq) return
      const safe = Array.isArray(list) ? list : []
      setInvestigations(safe)
      setInvestigationId((prev) => {
        if (prev && safe.some((item) => item.id === prev)) return prev
        return safe[0]?.id ?? ''
      })
    } catch (err) {
      if (seq !== invSeq) return
      const message = errorText(err)
      setInvError(message)
      notify('error', '加载调查工单失败', message)
    } finally {
      if (seq === invSeq) setInvLoading(false)
    }
  }

  createEffect(() => {
    const projectID = props.projectId
    if (!projectID) return
    setError('')
    setResult(null)
    setInvestigationId('')
    void loadInvestigations()
  })

  const investigationOptions = () => [
    {
      value: '',
      label: invLoading() ? '加载调查工单…' : '请选择调查工单',
    },
    ...investigations().map((item) => ({
      value: item.id,
      label: `${item.title || '未命名工单'} · ${item.status || 'unknown'}`,
    })),
  ]

  const parseDocIds = () =>
    Array.from(
      new Set(
        docIdsText()
          .split(/[\s,;，；]+/)
          .map((value) => Number(value))
          .filter((value) => Number.isInteger(value) && value > 0),
      ),
    )

  const scopeSummary = () => {
    switch (scope()) {
      case 'investigation': {
        const selected = investigations().find((item) => item.id === investigationId())
        return selected ? selected.title : '未选择工单'
      }
      case 'query':
        return queryText().trim() || '项目全量日志'
      case 'docs':
        return `${parseDocIds().length} 个 DocID`
      default:
        return ''
    }
  }

  const defaultOutputName = () => {
    const current = outputPath().trim()
    const name = current.split(/[\\/]/).pop()
    return name || `glog-forensics-${todayStamp()}.zip`
  }

  const chooseOutput = async () => {
    try {
      const path = await api.system.selectSaveFile(defaultOutputName(), '取证包', '*.zip')
      if (path) setOutputPath(path)
    } catch (err) {
      notify('error', '选择输出路径失败', errorText(err))
    }
  }

  const generate = async () => {
    if (busy()) return
    const path = outputPath().trim()
    if (!path) {
      notify('warn', '请设置输出路径', '取证包需要写入一个 ZIP 文件路径。')
      return
    }

    const request: ForensicsRequest = {
      project_id: props.projectId,
      output_path: path,
      include_entries: includeEntries(),
      include_evidence: includeEvidence(),
    }

    if (scope() === 'investigation') {
      if (!investigationId()) {
        notify('warn', '请选择调查工单', '当前取证范围为调查工单。')
        return
      }
      request.investigation_id = investigationId()
    } else if (scope() === 'query') {
      const text = queryText().trim()
      if (text) request.query = { text }
    } else if (scope() === 'docs') {
      const ids = parseDocIds()
      if (ids.length === 0) {
        notify('warn', '请填写 DocID 列表', '使用逗号、空格或换行分隔日志 DocID。')
        return
      }
      request.doc_ids = ids
    }

    setBusy(true)
    setError('')
    setResult(null)
    try {
      const generated = await api.threat.forensics(request)
      setResult(generated)
      notify('success', '取证包生成完成', generated?.path || path)
    } catch (err) {
      const message = errorText(err)
      setError(message)
      notify('error', '生成取证包失败', message)
    } finally {
      setBusy(false)
    }
  }

  const showInExplorer = async () => {
    const path = result()?.path
    if (!path) return
    try {
      await api.system.showInExplorer(path)
    } catch (err) {
      notify('error', '无法打开文件管理器', errorText(err))
    }
  }

  const copySha = async () => {
    const sha = result()?.sha256
    if (!sha) {
      notify('warn', 'SHA256 不可用', '后端未返回校验值。')
      return
    }
    try {
      await copyText(sha)
      notify('success', 'SHA256 已复制', sha)
    } catch (err) {
      notify('error', '复制 SHA256 失败', errorText(err))
    }
  }

  const copyResultPath = async () => {
    const path = result()?.path
    if (!path) return
    try {
      await copyText(path)
      notify('success', '路径已复制', path)
    } catch (err) {
      notify('error', '复制路径失败', errorText(err))
    }
  }

  return (
    <div class="stack">
      <div class="panel">
        <div class="panel-head">
          <div class="panel-title">
            <span class="tick" />
            取证包生成
          </div>
          <span class="mono-sm faint">ZIP · 含清单与 SHA256</span>
        </div>

        <div class="panel-body stack" style={{ gap: '14px' }}>
          <div>
            <div class="label" style={{ 'margin-bottom': '7px' }}>
              取证范围
            </div>
            <Segmented
              options={SCOPES}
              value={scope()}
              onChange={(value) => setScope(value as Scope)}
            />
            <div class="mono-sm faint" style={{ 'margin-top': '6px' }}>
              {
                {
                  investigation: '导出工单关联日志、证据与调查元数据。',
                  query: '按查询表达式筛选日志；留空表示项目全量日志。',
                  docs: '按 Doc ID 精确导出指定日志条目。',
                }[scope()]
              }
            </div>
          </div>

          <Switch>
            <Match when={scope() === 'investigation'}>
              <Field
                label="调查工单"
                hint={invLoading() ? '加载中…' : `${investigations().length} 个可用工单`}
              >
                <div class="row" style={{ 'align-items': 'stretch' }}>
                  <div class="grow">
                    <Select
                      value={investigationId()}
                      options={investigationOptions()}
                      disabled={invLoading() || Boolean(invError())}
                      onInput={setInvestigationId}
                    />
                  </div>
                  <Button
                    variant="ghost"
                    disabled={invLoading()}
                    onClick={() => void loadInvestigations()}
                  >
                    <Icon name="refresh" size={12} />
                    刷新
                  </Button>
                </div>
              </Field>
              <Show when={invError()}>
                <div class="danger-note">加载调查工单失败：{invError()}</div>
              </Show>
              <Show when={!invLoading() && !invError() && investigations().length === 0}>
                <div class="detail-note">
                  当前项目暂无调查工单，可切换至“当前查询”或“指定 DocID”范围。
                </div>
              </Show>
            </Match>

            <Match when={scope() === 'query'}>
              <Field label="查询表达式" hint="留空表示项目全量日志">
                <TextInput
                  value={queryText()}
                  mono
                  placeholder={'例如：level:ERROR AND host:web-01'}
                  onInput={setQueryText}
                />
              </Field>
              <div class="detail-note">
                页面未持有日志页的临时查询状态，请在此填写需要导出的查询条件。
              </div>
            </Match>

            <Match when={scope() === 'docs'}>
              <Field label="DocID 列表" hint={`已解析 ${parseDocIds().length} 个`}>
                <TextArea
                  value={docIdsText()}
                  rows={3}
                  mono
                  placeholder={'101, 205, 309\n或使用空格 / 换行分隔'}
                  onInput={setDocIdsText}
                />
              </Field>
            </Match>

          </Switch>

          <Field label="输出路径" hint="ZIP 取证包">
            <div class="row" style={{ 'align-items': 'stretch' }}>
              <div class="grow">
                <TextInput value={outputPath()} mono onInput={setOutputPath} />
              </div>
              <Button onClick={() => void chooseOutput()}>
                <Icon name="save" size={12} />
                选择…
              </Button>
            </div>
          </Field>

          <div class="row wrap" style={{ gap: '18px' }}>
            <Checkbox
              checked={includeEntries()}
              label="包含日志条目"
              onChange={setIncludeEntries}
            />
            <Checkbox
              checked={includeEvidence()}
              label="包含证据文件"
              onChange={setIncludeEvidence}
            />
          </div>

          <div class="detail-note">
            取证包将包含命中日志、证据清单、生成时间与 SHA256 校验值。包含日志时文件体积可能较大。
          </div>

          <div class="row between wrap" style={{ gap: '10px' }}>
            <span class="mono-sm faint">范围：{scopeSummary()}</span>
            <Button variant="primary" loading={busy()} onClick={() => void generate()}>
              <Icon name="package" size={12} />
              生成取证包
            </Button>
          </div>

          <Show when={busy()}>
            <PageBusy visible text="正在收集日志、证据并生成 ZIP 取证包…" />
          </Show>

          <Show when={error()}>
            <div class="danger-note">生成取证包失败：{error()}</div>
          </Show>
        </div>
      </div>

      <Show when={result()}>
        {(generated) => (
          <div class="panel">
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                取证包结果
                <Badge tone="mint">已生成</Badge>
              </div>
              <span class="mono-sm faint">
                生成于 {formatTimeShort(generated().generated_at)}
              </span>
            </div>

            <div class="panel-body stack">
              <div class="stat-grid">
                <StatTile
                  label="文件数"
                  value={generated().files?.length ?? 0}
                  note="ZIP 内文件条目"
                />
                <StatTile
                  label="日志条目"
                  value={formatInt(generated().entry_count)}
                  note="纳入包内的日志"
                  tone="sky"
                />
                <StatTile
                  label="包大小"
                  value={formatBytes(generated().bytes)}
                  note="压缩后体积"
                />
              </div>

              <dl class="kv">
                <dt>输出路径</dt>
                <dd class="mono-sm" style={{ 'word-break': 'break-all' }}>
                  {generated().path || '—'}
                </dd>
                <dt>SHA256</dt>
                <dd class="mono-sm" style={{ 'word-break': 'break-all' }}>
                  {generated().sha256 || '—'}
                </dd>
              </dl>

              <div class="row wrap" style={{ gap: '8px' }}>
                <Button
                  size="sm"
                  disabled={!generated().path}
                  onClick={() => void showInExplorer()}
                >
                  <Icon name="folder" size={12} />
                  在文件管理器中显示
                </Button>
                <Button size="sm" disabled={!generated().sha256} onClick={() => void copySha()}>
                  <Icon name="copy" size={12} />
                  复制 SHA256
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!generated().path}
                  onClick={() => void copyResultPath()}
                >
                  <Icon name="copy" size={12} />
                  复制路径
                </Button>
              </div>

              <Show when={(generated().files ?? []).length > 0}>
                <div>
                  <div class="section-label">包内文件</div>
                  <CodeBlock
                    text={(generated().files ?? []).join('\n')}
                    maxHeight="180px"
                  />
                </div>
              </Show>
            </div>
          </div>
        )}
      </Show>
    </div>
  )
}
