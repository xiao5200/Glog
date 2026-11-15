import { For, Show, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { AIProviderInfo, SaveAIConfigRequest } from '../../types'
import { formatTime } from '../../utils'
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Icon,
  Modal,
  Select,
  TextArea,
  TextInput,
  Toggle,
} from '../ui'
import { SettingsPanel, parseStringMap, toInt } from './shared'

const AI_DEFAULTS: AIProviderInfo[] = [
  {
    provider: 'openai',
    name: 'OpenAI',
    endpoint: 'https://api.openai.com/v1',
    model: 'gpt-4o-mini',
    enabled: false,
    priority: 0,
    has_key: false,
    configured: false,
    updated_at: '',
  },
  {
    provider: 'deepseek',
    name: 'DeepSeek',
    endpoint: 'https://api.deepseek.com/v1',
    model: 'deepseek-chat',
    enabled: false,
    priority: 0,
    has_key: false,
    configured: false,
    updated_at: '',
  },
  {
    provider: 'ollama',
    name: 'Ollama（本地）',
    endpoint: 'http://127.0.0.1:11434/v1',
    model: 'qwen2.5:7b',
    enabled: false,
    priority: 0,
    has_key: false,
    configured: false,
    updated_at: '',
  },
  {
    provider: 'custom',
    name: '自定义 / OpenAI 兼容',
    endpoint: '',
    model: '',
    enabled: false,
    priority: 0,
    has_key: false,
    configured: false,
    updated_at: '',
  },
]

const PROVIDER_LABELS: Record<string, string> = {
  openai: 'OpenAI',
  deepseek: 'DeepSeek',
  ollama: 'Ollama（本地）',
  custom: '自定义 / OpenAI 兼容',
}

function providerLabel(provider: string): string {
  return PROVIDER_LABELS[provider] ?? provider
}

function defaultEndpoint(provider: string): string {
  if (provider === 'openai') return 'https://api.openai.com/v1'
  if (provider === 'deepseek') return 'https://api.deepseek.com/v1'
  if (provider === 'ollama') return 'http://127.0.0.1:11434/v1'
  return ''
}

function defaultModel(provider: string): string {
  if (provider === 'openai') return 'gpt-4o-mini'
  if (provider === 'deepseek') return 'deepseek-chat'
  if (provider === 'ollama') return 'qwen2.5:7b'
  return ''
}

function endpointPlaceholder(provider: string): string {
  return defaultEndpoint(provider) || 'https://your-endpoint/v1'
}

function modelPlaceholder(provider: string): string {
  return defaultModel(provider) || 'model-name'
}

function mergeProviders(remote: AIProviderInfo[]): AIProviderInfo[] {
  const merged = new Map<string, AIProviderInfo>()
  for (const def of AI_DEFAULTS) merged.set(def.provider, { ...def })
  for (const item of remote) {
    if (!item || typeof item.provider !== 'string' || !item.provider) continue
    const key = item.provider.toLowerCase()
    const base = merged.get(key)
    const priority = Number(item.priority)
    merged.set(key, {
      provider: key,
      name: item.name || base?.name || providerLabel(key),
      endpoint: item.endpoint ?? base?.endpoint ?? '',
      model: item.model ?? base?.model ?? '',
      enabled: Boolean(item.enabled),
      priority: Number.isFinite(priority) ? priority : (base?.priority ?? 0),
      has_key: Boolean(item.has_key),
      configured: Boolean(item.configured),
      updated_at: item.updated_at || '',
    })
  }
  return Array.from(merged.values())
}

interface AIEditorState {
  provider: string
  isNew: boolean
  name: string
  endpoint: string
  model: string
  apiKey: string
  enabled: boolean
  priority: number
  parametersText: string
  hasKey: boolean
}

export function AISettings() {
  const [providers, setProviders] = createSignal<AIProviderInfo[]>(
    AI_DEFAULTS.map((item) => ({ ...item })),
  )
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [testing, setTesting] = createSignal<string | null>(null)
  const [toggling, setToggling] = createSignal<string | null>(null)
  const [editor, setEditor] = createSignal<AIEditorState | null>(null)
  const [editorErrors, setEditorErrors] = createSignal<Record<string, string>>({})
  const [saving, setSaving] = createSignal(false)
  const [detectedModels, setDetectedModels] = createSignal<string[]>([])
  const [detectingModels, setDetectingModels] = createSignal(false)
  const [detectError, setDetectError] = createSignal('')
  const [detectTried, setDetectTried] = createSignal(false)
  const [testingModel, setTestingModel] = createSignal(false)
  let detectSeq = 0

  async function loadProviders(reportError = true): Promise<void> {
    setLoading(true)
    try {
      const list = await api.ai.providers()
      setProviders(mergeProviders(Array.isArray(list) ? list : []))
      setLoadError('')
    } catch (err) {
      setLoadError(errorText(err))
      if (reportError) notify('error', '加载 AI 提供商失败', errorText(err))
    } finally {
      setLoading(false)
    }
  }

  onMount(() => {
    void loadProviders(true)
  })

  function resetModelDetection(): void {
    detectSeq += 1
    setDetectedModels([])
    setDetectingModels(false)
    setDetectError('')
    setDetectTried(false)
  }

  function openEditor(info: AIProviderInfo, isNew: boolean): void {
    resetModelDetection()
    setEditor({
      provider: info.provider,
      isNew,
      name: info.name,
      endpoint: info.endpoint,
      model: info.model,
      apiKey: '',
      enabled: info.enabled,
      priority: info.priority,
      parametersText: '',
      hasKey: info.has_key,
    })
    setEditorErrors({})
  }

  function openNewEditor(): void {
    const list = providers()
    const candidate = list.find((item) => !item.configured) ?? list[0] ?? AI_DEFAULTS[0]
    openEditor(candidate, true)
  }

  function patchEditor(patch: Partial<AIEditorState>): void {
    setEditor((prev) => (prev ? { ...prev, ...patch } : prev))
  }

  function closeEditor(): void {
    if (saving()) return
    resetModelDetection()
    setEditor(null)
    setEditorErrors({})
  }

  function changeProvider(provider: string): void {
    resetModelDetection()
    const current = providers().find((item) => item.provider === provider)
    const fallback = AI_DEFAULTS.find((item) => item.provider === provider)
    const base = current ?? fallback
    patchEditor({
      provider,
      name: base?.name || providerLabel(provider),
      endpoint: base?.endpoint || defaultEndpoint(provider),
      model: base?.model || defaultModel(provider),
      hasKey: Boolean(base?.has_key),
      enabled: base?.enabled ?? false,
      priority: base?.priority ?? 0,
    })
  }

  async function detectEditorModels(): Promise<void> {
    const state = editor()
    if (!state || detectingModels()) return
    const seq = ++detectSeq
    setDetectingModels(true)
    setDetectError('')
    setDetectTried(false)
    setDetectedModels([])
    try {
      const list = await api.ai.listModels({
        provider: state.provider,
        endpoint: state.endpoint.trim(),
        api_key: state.apiKey.trim() || undefined,
      })
      if (seq !== detectSeq) return
      const models = Array.from(
        new Set(
          (Array.isArray(list) ? list : [])
            .filter((item) => typeof item === 'string')
            .map((item) => item.trim())
            .filter((item) => item.length > 0),
        ),
      )
      setDetectedModels(models)
      setDetectTried(true)
    } catch (err) {
      if (seq !== detectSeq) return
      setDetectError(errorText(err))
    } finally {
      if (seq === detectSeq) setDetectingModels(false)
    }
  }

  function pickDetectedModel(model: string, checked: boolean): void {
    if (checked) {
      patchEditor({ model })
      setEditorErrors((prev) => {
        if (!prev.model) return prev
        const next = { ...prev }
        delete next.model
        return next
      })
      return
    }
    if (editor()?.model.trim() === model) patchEditor({ model: '' })
  }

  async function testEditorModel(): Promise<void> {
    const state = editor()
    if (!state || testingModel()) return
    const model = state.model.trim()
    if (!model) return
    setTestingModel(true)
    try {
      await api.ai.testModel({
        provider: state.provider,
        endpoint: state.endpoint.trim(),
        model,
        api_key: state.apiKey.trim() || undefined,
      })
      notify(
        'success',
        '模型连接成功',
        `${providerLabel(state.provider)} · ${model}`,
      )
    } catch (err) {
      notify('error', '模型连接测试失败', errorText(err))
    } finally {
      setTestingModel(false)
    }
  }

  async function saveEditor(): Promise<void> {
    const state = editor()
    if (!state) return
    const nextErrors: Record<string, string> = {}
    if (!state.endpoint.trim()) nextErrors.endpoint = '请填写 Endpoint'
    if (!state.model.trim()) nextErrors.model = '请填写模型名称'
    if (
      state.enabled &&
      state.provider !== 'ollama' &&
      !state.hasKey &&
      !state.apiKey.trim()
    ) {
      nextErrors.api_key = '启用该提供商需要填写 API Key'
    }
    if (!Number.isInteger(state.priority) || state.priority < 0 || state.priority > 1000) {
      nextErrors.priority = '优先级需为 0–1000 的整数'
    }
    let parameters: Record<string, string> | undefined
    if (state.parametersText.trim()) {
      const parsed = parseStringMap(state.parametersText, 'parameters')
      if (!parsed.ok) nextErrors.parameters = parsed.error
      else parameters = parsed.value
    }
    setEditorErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      notify('warn', '请检查 AI 配置', 'Endpoint、模型、优先级或参数格式不合法。')
      return
    }

    setSaving(true)
    try {
      const req: SaveAIConfigRequest = {
        provider: state.provider,
        endpoint: state.endpoint.trim(),
        model: state.model.trim(),
        api_key: state.apiKey.trim() || undefined,
        enabled: state.enabled,
        priority: state.priority,
        parameters,
      }
      await api.ai.saveConfig(req)
      setProviders((prev) =>
        prev.map((item) =>
          item.provider === state.provider
            ? {
                ...item,
                name: state.name || providerLabel(state.provider),
                endpoint: state.endpoint.trim(),
                model: state.model.trim(),
                enabled: state.enabled,
                priority: state.priority,
                has_key: state.hasKey || Boolean(state.apiKey.trim()),
                configured: state.enabled,
                updated_at: new Date().toISOString(),
              }
            : item,
        ),
      )
      setEditor(null)
      setEditorErrors({})
      notify('success', 'AI 提供商已保存', providerLabel(state.provider))
      void loadProviders(false)
    } catch (err) {
      notify('error', '保存 AI 配置失败', errorText(err))
    } finally {
      setSaving(false)
    }
  }

  async function testProvider(info: AIProviderInfo): Promise<void> {
    if (testing()) return
    setTesting(info.provider)
    try {
      await api.ai.test(info.provider)
      notify(
        'success',
        '连接测试成功',
        `${providerLabel(info.provider)} 已响应。`,
      )
    } catch (err) {
      notify('error', '连接测试失败', errorText(err))
    } finally {
      setTesting(null)
    }
  }

  async function toggleProvider(
    info: AIProviderInfo,
    enabled: boolean,
  ): Promise<void> {
    if (toggling()) return
    if (!info.endpoint.trim() || !info.model.trim()) {
      notify('warn', '提供商尚未配置', '请先编辑并填写 Endpoint 与模型。')
      return
    }
    if (enabled && info.provider !== 'ollama' && !info.has_key) {
      notify(
        'warn',
        '缺少 API Key',
        '该提供商需要 API Key 才能启用，请先编辑并保存密钥。',
      )
      return
    }
    setToggling(info.provider)
    try {
      await api.ai.saveConfig({
        provider: info.provider,
        endpoint: info.endpoint,
        model: info.model,
        enabled,
        priority: info.priority,
      })
      setProviders((prev) =>
        prev.map((item) =>
          item.provider === info.provider
            ? { ...item, enabled, configured: enabled }
            : item,
        ),
      )
      notify(
        'success',
        enabled ? '已启用 AI 提供商' : '已停用 AI 提供商',
        providerLabel(info.provider),
      )
    } catch (err) {
      notify('error', '更新提供商状态失败', errorText(err))
    } finally {
      setToggling(null)
    }
  }

  const editorTitle = (): string => {
    const state = editor()
    if (!state) return 'AI 提供商配置'
    if (state.isNew) return '配置 AI 提供商'
    return `编辑 AI 提供商 · ${providerLabel(state.provider)}`
  }

  return (
    <div class="stack">
      <SettingsPanel
        title="AI 提供商"
        note="内置 OpenAI、DeepSeek、Ollama 与自定义四类；后端返回的已配置项会按 provider 合并。启用与优先级通过 SaveAIConfig 持久化。"
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={loading()}
              onClick={() => void loadProviders(true)}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
            <Button size="sm" onClick={openNewEditor}>
              <Icon name="plus" size={12} />
              配置提供商
            </Button>
          </>
        }
      >
        <Show when={loading()}>
          <div class="loading-line" />
        </Show>
        <Show when={loadError()}>
          <div class="danger-note">
            无法从后端读取提供商列表：{loadError()}。当前展示内置默认项，保存与测试可能需要 Wails 桌面环境。
          </div>
        </Show>

        <div style={{ 'overflow-x': 'auto' }}>
          <table class="data-table">
            <thead>
              <tr>
                <th>提供商</th>
                <th>Endpoint / 模型</th>
                <th>启用</th>
                <th>优先级</th>
                <th>密钥</th>
                <th>更新时间</th>
                <th style={{ width: '170px' }}>操作</th>
              </tr>
            </thead>
            <tbody>
              <For each={providers()}>
                {(item) => (
                  <tr>
                    <td>
                      <div style={{ 'font-weight': 600 }}>
                        {item.name || providerLabel(item.provider)}
                      </div>
                      <div class="row wrap" style={{ gap: '4px', 'margin-top': '3px' }}>
                        <span class="mono-sm faint">{item.provider}</span>
                        <Badge tone={item.configured ? 'mint' : 'muted'}>
                          {item.configured ? '已配置' : '未配置'}
                        </Badge>
                      </div>
                    </td>
                    <td>
                      <div class="mono-sm truncate" title={item.endpoint}>
                        {item.endpoint || '— 未填写 Endpoint'}
                      </div>
                      <div class="mono-sm faint truncate" title={item.model}>
                        {item.model || '— 未填写模型'}
                      </div>
                    </td>
                    <td>
                      <Toggle
                        checked={item.enabled}
                        label={`启用 ${providerLabel(item.provider)}`}
                        disabled={loading() || toggling() !== null}
                        onChange={(checked) => void toggleProvider(item, checked)}
                      />
                    </td>
                    <td class="numeric mono-sm">{item.priority}</td>
                    <td>
                      <Badge tone={item.has_key ? 'mint' : 'muted'}>
                        {item.has_key ? '已保存密钥' : '无密钥'}
                      </Badge>
                    </td>
                    <td class="mono-sm faint">
                      {formatTime(item.updated_at)}
                    </td>
                    <td>
                      <div class="row" style={{ gap: '4px' }}>
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={testing() === item.provider}
                          disabled={testing() !== null}
                          onClick={() => void testProvider(item)}
                        >
                          测试连接
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => openEditor(item, false)}
                        >
                          编辑
                        </Button>
                      </div>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>

        <div class="detail-note">
          <div class="row" style={{ gap: '7px', 'align-items': 'flex-start' }}>
            <Icon name="lock" size={14} />
            <span>
              API 密钥使用 AES-256-GCM 加密存储在本地 data 目录，界面不会回显明文。只有在你主动测试连接、生成模板或发起 AI 分析时，日志样本才会发送给所选提供商。
            </span>
          </div>
        </div>
      </SettingsPanel>

      <Modal
        open={Boolean(editor())}
        title={editorTitle()}
        subtitle="密钥留空表示保持现有配置；参数会以 JSON 校验后提交。"
        onClose={closeEditor}
        footer={
          <>
            <Button variant="ghost" disabled={saving()} onClick={closeEditor}>
              取消
            </Button>
            <Button
              variant="primary"
              loading={saving()}
              onClick={() => void saveEditor()}
            >
              <Icon name="save" size={12} />
              保存配置
            </Button>
          </>
        }
      >
        <Show when={editor()}>
          {(state) => (
            <div class="form-stack">
              <div class="form-row">
                <Field label="提供商" hint="provider">
                  <Select
                    value={state().provider}
                    options={providers().map((item) => ({
                      value: item.provider,
                      label: providerLabel(item.provider),
                    }))}
                    disabled={!state().isNew}
                    onInput={changeProvider}
                  />
                </Field>
                <Field
                  label="优先级"
                  hint="priority"
                  error={editorErrors().priority}
                >
                  <TextInput
                    type="number"
                    value={String(state().priority)}
                    onInput={(v) => patchEditor({ priority: toInt(v) })}
                  />
                </Field>
                <div class="field">
                  <span class="label">
                    <span>启用</span>
                    <span class="hint">enabled</span>
                  </span>
                  <div class="row" style={{ 'min-height': '32px' }}>
                    <Toggle
                      checked={state().enabled}
                      label="启用提供商"
                      onChange={(checked) => patchEditor({ enabled: checked })}
                    />
                    <span class="mono-sm faint">
                      {state().enabled ? '启用' : '停用'}
                    </span>
                  </div>
                </div>
              </div>

              <Field
                label="Endpoint"
                hint="API 基础地址"
                error={editorErrors().endpoint}
              >
                <TextInput
                  mono
                  value={state().endpoint}
                  placeholder={endpointPlaceholder(state().provider)}
                  onInput={(v) => patchEditor({ endpoint: v })}
                />
              </Field>

              <div class="form-row">
                <Field label="模型" hint="model" error={editorErrors().model}>
                  <TextInput
                    mono
                    value={state().model}
                    placeholder={modelPlaceholder(state().provider)}
                    onInput={(v) => patchEditor({ model: v })}
                  />
                </Field>
                <Field
                  label="API Key"
                  hint={
                    state().hasKey
                      ? '留空保持不变'
                      : '未设置密钥；加密存储'
                  }
                  error={editorErrors().api_key}
                >
                  <TextInput
                    type="password"
                    mono
                    value={state().apiKey}
                    placeholder={
                      state().hasKey ? '••••••••（已存储）' : 'sk-...'
                    }
                    onInput={(v) => patchEditor({ apiKey: v })}
                  />
                </Field>
              </div>

              <div class="field">
                <span class="label">
                  <span>模型识别</span>
                  <span class="hint">list_models · test_model</span>
                </span>
                <div class="row wrap" style={{ gap: '8px' }}>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={detectingModels()}
                    onClick={() => void detectEditorModels()}
                  >
                    <Icon name="wand" size={12} />
                    识别模型
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={testingModel()}
                    disabled={!state().model.trim()}
                    onClick={() => void testEditorModel()}
                  >
                    <Icon name="wifi" size={12} />
                    测试模型
                  </Button>
                  <span class="mono-sm faint">
                    测试会直接使用上方当前填写的模型，无需先保存。
                  </span>
                </div>

                <Show when={!state().apiKey.trim() && !state().hasKey}>
                  <div class="detail-note">
                    尚未填写 API Key，也未保存过密钥。识别与测试仍会尝试连接；如果服务需要鉴权，请先填写密钥（本地 Ollama 等服务通常可免密钥）。
                  </div>
                </Show>

                <Show when={detectError()}>
                  <div class="danger-note">识别模型失败：{detectError()}</div>
                </Show>

                <Show when={detectedModels().length > 0}>
                  <span class="mono-sm faint">
                    共 {detectedModels().length} 个可用模型；当前每个提供商使用一个模型。
                  </span>
                  <div
                    class="scrollable scroll-thin"
                    style={{
                      'max-height': '180px',
                      padding: '8px 10px',
                      border: '1px solid var(--border)',
                      'border-radius': 'var(--r-md)',
                      background: 'var(--surface)',
                    }}
                  >
                    <div style={{ display: 'grid', gap: '7px' }}>
                      <For each={detectedModels()}>
                        {(model) => (
                          <Checkbox
                            checked={state().model.trim() === model}
                            label={model}
                            onChange={(checked) =>
                              pickDetectedModel(model, checked)
                            }
                          />
                        )}
                      </For>
                    </div>
                  </div>
                </Show>

                <Show
                  when={
                    detectTried() &&
                    detectedModels().length === 0 &&
                    !detectError() &&
                    !detectingModels()
                  }
                >
                  <span class="mono-sm faint">
                    未识别到可用模型，请检查 Endpoint、API Key，或该服务是否提供模型列表接口。
                  </span>
                </Show>
              </div>

              <Field
                label="请求参数"
                hint="parameters · JSON 对象，可选"
                error={editorErrors().parameters}
              >
                <TextArea
                  mono
                  rows={5}
                  value={state().parametersText}
                  placeholder={'{\n  "temperature": "0.2",\n  "max_tokens": "4096"\n}'}
                  onInput={(v) => patchEditor({ parametersText: v })}
                />
              </Field>

              <div class="detail-note">
                请求参数必须是 JSON 对象，键和值均按字符串传递；留空表示不提交 parameters。保存前会尝试 JSON.parse 并阻止非法格式。
              </div>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  )
}
