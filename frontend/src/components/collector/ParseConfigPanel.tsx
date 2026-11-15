import { For, Show, createMemo, createSignal, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { navigate, notify } from '../../stores/app'
import type {
  AIProviderInfo,
  FormatDetection,
  ParseTemplate,
  PreviewResult,
} from '../../types'
import { formatPercent, formatTime, truncate } from '../../utils'
import {
  Badge,
  Button,
  Field,
  Icon,
  LevelBadge,
  ProgressBar,
  Segmented,
  Select,
  Spinner,
  TextArea,
  TextInput,
  Toggle,
} from '../ui'
import {
  categoryLabel,
  groupTemplates,
  parseModeOptionsForTab,
  ratePercent,
  rateRatio,
  splitSamples,
  type BadgeTone,
  type ParseState,
  type ParseTabKey,
} from './shared'

export interface ParseConfigPanelProps {
  tab: ParseTabKey
  parse: ParseState
  onChange: (patch: Partial<ParseState>) => void
  templates: ParseTemplate[]
  providers: AIProviderInfo[]
  previewLines: string[]
  previewSource: string
  templatesLoading?: boolean
  templatesError?: string
  onReload?: () => void
  onGenerated: (template: ParseTemplate) => void
}

/** 所有采集方式共用的解析配置、样本与模板预览面板。 */
export function ParseConfigPanel(props: ParseConfigPanelProps): JSX.Element {
  const [detecting, setDetecting] = createSignal(false)
  const [previewing, setPreviewing] = createSignal(false)
  const [generating, setGenerating] = createSignal(false)
  const [detection, setDetection] = createSignal<FormatDetection | null>(null)
  const [previewResult, setPreviewResult] = createSignal<PreviewResult | null>(null)
  const [generated, setGenerated] = createSignal<ParseTemplate | null>(null)

  const modeOptions = createMemo(() => parseModeOptionsForTab(props.tab))
  const templateGroups = createMemo(() => groupTemplates(props.templates))
  const selectedTemplate = createMemo(() =>
    props.templates.find((template) => template.id === props.parse.template_id),
  )
  const orphanTemplate = createMemo(() => {
    const id = props.parse.template_id.trim()
    if (!id) return ''
    return props.templates.some((template) => template.id === id) ? '' : id
  })
  const providerOptions = createMemo(() =>
    props.providers.map((provider) => ({
      value: provider.provider,
      label: `${provider.name || provider.provider} · ${provider.model || provider.provider}${
        provider.configured ? '' : ' · 未配置'
      }`,
    })),
  )
  const samples = createMemo(() => splitSamples(props.parse.samples))
  const sampleCount = createMemo(() => samples().length)
  const previewRate = createMemo(() => ratePercent(previewResult()?.success_rate ?? 0))
  const previewRateTone = (): BadgeTone => {
    const value = previewRate()
    if (value >= 90) return 'mint'
    if (value >= 60) return 'amber'
    return 'ember'
  }

  function changeMode(value: string) {
    const option = modeOptions().find((item) => item.value === value)
    if (option) props.onChange({ mode: option.value })
  }

  function applyPreviewSamples() {
    const lines = props.previewLines.slice(0, 500)
    if (!lines.length) return
    props.onChange({ samples: lines.join('\n') })
    notify(
      'info',
      '已填入文件预览样本',
      `${lines.length} 行${props.previewSource ? ` · ${props.previewSource}` : ''}`,
    )
  }

  function clearSamples() {
    props.onChange({ samples: '' })
  }

  async function detectFormat() {
    const list = samples().slice(0, 200)
    if (!list.length) {
      notify('warn', '缺少解析样本', '请先粘贴日志样本，或使用文件预览填入。')
      return
    }
    setDetecting(true)
    try {
      const result = await api.templates.detectFormat(list)
      setDetection(result)
      notify('success', '格式检测完成', result?.detected_format || '未知格式')
    } catch (err) {
      notify('error', '格式检测失败', errorText(err))
    } finally {
      setDetecting(false)
    }
  }

  async function previewParse() {
    const template = selectedTemplate()
    if (!template) {
      notify('warn', '未选择模板', '请选择一个现有模板，或先在 AI 模式生成模板。')
      return
    }
    const list = samples().slice(0, 200)
    if (!list.length) {
      notify('warn', '缺少解析样本', '请先粘贴日志样本，或使用文件预览填入。')
      return
    }
    setPreviewing(true)
    setPreviewResult(null)
    try {
      const result = await api.templates.preview({ template, samples: list })
      if (!result) throw new Error('后端未返回预览结果')
      const normalized: PreviewResult = {
        samples: Array.isArray(result.samples) ? result.samples : [],
        success_rate:
          typeof result.success_rate === 'number' ? result.success_rate : 0,
      }
      setPreviewResult(normalized)
      notify(
        'success',
        '解析预览完成',
        `成功率 ${formatPercent(ratePercent(normalized.success_rate))}`,
      )
    } catch (err) {
      notify('error', '解析预览失败', errorText(err))
    } finally {
      setPreviewing(false)
    }
  }

  async function generateTemplate() {
    const list = samples()
    if (!list.length) {
      notify('warn', '缺少解析样本', '请先粘贴日志样本，或使用文件预览填入。')
      return
    }
    const provider = props.parse.ai_provider.trim()
    if (!provider) {
      notify('warn', '未选择 AI 提供商', '请在 AI 解析配置中选择一个提供商。')
      return
    }
    const category = props.parse.ai_category.trim() || 'custom'
    const templateName =
      props.parse.ai_template_name.trim() || `${categoryLabel(category)}日志模板`
    const limit = Math.max(
      1,
      Math.min(500, Math.floor(Number(props.parse.sample_size) || 50)),
    )
    setGenerating(true)
    try {
      const template = await api.templates.generateAI({
        samples: list.slice(0, limit),
        provider,
        template_name: templateName,
        category,
      })
      if (!template?.id) throw new Error('后端未返回有效模板')
      setGenerated(template)
      props.onGenerated(template)
      notify('success', 'AI 模板生成成功', `${template.name || templateName} · ${template.id}`)
    } catch (err) {
      notify('error', 'AI 模板生成失败', errorText(err))
    } finally {
      setGenerating(false)
    }
  }

  function selectSuggested(id: string) {
    const match = props.templates.find(
      (template) => template.id === id || template.name === id,
    )
    props.onChange({ template_id: match?.id ?? id })
  }

  function templateDisplayName(id: string): string {
    const template = props.templates.find(
      (item) => item.id === id || item.name === id,
    )
    return template ? template.name : id
  }

  return (
    <div class="stack" style={{ gap: '12px' }}>
      <div class="row between wrap" style={{ gap: '8px' }}>
        <div class="panel-title" style={{ 'font-size': '11px' }}>
          <span class="tick" style={{ height: '10px' }} />
          解析配置
        </div>
        <Segmented options={modeOptions()} value={props.parse.mode} onChange={changeMode} />
      </div>

      <Show when={props.tab === 'evtx'}>
        <div class="detail-note">
          Windows 事件日志默认使用原生 EVTX 解析；仅当文件实际为文本日志时切换为内置模板。
        </div>
      </Show>

      <Show when={props.templatesLoading}>
        <div class="row" style={{ gap: '6px' }}>
          <Spinner />
          <span class="mono-sm faint">正在加载解析模板与 AI 提供商…</span>
        </div>
      </Show>

      <Show when={props.templatesError}>
        <div class="danger-note">
          <div class="row between wrap" style={{ gap: '8px' }}>
            <span class="mono-sm">{props.templatesError}</span>
            <Show when={props.onReload}>
              <Button size="sm" variant="ghost" onClick={() => props.onReload?.()}>
                重新加载
              </Button>
            </Show>
          </div>
        </div>
      </Show>

      <Show
        when={props.parse.mode !== 'native_evtx'}
        fallback={
          <div class="faint mono-sm">
            原生 EVTX 模式：无需模板与 AI 配置，直接解析事件结构。
          </div>
        }
      >
        <div class="stack" style={{ gap: '12px' }}>
          <Field
            label={props.parse.mode === 'ai' ? '模板（可先选择现有模板）' : '解析模板'}
            hint={`${props.templates.length} 个可用`}
          >
            <select
              class="select"
              value={props.parse.template_id}
              onInput={(event) =>
                props.onChange({ template_id: event.currentTarget.value })
              }
            >
              <option value="">自动匹配 / 内置识别</option>
              <For each={templateGroups()}>
                {(group) => (
                  <optgroup label={group.label}>
                    <For each={group.items}>
                      {(template) => (
                        <option value={template.id}>
                          {template.name}
                          {template.type === 'ai_generated' ? ' · AI' : ''}
                        </option>
                      )}
                    </For>
                  </optgroup>
                )}
              </For>
              <Show when={orphanTemplate()}>
                <option value={props.parse.template_id}>
                  {`未知模板 · ${props.parse.template_id}`}
                </option>
              </Show>
            </select>
          </Field>

          <Show when={selectedTemplate()}>
            {(template) => (
              <div class="row wrap" style={{ gap: '5px' }}>
                <Badge tone="muted">{template().type || 'template'}</Badge>
                <Badge tone="sky">{categoryLabel(template().category)}</Badge>
                <Show when={template().description}>
                  <span class="mono-sm faint truncate">{template().description}</span>
                </Show>
                <span class="mono-sm faint">· {template().id}</span>
              </div>
            )}
          </Show>

          <Show when={props.parse.mode === 'ai'}>
            <div
              class="stack"
              style={{
                gap: '10px',
                padding: '10px 12px',
                border: '1px solid var(--border)',
                background: '#f4f3ff',
              }}
            >
              <div class="row wrap" style={{ gap: '6px' }}>
                <Badge tone="violet">AI 解析</Badge>
                <span class="mono-sm faint">
                  从样本生成模板后可预览、保存并复用。
                </span>
              </div>

              <Show when={props.providers.length === 0}>
                <div class="detail-note">
                  <div class="row between wrap" style={{ gap: '8px' }}>
                    <span>尚无 AI 提供商配置，请先在系统设置中添加并启用。</span>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => navigate('settings')}
                    >
                      前往系统设置
                    </Button>
                  </div>
                </div>
              </Show>

              <Show when={props.providers.length > 0}>
                <Field label="AI 提供商">
                  <Select
                    value={props.parse.ai_provider}
                    options={providerOptions()}
                    onInput={(value) => props.onChange({ ai_provider: value })}
                  />
                </Field>
              </Show>

              <div class="form-row">
                <Field label="新模板名称" hint="生成时使用">
                  <TextInput
                    mono
                    value={props.parse.ai_template_name}
                    placeholder="如 nginx-access"
                    onInput={(value) => props.onChange({ ai_template_name: value })}
                  />
                </Field>
                <Field label="模板类别" hint="如 web_server">
                  <TextInput
                    mono
                    value={props.parse.ai_category}
                    placeholder="custom"
                    onInput={(value) => props.onChange({ ai_category: value })}
                  />
                </Field>
                <Field label="AI 样本数" hint="1-500">
                  <TextInput
                    type="number"
                    value={String(props.parse.sample_size)}
                    onInput={(value) => {
                      const parsed = Number(value)
                      props.onChange({
                        sample_size:
                          Number.isFinite(parsed) && parsed > 0
                            ? Math.min(500, Math.floor(parsed))
                            : 50,
                      })
                    }}
                  />
                </Field>
              </div>

              <div class="row" style={{ gap: '8px' }}>
                <Toggle
                  checked={props.parse.save_template}
                  label="保存 AI 模板"
                  onChange={(checked) => props.onChange({ save_template: checked })}
                />
                <span class="muted" style={{ 'font-size': '11.5px' }}>
                  AI 模式采集时自动保存生成的模板到模板库
                </span>
              </div>
            </div>
          </Show>

          <div class="divider" style={{ margin: '0' }} />

          <Field label="解析样本" hint={`${sampleCount()} 行`}>
            <TextArea
              mono
              rows={6}
              value={props.parse.samples}
              placeholder={
                '每行一条日志样本，用于格式检测、AI 生成模板与解析预览。\n可从文件预览自动填入。'
              }
              onInput={(value) => props.onChange({ samples: value })}
            />
          </Field>

          <div class="row wrap" style={{ gap: '6px' }}>
            <Show when={props.previewLines.length > 0}>
              <Button size="sm" variant="ghost" onClick={applyPreviewSamples}>
                <Icon name="file" size={11} />
                填入文件预览（{props.previewLines.length} 行）
              </Button>
            </Show>
            <Button
              size="sm"
              variant="ghost"
              disabled={!props.parse.samples}
              onClick={clearSamples}
            >
              <Icon name="x" size={11} />
              清空
            </Button>
            <Show when={props.previewSource}>
              <span class="mono-sm faint truncate" title={props.previewSource}>
                来源：{props.previewSource}
              </span>
            </Show>
          </div>

          <div class="row wrap" style={{ gap: '6px', 'align-items': 'center' }}>
            <Button
              size="sm"
              loading={detecting()}
              disabled={sampleCount() === 0}
              title="对样本进行日志格式检测"
              onClick={() => void detectFormat()}
            >
              <Show when={!detecting()}>
                <Icon name="crosshair" size={11} />
              </Show>
              检测格式
            </Button>
            <Button
              size="sm"
              loading={previewing()}
              disabled={sampleCount() === 0 || !selectedTemplate()}
              title={
                selectedTemplate()
                  ? '使用当前模板预览解析结果'
                  : '请先选择或生成一个模板'
              }
              onClick={() => void previewParse()}
            >
              <Show when={!previewing()}>
                <Icon name="eye" size={11} />
              </Show>
              解析预览
            </Button>
            <Show when={props.parse.mode === 'ai'}>
              <Button
                size="sm"
                variant="primary"
                loading={generating()}
                disabled={sampleCount() === 0 || !props.parse.ai_provider}
                onClick={() => void generateTemplate()}
              >
                <Show when={!generating()}>
                  <Icon name="wand" size={11} />
                </Show>
                AI 生成模板
              </Button>
            </Show>
            <span class="mono-sm faint">预览使用当前模板；AI 模式可先生成模板</span>
          </div>

          <Show when={generated()}>
            {(template) => (
              <div class="success-note stack" style={{ gap: '4px' }}>
                <div class="row wrap" style={{ gap: '6px' }}>
                  <Icon name="wand" size={12} />
                  <span>已生成模板</span>
                  <Badge tone="violet">{template().name || template().id}</Badge>
                  <span class="mono-sm faint">{template().id}</span>
                </div>
                <div class="mono-sm faint truncate" title={template().regex}>
                  {template().regex || '（无正则）'}
                </div>
              </div>
            )}
          </Show>

          <Show when={detection()}>
            {(result) => (
              <div class="success-note stack" style={{ gap: '6px' }}>
                <div class="row wrap" style={{ gap: '6px' }}>
                  <span class="mono-sm">检测格式</span>
                  <Badge tone="sky">{result().detected_format || 'unknown'}</Badge>
                  <span class="mono-sm faint">
                    置信度 {formatPercent(ratePercent(result().confidence))}
                  </span>
                </div>
                <Show when={(result().suggested_templates?.length ?? 0) > 0}>
                  <div class="row wrap" style={{ gap: '4px' }}>
                    <span class="mono-sm faint">建议模板</span>
                    <For each={result().suggested_templates}>
                      {(id) => (
                        <button
                          type="button"
                          class="chip"
                          title={id}
                          onClick={() => selectSuggested(id)}
                        >
                          {templateDisplayName(id)}
                        </button>
                      )}
                    </For>
                  </div>
                </Show>
              </div>
            )}
          </Show>

          <Show when={previewResult()}>
            {(result) => (
              <div class="stack" style={{ gap: '8px' }}>
                <div class="row between wrap" style={{ gap: '6px' }}>
                  <div class="row wrap" style={{ gap: '6px' }}>
                    <span class="mono-sm">解析预览</span>
                    <span class="mono-sm faint">{result().samples.length} 条</span>
                    <Badge tone={previewRateTone()}>
                      成功率 {formatPercent(previewRate())}
                    </Badge>
                  </div>
                  <span class="mono-sm faint">
                    成功 {result().samples.filter((sample) => sample.success).length} · 失败{' '}
                    {result().samples.filter((sample) => !sample.success).length}
                  </span>
                </div>
                <ProgressBar value={rateRatio(result().success_rate)} />
                <div
                  class="scrollable"
                  style={{ 'max-height': '320px', border: '1px solid var(--border)' }}
                >
                  <table class="data-table" style={{ 'min-width': '680px' }}>
                    <thead>
                      <tr>
                        <th>状态</th>
                        <th>时间</th>
                        <th>级别</th>
                        <th>消息</th>
                        <th>原始日志</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={result().samples}>
                        {(sample) => (
                          <tr>
                            <td>
                              <Badge tone={sample.success ? 'mint' : 'ember'}>
                                {sample.success ? '成功' : '失败'}
                              </Badge>
                            </td>
                            <td class="mono-sm">
                              {sample.parsed?.timestamp
                                ? formatTime(sample.parsed?.timestamp)
                                : '—'}
                            </td>
                            <td>
                              <Show
                                when={sample.parsed}
                                fallback={<span class="faint">—</span>}
                              >
                                <LevelBadge level={sample.parsed?.level} />
                              </Show>
                            </td>
                            <td class="mono-sm" style={{ 'max-width': '260px' }}>
                              <Show
                                when={sample.success}
                                fallback={
                                  <span class="ember">
                                    {sample.error || '解析失败'}
                                  </span>
                                }
                              >
                                {truncate(sample.parsed?.message || '', 160)}
                              </Show>
                            </td>
                            <td
                              class="mono-sm faint"
                              style={{ 'max-width': '220px' }}
                              title={sample.raw_log}
                            >
                              {truncate(sample.raw_log.replace(/\s+/g, ' '), 120)}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </Show>
        </div>
      </Show>
    </div>
  )
}
