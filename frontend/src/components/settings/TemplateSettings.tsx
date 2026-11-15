import { For, Show, createMemo, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { ParseTemplate, PreviewResult } from '../../types'
import { formatInt, formatPercent, formatTime, truncate } from '../../utils'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  Icon,
  LevelBadge,
  Modal,
  Select,
  TextArea,
  TextInput,
} from '../ui'
import {
  SettingsPanel,
  parseNumberMap,
  parseStringMap,
  splitLines,
  toInt,
  withCurrentOption,
} from './shared'

const FORMAT_OPTIONS = [
  { value: 'regex', label: 'regex（正则）' },
  { value: 'json', label: 'json（结构化日志）' },
  { value: 'syslog', label: 'syslog（RFC 3164/5424）' },
  { value: 'auto', label: 'auto（自动检测）' },
]

const MULTILINE_MATCH_OPTIONS = [
  { value: 'after', label: 'after（归属后续行）' },
  { value: 'before', label: 'before（归属前序行）' },
]

const AI_PROVIDER_LABELS: Record<string, string> = {
  openai: 'OpenAI',
  deepseek: 'DeepSeek',
  ollama: 'Ollama（本地）',
  custom: '自定义',
}

function aiProviderLabel(provider: string): string {
  return AI_PROVIDER_LABELS[provider] ?? provider
}

interface TemplateDraft {
  id: string
  name: string
  description: string
  type: string
  source: string
  category: string
  format: string
  priority: number
  regex: string
  fieldMapText: string
  timeFormat: string
  timeField: string
  levelField: string
  levelMapText: string
  levelExtraction?: ParseTemplate['level_extraction']
  multilineEnabled: boolean
  multilinePattern: string
  multilineNegate: boolean
  multilineMatch: string
  multilineMaxLines: number
  multilineTimeoutMs: number
  examplesText: string
  createdAt?: string
  updatedAt?: string
  usageCount?: number
}

type TemplateBuildResult =
  | { ok: true; template: ParseTemplate }
  | { ok: false; errors: Record<string, string> }

function draftFromTemplate(template: ParseTemplate): TemplateDraft {
  return {
    id: template.id ?? '',
    name: template.name ?? '',
    description: template.description ?? '',
    type: template.type || 'custom',
    source: template.source || 'user',
    category: template.category || 'custom',
    format: template.format || 'regex',
    priority: Number(template.priority ?? 0),
    regex: template.regex ?? '',
    fieldMapText: JSON.stringify(template.field_map ?? {}, null, 2),
    timeFormat: template.time_format ?? '',
    timeField: template.time_field ?? '',
    levelField: template.level_field ?? '',
    levelMapText: JSON.stringify(template.level_map ?? {}, null, 2),
    levelExtraction: template.level_extraction,
    multilineEnabled: Boolean(template.multiline?.enabled),
    multilinePattern: template.multiline?.pattern ?? '',
    multilineNegate: Boolean(template.multiline?.negate),
    multilineMatch: template.multiline?.match || 'after',
    multilineMaxLines: Number(template.multiline?.max_lines ?? 500),
    multilineTimeoutMs: Number(template.multiline?.timeout_ms ?? 5000),
    examplesText: (template.examples ?? []).join('\n'),
    createdAt: template.created_at,
    updatedAt: template.updated_at,
    usageCount: template.usage_count,
  }
}

function newDraft(): TemplateDraft {
  return {
    id: '',
    name: '',
    description: '',
    type: 'custom',
    source: 'user',
    category: 'custom',
    format: 'regex',
    priority: 0,
    regex: '',
    fieldMapText: '{\n  "timestamp": 1,\n  "level": 2,\n  "message": 3\n}',
    timeFormat: '',
    timeField: 'timestamp',
    levelField: 'level',
    levelMapText: '{\n  "ERROR": "ERROR",\n  "WARN": "WARN",\n  "INFO": "INFO"\n}',
    multilineEnabled: false,
    multilinePattern: '',
    multilineNegate: false,
    multilineMatch: 'after',
    multilineMaxLines: 500,
    multilineTimeoutMs: 5000,
    examplesText: '',
  }
}

function buildTemplate(draft: TemplateDraft): TemplateBuildResult {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = '请填写模板名称'
  if (!draft.category.trim()) errors.category = '请填写模板分类'
  if (
    !Number.isInteger(draft.priority) ||
    draft.priority < 0 ||
    draft.priority > 10_000
  ) {
    errors.priority = '优先级需为 0–10000 的整数'
  }
  if (draft.format === 'regex' && !draft.regex.trim()) {
    errors.regex = 'regex 格式必须填写正则表达式'
  }
  const fieldMap = parseNumberMap(draft.fieldMapText, 'field_map')
  if (!fieldMap.ok) errors.field_map = fieldMap.error
  const levelMap = parseStringMap(draft.levelMapText, 'level_map')
  if (!levelMap.ok) errors.level_map = levelMap.error
  if (draft.multilineEnabled) {
    if (!draft.multilinePattern.trim()) {
      errors.multiline_pattern = '启用多行合并时需填写 pattern'
    }
    if (
      !Number.isInteger(draft.multilineMaxLines) ||
      draft.multilineMaxLines < 1 ||
      draft.multilineMaxLines > 100_000
    ) {
      errors.multiline_max_lines = '最大行数需为 1–100000 的整数'
    }
    if (
      !Number.isInteger(draft.multilineTimeoutMs) ||
      draft.multilineTimeoutMs < 1 ||
      draft.multilineTimeoutMs > 600_000
    ) {
      errors.multiline_timeout = '超时需为 1–600000 毫秒'
    }
  }
  if (Object.keys(errors).length > 0) return { ok: false, errors }

  const template: ParseTemplate = {
    id: draft.id,
    name: draft.name.trim(),
    description: draft.description.trim(),
    type: draft.type || 'custom',
    category: draft.category.trim(),
    format: draft.format || 'regex',
    priority: draft.priority,
    regex: draft.regex,
    field_map: fieldMap.ok ? fieldMap.value : {},
    time_format: draft.timeFormat.trim(),
    time_field: draft.timeField.trim(),
    level_field: draft.levelField.trim(),
    level_map: levelMap.ok ? levelMap.value : {},
    level_extraction: draft.levelExtraction,
    multiline: {
      enabled: draft.multilineEnabled,
      pattern: draft.multilinePattern,
      negate: draft.multilineNegate,
      match: draft.multilineMatch || 'after',
      max_lines: draft.multilineMaxLines,
      timeout_ms: draft.multilineTimeoutMs,
    },
    examples: splitLines(draft.examplesText),
    created_at: draft.createdAt,
    updated_at: draft.updatedAt,
    usage_count: draft.usageCount,
    source: draft.source || 'user',
  }
  return { ok: true, template }
}

function isBuiltinTemplate(template: ParseTemplate): boolean {
  return (
    String(template.type ?? '').toLowerCase() === 'builtin' ||
    String(template.source ?? '').toLowerCase() === 'builtin'
  )
}

function templateTypeLabel(template: ParseTemplate): string {
  const type = String(template.type ?? template.source ?? 'custom').toLowerCase()
  if (type === 'builtin') return '内置'
  if (type === 'ai' || type === 'ai_generated') return 'AI 生成'
  if (type === 'custom') return '自定义'
  return template.type || '未知'
}

function templateTypeTone(
  template: ParseTemplate,
): 'muted' | 'violet' | 'mint' | 'sky' {
  const type = String(template.type ?? '').toLowerCase()
  if (isBuiltinTemplate(template)) return 'muted'
  if (type === 'ai' || type === 'ai_generated') return 'violet'
  if (type === 'custom') return 'mint'
  return 'sky'
}

export function TemplateSettings() {
  const [templates, setTemplates] = createSignal<ParseTemplate[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [search, setSearch] = createSignal('')
  const [category, setCategory] = createSignal('all')

  const [editor, setEditor] = createSignal<TemplateDraft | null>(null)
  const [readonly, setReadonly] = createSignal(false)
  const [editorErrors, setEditorErrors] = createSignal<Record<string, string>>({})
  const [saving, setSaving] = createSignal(false)

  const [previewSamples, setPreviewSamples] = createSignal('')
  const [previewResult, setPreviewResult] = createSignal<PreviewResult | null>(null)
  const [previewing, setPreviewing] = createSignal(false)
  const [previewError, setPreviewError] = createSignal('')

  const [deleting, setDeleting] = createSignal<ParseTemplate | null>(null)
  const [deleteLoading, setDeleteLoading] = createSignal(false)

  const [aiOpen, setAiOpen] = createSignal(false)
  const [aiSamples, setAiSamples] = createSignal('')
  const [aiProvider, setAiProvider] = createSignal('ollama')
  const [aiName, setAiName] = createSignal('')
  const [aiCategory, setAiCategory] = createSignal('custom')
  const [aiProviders, setAiProviders] = createSignal<string[]>([
    'ollama',
    'openai',
    'deepseek',
    'custom',
  ])
  const [aiErrors, setAiErrors] = createSignal<Record<string, string>>({})
  const [aiGenerating, setAiGenerating] = createSignal(false)

  async function loadTemplates(reportError = true): Promise<void> {
    setLoading(true)
    try {
      const list = await api.templates.list({ type: 'all' })
      setTemplates(Array.isArray(list) ? list : [])
      setLoadError('')
    } catch (err) {
      setLoadError(errorText(err))
      if (reportError) notify('error', '加载解析模板失败', errorText(err))
    } finally {
      setLoading(false)
    }
  }

  async function loadAiProviders(): Promise<void> {
    try {
      const list = await api.ai.providers()
      const names = (Array.isArray(list) ? list : [])
        .map((item) => item.provider)
        .filter((value): value is string => typeof value === 'string' && value.length > 0)
      const merged = Array.from(
        new Set(['ollama', 'openai', 'deepseek', 'custom', ...names]),
      )
      setAiProviders(merged)
      if (!merged.includes(aiProvider())) setAiProvider(merged[0] ?? 'ollama')
    } catch {
      /* 提供商列表不可用时保留内置默认项，不阻塞模板管理 */
    }
  }

  onMount(() => {
    void loadTemplates(true)
    void loadAiProviders()
  })

  const categories = createMemo(() => {
    const names = new Set<string>()
    for (const item of templates()) {
      if (item.category) names.add(item.category)
    }
    return Array.from(names).sort((a, b) => a.localeCompare(b, 'zh-CN'))
  })

  const categoryOptions = createMemo(() => [
    { value: 'all', label: `全部分类（${templates().length}）` },
    ...categories().map((name) => ({ value: name, label: name })),
  ])

  const filtered = createMemo(() => {
    const keyword = search().trim().toLowerCase()
    const selected = category()
    return templates().filter((item) => {
      if (selected !== 'all' && item.category !== selected) return false
      if (!keyword) return true
      return [item.name, item.id, item.description, item.category].some((value) =>
        String(value ?? '').toLowerCase().includes(keyword),
      )
    })
  })

  function openTemplate(template: ParseTemplate, readOnly: boolean): void {
    setEditor(draftFromTemplate(template))
    setReadonly(readOnly)
    setEditorErrors({})
    setPreviewSamples((template.examples ?? []).join('\n'))
    setPreviewResult(null)
    setPreviewError('')
  }

  function openNewTemplate(): void {
    setEditor(newDraft())
    setReadonly(false)
    setEditorErrors({})
    setPreviewSamples('')
    setPreviewResult(null)
    setPreviewError('')
  }

  function closeEditor(): void {
    if (saving()) return
    setEditor(null)
    setEditorErrors({})
    setPreviewResult(null)
    setPreviewError('')
  }

  function patchDraft(patch: Partial<TemplateDraft>): void {
    setEditor((prev) => (prev ? { ...prev, ...patch } : prev))
  }

  function saveAsCustom(): void {
    const state = editor()
    if (!state) return
    setEditor({
      ...state,
      id: '',
      type: 'custom',
      source: 'user',
      name: `${state.name || '未命名模板'}（自定义）`,
    })
    setReadonly(false)
    setEditorErrors({})
    setPreviewResult(null)
    notify('info', '已创建自定义副本', '确认字段后点击“保存模板”写入模板库。')
  }

  async function saveDraft(): Promise<void> {
    const state = editor()
    if (!state || readonly()) return
    if (String(state.type).toLowerCase() === 'builtin') {
      notify('warn', '内置模板不可覆盖', '请使用“另存为自定义”创建副本。')
      return
    }
    const built = buildTemplate(state)
    if (!built.ok) {
      setEditorErrors(built.errors)
      notify(
        'warn',
        '模板校验失败',
        Object.values(built.errors)[0] ?? '请检查表单字段。',
      )
      return
    }
    setEditorErrors({})
    setSaving(true)
    try {
      await api.templates.save(built.template)
      notify(
        'success',
        state.id ? '模板已更新' : '模板已保存',
        built.template.name,
      )
      setEditor(null)
      setPreviewResult(null)
      setPreviewError('')
      await loadTemplates(false)
    } catch (err) {
      notify('error', '保存模板失败', errorText(err))
    } finally {
      setSaving(false)
    }
  }

  async function previewTemplate(
    template: ParseTemplate,
    sampleText: string,
  ): Promise<void> {
    const samples = splitLines(sampleText)
    if (samples.length === 0) {
      notify('warn', '缺少解析样本', '请粘贴至少一行日志样本。')
      return
    }
    setPreviewing(true)
    setPreviewError('')
    try {
      const result = await api.templates.preview({ template, samples })
      if (!result || !Array.isArray(result.samples)) {
        setPreviewResult(null)
        setPreviewError('后端未返回有效的预览结果。')
      } else {
        setPreviewResult(result)
      }
    } catch (err) {
      setPreviewResult(null)
      setPreviewError(errorText(err))
      notify('error', '解析预览失败', errorText(err))
    } finally {
      setPreviewing(false)
    }
  }

  function runPreview(): void {
    const state = editor()
    if (!state) return
    const built = buildTemplate(state)
    if (!built.ok) {
      setEditorErrors(built.errors)
      notify(
        'warn',
        '模板配置不合法',
        Object.values(built.errors)[0] ?? '请检查表单字段。',
      )
      return
    }
    void previewTemplate(built.template, previewSamples())
  }

  function previewRateText(): string {
    const result = previewResult()
    if (!result || result.samples.length === 0) return '—'
    const success = result.samples.filter((sample) => sample.success).length
    return `${success}/${result.samples.length} · ${formatPercent(
      (success / result.samples.length) * 100,
      1,
    )}`
  }

  function previewRateTone(): 'mint' | 'amber' | 'ember' {
    const result = previewResult()
    if (!result || result.samples.length === 0) return 'mint'
    const success = result.samples.filter((sample) => sample.success).length
    const rate = success / result.samples.length
    if (rate >= 0.9) return 'mint'
    if (rate >= 0.5) return 'amber'
    return 'ember'
  }

  function openAiModal(): void {
    setAiSamples('')
    setAiName('')
    setAiCategory('custom')
    setAiErrors({})
    setAiOpen(true)
    void loadAiProviders()
  }

  function closeAiModal(): void {
    if (aiGenerating()) return
    setAiOpen(false)
    setAiErrors({})
  }

  function openGeneratedTemplate(
    template: ParseTemplate,
    samplesText: string,
  ): void {
    const draft = draftFromTemplate(template)
    if (!template.type) draft.type = 'ai_generated'
    if (!template.source) draft.source = 'ai'
    setEditor(draft)
    setReadonly(false)
    setEditorErrors({})
    setPreviewSamples(samplesText)
    setPreviewResult(null)
    setPreviewError('')
    const built = buildTemplate(draft)
    if (built.ok) void previewTemplate(built.template, samplesText)
  }

  async function generateWithAi(): Promise<void> {
    const samples = splitLines(aiSamples())
    const nextErrors: Record<string, string> = {}
    if (samples.length === 0) nextErrors.samples = '请粘贴至少一行日志样本'
    if (!aiName().trim()) nextErrors.template_name = '请填写模板名称'
    if (!aiCategory().trim()) nextErrors.category = '请填写分类'
    if (!aiProvider()) nextErrors.provider = '请选择 AI 提供商'
    setAiErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      notify('warn', '请完善 AI 生成参数', '样本、提供商、名称与分类均为必填。')
      return
    }

    setAiGenerating(true)
    try {
      const generated = await api.templates.generateAI({
        samples,
        provider: aiProvider(),
        template_name: aiName().trim(),
        category: aiCategory().trim(),
      })
      if (!generated) throw new Error('后端未返回模板内容')
      notify('success', 'AI 模板已生成', '请检查字段映射并预览解析效果。')
      setAiOpen(false)
      openGeneratedTemplate(generated, samples.join('\n'))
    } catch (err) {
      notify('error', 'AI 生成模板失败', errorText(err))
    } finally {
      setAiGenerating(false)
    }
  }

  async function confirmDelete(): Promise<void> {
    const target = deleting()
    if (!target) return
    if (isBuiltinTemplate(target)) {
      notify('warn', '内置模板不可删除', '请另存为自定义模板后再删除副本。')
      setDeleting(null)
      return
    }
    setDeleteLoading(true)
    try {
      await api.templates.remove(target.id)
      setTemplates((prev) => prev.filter((item) => item.id !== target.id))
      setDeleting(null)
      notify('success', '模板已删除', target.name)
      await loadTemplates(false)
    } catch (err) {
      notify('error', '删除模板失败', errorText(err))
    } finally {
      setDeleteLoading(false)
    }
  }

  const editorTitle = (): string => {
    const state = editor()
    if (!state) return '解析模板'
    if (readonly()) return `查看模板 · ${state.name || '未命名'}`
    if (!state.id) return '新建自定义模板'
    return `编辑模板 · ${state.name || '未命名'}`
  }

  const editorSubtitle = (): string | undefined => {
    const state = editor()
    if (!state) return undefined
    if (readonly()) return '内置模板只读 · 可另存为自定义副本'
    return state.id ? `ID: ${state.id}` : '尚未保存 · 可在保存前预览解析结果'
  }

  return (
    <div class="stack">
      <SettingsPanel
        title="解析模板"
        note="内置模板随应用分发，只读；自定义与 AI 模板保存在数据目录。字段映射为“字段名 → 捕获组索引”，索引从 1 开始。"
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={loading()}
              onClick={() => void loadTemplates(true)}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
            <Button size="sm" variant="ghost" onClick={openAiModal}>
              <Icon name="wand" size={12} />
              AI 生成
            </Button>
            <Button size="sm" onClick={openNewTemplate}>
              <Icon name="plus" size={12} />
              新建模板
            </Button>
          </>
        }
      >
        <div class="row wrap between">
          <div class="row wrap grow">
            <div class="search-box" style={{ 'max-width': '360px', 'min-width': '240px' }}>
              <Icon name="search" size={13} />
              <input
                value={search()}
                placeholder="搜索名称 / ID / 描述 / 分类"
                aria-label="搜索解析模板"
                onInput={(e) => setSearch(e.currentTarget.value)}
              />
            </div>
            <div style={{ width: '210px' }}>
              <Select
                value={category()}
                options={categoryOptions()}
                onInput={(value) => setCategory(value)}
              />
            </div>
          </div>
          <Badge tone="muted">
            {filtered().length} / {templates().length} 条
          </Badge>
        </div>

        <Show when={loading()}>
          <div class="loading-line" />
        </Show>
        <Show when={loadError()}>
          <div class="danger-note">
            无法从后端读取模板列表：{loadError()}。模板管理暂时不可用。
          </div>
        </Show>

        <Show
          when={templates().length > 0}
          fallback={
            <EmptyState
              icon="book"
              title="模板库为空"
              text="可新建自定义模板，或基于日志样本使用 AI 生成解析模板。"
              action={
                <div class="row wrap">
                  <Button variant="primary" onClick={openNewTemplate}>
                    <Icon name="plus" size={12} />
                    新建模板
                  </Button>
                  <Button variant="ghost" onClick={openAiModal}>
                    <Icon name="wand" size={12} />
                    AI 生成
                  </Button>
                </div>
              }
            />
          }
        >
          <Show
            when={filtered().length > 0}
            fallback={
              <EmptyState
                icon="search"
                title="没有匹配的模板"
                text="尝试更换关键词或分类筛选。"
                action={
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setSearch('')
                      setCategory('all')
                    }}
                  >
                    清除筛选
                  </Button>
                }
              />
            }
          >
            <div style={{ 'overflow-x': 'auto' }}>
              <table class="data-table">
                <thead>
                  <tr>
                    <th>名称 / ID</th>
                    <th>分类</th>
                    <th>格式</th>
                    <th>优先级</th>
                    <th>类型</th>
                    <th>使用次数</th>
                    <th style={{ width: '150px' }}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={filtered()}>
                    {(template) => (
                      <tr>
                        <td>
                          <div style={{ 'font-weight': 600 }}>
                            {template.name || '未命名模板'}
                          </div>
                          <div
                            class="mono-sm faint truncate"
                            title={template.id}
                            style={{ 'max-width': '280px' }}
                          >
                            {template.id || '尚未生成 ID'}
                          </div>
                        </td>
                        <td>
                          <Badge tone="muted">
                            {template.category || '未分类'}
                          </Badge>
                        </td>
                        <td class="mono-sm">
                          {(template.format || 'regex').toUpperCase()}
                        </td>
                        <td class="numeric mono-sm">
                          {template.priority ?? 0}
                        </td>
                        <td>
                          <Badge tone={templateTypeTone(template)}>
                            {templateTypeLabel(template)}
                          </Badge>
                        </td>
                        <td class="numeric mono-sm">
                          {formatInt(template.usage_count ?? 0)}
                        </td>
                        <td>
                          <div class="row" style={{ gap: '4px' }}>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() =>
                                openTemplate(template, isBuiltinTemplate(template))
                              }
                            >
                              {isBuiltinTemplate(template) ? '查看' : '编辑'}
                            </Button>
                            <Button
                              size="sm"
                              variant="danger"
                              disabled={isBuiltinTemplate(template)}
                              title={
                                isBuiltinTemplate(template)
                                  ? '内置模板不可删除'
                                  : '删除模板'
                              }
                              onClick={() => setDeleting(template)}
                            >
                              删除
                            </Button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </Show>

        <div class="detail-note">
          内置模板不可直接覆盖；在查看弹窗中点击“另存为自定义”会创建独立副本。AI 生成模板使用所选提供商，样本会离开本机，请先脱敏。
        </div>
      </SettingsPanel>

      <Modal
        open={Boolean(editor())}
        title={editorTitle()}
        subtitle={editorSubtitle()}
        size="lg"
        onClose={closeEditor}
        footer={
          <>
            <Show when={readonly()}>
              <Button variant="primary" onClick={saveAsCustom}>
                <Icon name="copy" size={12} />
                另存为自定义
              </Button>
            </Show>
            <Show when={!readonly()}>
              <Button
                variant="primary"
                loading={saving()}
                onClick={() => void saveDraft()}
              >
                <Icon name="save" size={12} />
                保存模板
              </Button>
            </Show>
            <Button variant="ghost" disabled={saving()} onClick={closeEditor}>
              关闭
            </Button>
          </>
        }
      >
        <Show when={editor()}>
          {(state) => (
            <div class="form-stack">
              <Show when={readonly()}>
                <div class="detail-note">
                  内置模板为只读资源。需要调整时请点击“另存为自定义”，保存后创建独立模板，不会覆盖内置 YAML。
                </div>
              </Show>

              <div class="form-row">
                <Field
                  label="名称"
                  hint="必填"
                  error={editorErrors().name}
                >
                  <TextInput
                    value={state().name}
                    disabled={readonly()}
                    placeholder="Nginx access.log"
                    onInput={(v) => patchDraft({ name: v })}
                  />
                </Field>
                <Field
                  label="分类"
                  hint="category"
                  error={editorErrors().category}
                >
                  <TextInput
                    value={state().category}
                    disabled={readonly()}
                    placeholder="web_server"
                    onInput={(v) => patchDraft({ category: v })}
                  />
                </Field>
                <Field label="格式" hint="format">
                  <Select
                    value={state().format}
                    options={withCurrentOption(FORMAT_OPTIONS, state().format)}
                    disabled={readonly()}
                    onInput={(v) => patchDraft({ format: v })}
                  />
                </Field>
                <Field
                  label="优先级"
                  hint="priority · 越大越优先"
                  error={editorErrors().priority}
                >
                  <TextInput
                    type="number"
                    value={String(state().priority)}
                    disabled={readonly()}
                    onInput={(v) => patchDraft({ priority: toInt(v) })}
                  />
                </Field>
              </div>

              <Field label="描述" hint="description">
                <TextInput
                  value={state().description}
                  disabled={readonly()}
                  placeholder="用于解析 Nginx 访问日志的正则模板"
                  onInput={(v) => patchDraft({ description: v })}
                />
              </Field>

              <div class="form-row">
                <Field label="时间格式" hint="time_format · Go layout">
                  <TextInput
                    mono
                    value={state().timeFormat}
                    disabled={readonly()}
                    placeholder="2006-01-02 15:04:05"
                    onInput={(v) => patchDraft({ timeFormat: v })}
                  />
                </Field>
                <Field label="时间字段" hint="time_field">
                  <TextInput
                    mono
                    value={state().timeField}
                    disabled={readonly()}
                    placeholder="timestamp"
                    onInput={(v) => patchDraft({ timeField: v })}
                  />
                </Field>
                <Field label="级别字段" hint="level_field">
                  <TextInput
                    mono
                    value={state().levelField}
                    disabled={readonly()}
                    placeholder="level"
                    onInput={(v) => patchDraft({ levelField: v })}
                  />
                </Field>
              </div>

              <Field
                label="正则表达式"
                hint="regex · Go RE2 语法"
                error={editorErrors().regex}
              >
                <TextArea
                  mono
                  rows={4}
                  value={state().regex}
                  disabled={readonly()}
                  placeholder={'^(?P<timestamp>\\S+ \\S+) \\[(?P<level>\\w+)\\] (?P<message>.*)$'}
                  onInput={(v) => patchDraft({ regex: v })}
                />
              </Field>

              <div class="form-row">
                <Field
                  label="字段映射"
                  hint="field_map · JSON，字段名 → 捕获组索引"
                  error={editorErrors().field_map}
                >
                  <TextArea
                    mono
                    rows={6}
                    value={state().fieldMapText}
                    disabled={readonly()}
                    placeholder={'{\n  "timestamp": 1,\n  "level": 2,\n  "message": 3\n}'}
                    onInput={(v) => patchDraft({ fieldMapText: v })}
                  />
                </Field>
                <Field
                  label="级别映射"
                  hint="level_map · JSON，原始值 → 标准级别"
                  error={editorErrors().level_map}
                >
                  <TextArea
                    mono
                    rows={6}
                    value={state().levelMapText}
                    disabled={readonly()}
                    placeholder={'{\n  "ERR": "ERROR",\n  "WARNING": "WARN"\n}'}
                    onInput={(v) => patchDraft({ levelMapText: v })}
                  />
                </Field>
              </div>

              <div class="divider" />
              <div class="section-label">多行合并</div>

              <div class="form-row">
                <div class="field">
                  <span class="label">
                    <span>启用</span>
                    <span class="hint">multiline.enabled</span>
                  </span>
                  <div class="row" style={{ 'min-height': '32px' }}>
                    <Checkbox
                      checked={state().multilineEnabled}
                      disabled={readonly()}
                      label="启用多行日志合并"
                      onChange={(checked) =>
                        patchDraft({ multilineEnabled: checked })
                      }
                    />
                  </div>
                </div>
                <Field
                  label="起始 Pattern"
                  hint="multiline.pattern"
                  error={editorErrors().multiline_pattern}
                >
                  <TextInput
                    mono
                    value={state().multilinePattern}
                    disabled={readonly()}
                    placeholder="^\d{4}-\d{2}-\d{2}"
                    onInput={(v) => patchDraft({ multilinePattern: v })}
                  />
                </Field>
                <Field label="合并方向" hint="match">
                  <Select
                    value={state().multilineMatch}
                    options={withCurrentOption(
                      MULTILINE_MATCH_OPTIONS,
                      state().multilineMatch,
                    )}
                    disabled={readonly()}
                    onInput={(v) => patchDraft({ multilineMatch: v })}
                  />
                </Field>
                <div class="field">
                  <span class="label">
                    <span>取反</span>
                    <span class="hint">multiline.negate</span>
                  </span>
                  <div class="row" style={{ 'min-height': '32px' }}>
                    <Checkbox
                      checked={state().multilineNegate}
                      disabled={readonly()}
                      label="反转匹配条件"
                      onChange={(checked) =>
                        patchDraft({ multilineNegate: checked })
                      }
                    />
                  </div>
                </div>
                <Field
                  label="最大行数"
                  hint="max_lines"
                  error={editorErrors().multiline_max_lines}
                >
                  <TextInput
                    type="number"
                    value={String(state().multilineMaxLines)}
                    disabled={readonly()}
                    onInput={(v) =>
                      patchDraft({ multilineMaxLines: toInt(v) })
                    }
                  />
                </Field>
                <Field
                  label="超时（毫秒）"
                  hint="timeout_ms"
                  error={editorErrors().multiline_timeout}
                >
                  <TextInput
                    type="number"
                    value={String(state().multilineTimeoutMs)}
                    disabled={readonly()}
                    onInput={(v) =>
                      patchDraft({ multilineTimeoutMs: toInt(v) })
                    }
                  />
                </Field>
              </div>

              <Field label="示例日志" hint="examples · 每行一个样例">
                <TextArea
                  mono
                  rows={4}
                  value={state().examplesText}
                  disabled={readonly()}
                  placeholder={'2025-01-01 12:00:00 [INFO] started\n2025-01-01 12:00:01 [ERROR] failed'}
                  onInput={(v) => patchDraft({ examplesText: v })}
                />
              </Field>

              <div class="divider" />
              <div class="section-label">解析预览</div>

              <Field label="样本日志" hint="每行一条，按行拆分">
                <TextArea
                  mono
                  rows={5}
                  value={previewSamples()}
                  placeholder="粘贴用于验证模板的日志样本，每行一条…"
                  onInput={(v) => setPreviewSamples(v)}
                />
              </Field>

              <div class="row wrap">
                <Button
                  size="sm"
                  variant="primary"
                  loading={previewing()}
                  onClick={runPreview}
                >
                  <Icon name="play" size={12} />
                  解析样本
                </Button>
                <span class="mono-sm faint">
                  当前 {splitLines(previewSamples()).length} 行样本
                </span>
                <Show when={previewResult()}>
                  <Badge tone={previewRateTone()}>
                    成功率 {previewRateText()}
                  </Badge>
                </Show>
              </div>

              <Show when={previewError()}>
                <div class="danger-note">{previewError()}</div>
              </Show>

              <Show when={previewResult()}>
                {(result) => (
                  <div style={{ 'max-height': '320px', 'overflow': 'auto' }}>
                    <table class="data-table">
                      <thead>
                        <tr>
                          <th>原始日志</th>
                          <th>结果</th>
                          <th>时间</th>
                          <th>级别</th>
                          <th>消息</th>
                          <th>错误</th>
                        </tr>
                      </thead>
                      <tbody>
                        <For each={result().samples}>
                          {(sample) => (
                            <tr>
                              <td
                                class="mono-sm"
                                title={sample.raw_log}
                                style={{
                                  'max-width': '220px',
                                  'word-break': 'break-word',
                                }}
                              >
                                {truncate(sample.raw_log, 140)}
                              </td>
                              <td>
                                <Badge tone={sample.success ? 'mint' : 'ember'}>
                                  {sample.success ? '成功' : '失败'}
                                </Badge>
                              </td>
                              <td class="mono-sm faint">
                                {formatTime(sample.parsed?.timestamp)}
                              </td>
                              <td>
                                <Show
                                  when={sample.parsed}
                                  fallback={<span class="faint">—</span>}
                                >
                                  {(parsed) => (
                                    <LevelBadge level={parsed().level} />
                                  )}
                                </Show>
                              </td>
                              <td
                                class="mono-sm"
                                style={{
                                  'max-width': '260px',
                                  'word-break': 'break-word',
                                }}
                              >
                                {truncate(sample.parsed?.message ?? '—', 180)}
                              </td>
                              <td
                                class="mono-sm ember"
                                style={{
                                  'max-width': '200px',
                                  'word-break': 'break-word',
                                }}
                              >
                                {sample.error || '—'}
                              </td>
                            </tr>
                          )}
                        </For>
                      </tbody>
                    </table>
                  </div>
                )}
              </Show>
            </div>
          )}
        </Show>
      </Modal>

      <Modal
        open={aiOpen()}
        title="AI 生成解析模板"
        subtitle="从样本推断正则、时间字段与级别映射"
        size="lg"
        onClose={closeAiModal}
        footer={
          <>
            <Button
              variant="ghost"
              disabled={aiGenerating()}
              onClick={closeAiModal}
            >
              取消
            </Button>
            <Button
              variant="primary"
              loading={aiGenerating()}
              onClick={() => void generateWithAi()}
            >
              <Icon name="wand" size={12} />
              生成模板
            </Button>
          </>
        }
      >
        <div class="form-stack">
          <div class="danger-note">
            样本文本会发送给所选 AI 提供商用于生成模板。请先脱敏，避免包含口令、密钥、Token 或个人信息。
          </div>

          <Field
            label="样本日志"
            hint={`每行一条 · 当前 ${splitLines(aiSamples()).length} 行`}
            error={aiErrors().samples}
          >
            <TextArea
              mono
              rows={8}
              value={aiSamples()}
              placeholder="粘贴至少一行具有代表性的日志样本，每行一条…"
              onInput={(v) => setAiSamples(v)}
            />
          </Field>

          <div class="form-row">
            <Field
              label="AI 提供商"
              hint="provider"
              error={aiErrors().provider}
            >
              <Select
                value={aiProvider()}
                options={aiProviders().map((provider) => ({
                  value: provider,
                  label: aiProviderLabel(provider),
                }))}
                onInput={(v) => setAiProvider(v)}
              />
            </Field>
            <Field
              label="模板名称"
              hint="template_name"
              error={aiErrors().template_name}
            >
              <TextInput
                value={aiName()}
                placeholder="Nginx access 模板"
                onInput={(v) => setAiName(v)}
              />
            </Field>
            <Field
              label="分类"
              hint="category"
              error={aiErrors().category}
            >
              <TextInput
                value={aiCategory()}
                placeholder="web_server"
                onInput={(v) => setAiCategory(v)}
              />
            </Field>
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleting())}
        title="删除解析模板"
        message={`确认删除模板「${deleting()?.name ?? ''}」？删除后不可恢复，已使用该模板采集的数据不受影响。`}
        confirmText="删除模板"
        danger
        loading={deleteLoading()}
        onConfirm={() => void confirmDelete()}
        onCancel={() => {
          if (!deleteLoading()) setDeleting(null)
        }}
      />
    </div>
  )
}
