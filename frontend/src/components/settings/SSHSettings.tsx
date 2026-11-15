import { For, Show, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { SaveSSHHostRequest, SSHHost } from '../../types'
import { formatTime } from '../../utils'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Icon,
  Modal,
  Select,
  TextInput,
} from '../ui'
import { SettingsPanel, toInt } from './shared'

const AUTH_OPTIONS = [
  { value: 'password', label: '密码认证' },
  { value: 'key', label: '密钥认证' },
]

interface HostDraft {
  id?: string
  name: string
  host: string
  port: number
  username: string
  auth_method: string
  password: string
  key_path: string
  proxy_jump: string
  has_password: boolean
}

function emptyDraft(): HostDraft {
  return {
    name: '',
    host: '',
    port: 22,
    username: 'root',
    auth_method: 'password',
    password: '',
    key_path: '',
    proxy_jump: '',
    has_password: false,
  }
}

export function SSHSettings() {
  const [hosts, setHosts] = createSignal<SSHHost[]>([])
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [testingId, setTestingId] = createSignal<string | null>(null)
  const [editor, setEditor] = createSignal<HostDraft | null>(null)
  const [editorErrors, setEditorErrors] = createSignal<Record<string, string>>({})
  const [saving, setSaving] = createSignal(false)
  const [removeTarget, setRemoveTarget] = createSignal<SSHHost | null>(null)
  const [removing, setRemoving] = createSignal(false)

  async function loadHosts(reportError = true): Promise<void> {
    setLoading(true)
    try {
      const list = await api.ssh.hosts()
      setHosts(Array.isArray(list) ? list : [])
      setLoadError('')
    } catch (err) {
      setLoadError(errorText(err))
      if (reportError) notify('error', '加载 SSH 主机失败', errorText(err))
    } finally {
      setLoading(false)
    }
  }

  onMount(() => {
    void loadHosts(true)
  })

  function openHost(host?: SSHHost): void {
    if (!host) {
      setEditor(emptyDraft())
      setEditorErrors({})
      return
    }
    setEditor({
      id: host.id,
      name: host.name ?? '',
      host: host.host ?? '',
      port: Number(host.port) || 22,
      username: host.username ?? '',
      auth_method: host.auth_method || 'password',
      password: '',
      key_path: host.key_path ?? '',
      proxy_jump: host.proxy_jump ?? '',
      has_password: Boolean(host.has_password),
    })
    setEditorErrors({})
  }

  function patchEditor(patch: Partial<HostDraft>): void {
    setEditor((prev) => (prev ? { ...prev, ...patch } : prev))
  }

  function closeEditor(): void {
    if (saving()) return
    setEditor(null)
    setEditorErrors({})
  }

  async function saveHost(): Promise<void> {
    const state = editor()
    if (!state) return
    const nextErrors: Record<string, string> = {}
    if (!state.name.trim()) nextErrors.name = '请填写主机名称'
    if (!state.host.trim()) nextErrors.host = '请填写主机地址'
    if (!Number.isInteger(state.port) || state.port < 1 || state.port > 65535) {
      nextErrors.port = '端口需为 1–65535 的整数'
    }
    if (!state.username.trim()) nextErrors.username = '请填写用户名'
    if (state.auth_method === 'password') {
      if (!state.password && !(state.id && state.has_password)) {
        nextErrors.password = '请填写密码'
      }
    } else if (state.auth_method === 'key' && !state.key_path.trim()) {
      nextErrors.key_path = '请填写密钥路径'
    }
    setEditorErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      notify('warn', '请检查 SSH 主机信息', '带 * 的必填字段或端口格式不正确。')
      return
    }

    setSaving(true)
    try {
      const req: SaveSSHHostRequest = {
        id: state.id,
        name: state.name.trim(),
        host: state.host.trim(),
        port: state.port,
        username: state.username.trim(),
        auth_method: state.auth_method,
        password: state.password || undefined,
        key_path:
          state.auth_method === 'key' ? state.key_path.trim() : undefined,
        proxy_jump: state.proxy_jump.trim() || undefined,
      }
      await api.ssh.save(req)
      notify(
        'success',
        state.id ? 'SSH 主机已更新' : 'SSH 主机已添加',
        `${state.name.trim()} · ${state.host.trim()}:${state.port}`,
      )
      setEditor(null)
      setEditorErrors({})
      await loadHosts(false)
    } catch (err) {
      notify('error', '保存 SSH 主机失败', errorText(err))
    } finally {
      setSaving(false)
    }
  }

  async function testHost(host: SSHHost): Promise<void> {
    if (testingId()) return
    setTestingId(host.id)
    try {
      await api.ssh.test(host.id)
      notify(
        'success',
        'SSH 连接成功',
        `${host.name} · ${host.host}:${host.port}`,
      )
    } catch (err) {
      notify('error', 'SSH 连接失败', errorText(err))
    } finally {
      setTestingId(null)
    }
  }

  async function confirmRemove(): Promise<void> {
    const target = removeTarget()
    if (!target) return
    setRemoving(true)
    try {
      await api.ssh.remove(target.id)
      setHosts((prev) => prev.filter((item) => item.id !== target.id))
      setRemoveTarget(null)
      notify('success', 'SSH 主机已删除', target.name)
      await loadHosts(false)
    } catch (err) {
      notify('error', '删除 SSH 主机失败', errorText(err))
    } finally {
      setRemoving(false)
    }
  }

  const editorTitle = (): string => {
    const state = editor()
    if (!state) return 'SSH 主机'
    return state.id
      ? `编辑 SSH 主机 · ${state.name || '未命名'}`
      : '新增 SSH 主机'
  }

  return (
    <div class="stack">
      <SettingsPanel
        title="SSH 主机"
        note="远程采集使用只读命令读取日志；凭据仅保存在本机。测试连接会真实建立一次 SSH 会话。"
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={loading()}
              onClick={() => void loadHosts(true)}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
            <Button size="sm" onClick={() => openHost()}>
              <Icon name="plus" size={12} />
              新增主机
            </Button>
          </>
        }
      >
        <Show when={loading()}>
          <div class="loading-line" />
        </Show>
        <Show when={loadError()}>
          <div class="danger-note">
            无法从后端读取 SSH 主机：{loadError()}。请确认 Wails 后端已启动后重试。
          </div>
        </Show>

        <Show
          when={hosts().length > 0}
          fallback={
            <EmptyState
              icon="server"
              title="尚未配置 SSH 主机"
              text="添加远程主机后，可在数据采集页通过 SSH 拉取只读日志。"
              action={
                <Button variant="primary" onClick={() => openHost()}>
                  <Icon name="plus" size={12} />
                  新增 SSH 主机
                </Button>
              }
            />
          }
        >
          <div style={{ 'overflow-x': 'auto' }}>
            <table class="data-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>地址</th>
                  <th>用户名</th>
                  <th>认证</th>
                  <th>代理跳板</th>
                  <th>最后使用</th>
                  <th style={{ width: '210px' }}>操作</th>
                </tr>
              </thead>
              <tbody>
                <For each={hosts()}>
                  {(host) => (
                    <tr>
                      <td>
                        <div style={{ 'font-weight': 600 }}>{host.name}</div>
                        <div
                          class="mono-sm faint truncate"
                          title={host.id}
                          style={{ 'max-width': '220px' }}
                        >
                          {host.id}
                        </div>
                      </td>
                      <td class="mono-sm">
                        {host.host}:{host.port}
                      </td>
                      <td class="mono-sm">{host.username}</td>
                      <td>
                        <div class="row wrap" style={{ gap: '4px' }}>
                          <Badge
                            tone={
                              host.auth_method === 'key' ? 'sky' : 'amber'
                            }
                          >
                            {host.auth_method === 'key' ? '密钥' : '密码'}
                          </Badge>
                          <Show
                            when={
                              host.auth_method !== 'key' &&
                              host.has_password
                            }
                          >
                            <span class="mono-sm faint">已保存</span>
                          </Show>
                        </div>
                      </td>
                      <td class="mono-sm">
                        {host.proxy_jump || '—'}
                      </td>
                      <td class="mono-sm faint">
                        {formatTime(host.last_used_at)}
                      </td>
                      <td>
                        <div class="row" style={{ gap: '4px' }}>
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={testingId() === host.id}
                            disabled={testingId() !== null}
                            onClick={() => void testHost(host)}
                          >
                            测试
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => openHost(host)}
                          >
                            编辑
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => setRemoveTarget(host)}
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

        <div class="detail-note">
          SSH 密码使用 AES-256-GCM 加密存储在本地 data 目录。SSH 采集仅执行只读命令（如 cat、tail、zgrep），不会修改远端系统或文件。
        </div>
      </SettingsPanel>

      <Modal
        open={Boolean(editor())}
        title={editorTitle()}
        subtitle="编辑时密码留空表示保持原密码；密码与密钥路径不会在列表中回显。"
        onClose={closeEditor}
        footer={
          <>
            <Button variant="ghost" disabled={saving()} onClick={closeEditor}>
              取消
            </Button>
            <Button
              variant="primary"
              loading={saving()}
              onClick={() => void saveHost()}
            >
              <Icon name="save" size={12} />
              保存主机
            </Button>
          </>
        }
      >
        <Show when={editor()}>
          {(state) => (
            <div class="form-stack">
              <div class="form-row">
                <Field label="名称" hint="必填" error={editorErrors().name}>
                  <TextInput
                    value={state().name}
                    placeholder="生产 Web 服务器"
                    onInput={(v) => patchEditor({ name: v })}
                  />
                </Field>
                <Field
                  label="地址"
                  hint="host / IP · 必填"
                  error={editorErrors().host}
                >
                  <TextInput
                    mono
                    value={state().host}
                    placeholder="192.168.1.10"
                    onInput={(v) => patchEditor({ host: v })}
                  />
                </Field>
                <Field
                  label="端口"
                  hint="默认 22；靶场等非标准端口（如 222）请手动修改"
                  error={editorErrors().port}
                >
                  <TextInput
                    type="number"
                    value={String(state().port)}
                    onInput={(v) => patchEditor({ port: toInt(v) })}
                  />
                </Field>
                <Field
                  label="用户名"
                  hint="必填"
                  error={editorErrors().username}
                >
                  <TextInput
                    mono
                    value={state().username}
                    placeholder="root"
                    onInput={(v) => patchEditor({ username: v })}
                  />
                </Field>
              </div>

              <div class="form-row">
                <Field label="认证方式" hint="auth_method">
                  <Select
                    value={state().auth_method}
                    options={AUTH_OPTIONS}
                    onInput={(v) => patchEditor({ auth_method: v })}
                  />
                </Field>
                <Show
                  when={state().auth_method === 'key'}
                  fallback={
                    <Field
                      label="密码"
                      hint={
                        state().id
                          ? state().has_password
                            ? '留空保持不变'
                            : '该主机未保存密码'
                          : 'AES-256-GCM 加密存储'
                      }
                      error={editorErrors().password}
                    >
                      <TextInput
                        type="password"
                        value={state().password}
                        placeholder={
                          state().has_password ? '••••••••' : '输入密码'
                        }
                        onInput={(v) => patchEditor({ password: v })}
                      />
                    </Field>
                  }
                >
                  <Field
                    label="密钥路径"
                    hint="本地私钥文件，如 ~/.ssh/id_rsa"
                    error={editorErrors().key_path}
                  >
                    <TextInput
                      mono
                      value={state().key_path}
                      placeholder="C:\Users\me\.ssh\id_rsa"
                      onInput={(v) => patchEditor({ key_path: v })}
                    />
                  </Field>
                </Show>
                <Field label="ProxyJump" hint="可选 · user@jump:22">
                  <TextInput
                    mono
                    value={state().proxy_jump}
                    placeholder="ops@jump-host:22"
                    onInput={(v) => patchEditor({ proxy_jump: v })}
                  />
                </Field>
              </div>

              <div class="detail-note">
                密码使用 AES-256-GCM 加密保存。SSH 采集只执行只读命令，建议使用受限账号并通过 ProxyJump 跳板访问目标主机。
              </div>
            </div>
          )}
        </Show>
      </Modal>

      <ConfirmDialog
        open={Boolean(removeTarget())}
        title="删除 SSH 主机"
        message={`确认删除「${removeTarget()?.name ?? ''}」？该操作会移除已保存的凭据，且不可撤销。`}
        confirmText="删除主机"
        danger
        loading={removing()}
        onConfirm={() => void confirmRemove()}
        onCancel={() => {
          if (!removing()) setRemoveTarget(null)
        }}
      />
    </div>
  )
}
