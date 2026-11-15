import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
  onMount,
  untrack,
} from 'solid-js'
import type { HostNetworkSnapshot } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { formatInt, formatTime } from '../../utils'
import { Badge, Button, Checkbox, EmptyState, Icon, PageBusy } from '../ui'

const PORT_LIMIT = 100
const CONNECTION_LIMIT = 100
const LIVE_REFRESH_MS = 5000

function endpoint(address: string | undefined, port: number | undefined): string {
  const host = String(address ?? '').trim() || '*'
  const value = Number(port ?? 0)
  if (host.includes(':') && !host.startsWith('[')) return `[${host}]:${value}`
  return `${host}:${value}`
}

function processLabel(process: string | undefined, pid: number | undefined): string {
  const name = String(process ?? '').trim()
  const numeric = Number(pid ?? 0)
  if (name && numeric > 0) return `${name} (${numeric})`
  if (name) return name
  if (numeric > 0) return `PID ${numeric}`
  return '—'
}

/** 展示本机监听端口、外联连接与外部 IP 的网络连接快照，支持实时刷新。 */
export function HostNetworkPanel() {
  const [snapshot, setSnapshot] = createSignal<HostNetworkSnapshot | null>(null)
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [showAll, setShowAll] = createSignal(false)
  const [live, setLive] = createSignal(false)
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal('')

  let loadSeq = 0

  const load = async (manual = false) => {
    if (loading()) return
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const result = await api.system.hostNetwork()
      if (seq !== loadSeq) return
      setSnapshot(result ?? null)
      setLastUpdatedAt(new Date().toISOString())
      if (manual) {
        notify(
          'success',
          '主机网络信息已刷新',
          `${formatInt(result?.ports?.length ?? 0)} 个监听端口，${
            result?.connections?.filter((item) => item.external).length ?? 0
          } 条外部连接。`,
        )
      }
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      // 实时轮询失败时只更新内联错误，避免每 5 秒重复弹出通知。
      if (manual || !live()) {
        notify('error', '读取主机网络信息失败', message)
      }
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  onMount(() => {
    void load(false)
  })

  createEffect(() => {
    if (!live()) return
    const timer = setInterval(() => {
      void load(false)
    }, LIVE_REFRESH_MS)
    onCleanup(() => clearInterval(timer))
    // 立即刷新一次；用 untrack 避免把 load 内部读取的 loading 信号变成依赖。
    untrack(() => {
      void load(false)
    })
  })

  const ports = createMemo(() => snapshot()?.ports ?? [])
  const connections = createMemo(() => snapshot()?.connections ?? [])
  const externalConnections = createMemo(() => connections().filter((item) => item.external))
  const externalIps = createMemo(() => snapshot()?.external_ips ?? [])
  const errors = createMemo(() => snapshot()?.errors ?? [])

  const visiblePorts = createMemo(() => ports().slice(0, PORT_LIMIT))
  const visibleConnections = createMemo(() =>
    (showAll() ? connections() : externalConnections()).slice(0, CONNECTION_LIMIT),
  )

  const portTotal = () => snapshot()?.total_ports ?? ports().length
  const connectionTotal = () => snapshot()?.total_connections ?? connections().length

  const externalCount = () => externalConnections().length

  return (
    <div class="panel">
      <div class="panel-head">
        <div class="panel-title">
          <span class="tick" />
          网络连接
          <Badge tone="muted">HOST</Badge>
        </div>
        <div class="row wrap" style={{ gap: '8px', 'align-items': 'center' }}>
          <span class="mono-sm faint">
            更新于 {formatTime(lastUpdatedAt() || snapshot()?.generated_at)}
          </span>
          <Checkbox
            checked={live()}
            label="实时刷新"
            onChange={(checked) => setLive(checked)}
          />
          <Show when={live()}>
            <span class="mono-sm faint">每 5 秒</span>
          </Show>
          <Button
            size="sm"
            variant="ghost"
            loading={loading()}
            disabled={loading()}
            onClick={() => void load(true)}
          >
            <Icon name="refresh" size={12} />
            刷新
          </Button>
        </div>
      </div>

      <Show when={loading()}>
        <div class="loading-line" />
      </Show>

      <Show when={loading() && !snapshot()}>
        <div class="panel-body" style={{ 'padding-top': '8px', 'padding-bottom': '4px' }}>
          <PageBusy visible text="正在采集本机监听端口与连接…" />
        </div>
      </Show>

      <Show when={loadError()}>
        <div class="panel-body" style={{ 'padding-top': '10px', 'padding-bottom': '0' }}>
          <div class="danger-note">
            <div class="row between wrap" style={{ gap: '8px' }}>
              <span>读取主机网络信息失败：{loadError()}</span>
              <Button size="sm" variant="ghost" onClick={() => void load(true)}>
                重试
              </Button>
            </div>
          </div>
        </div>
      </Show>

      <Show
        when={snapshot()}
        fallback={
          <Show when={!loading() && !loadError()}>
            <div class="panel-body">
              <EmptyState
                icon="globe"
                title="暂无主机网络信息"
                text="点击刷新读取本机监听端口、外部连接与外部 IP，或开启实时刷新每 5 秒自动更新。"
              />
            </div>
          </Show>
        }
      >
        {(data) => (
          <div class="panel-body stack" style={{ gap: '16px' }}>
            <div class="row wrap" style={{ gap: '8px', 'align-items': 'center' }}>
              <span class="mono-sm faint">
                {data().hostname || '本机'} · {data().platform || '—'} / {data().arch || '—'}
              </span>
              <span class="chip">
                <Icon name="server" size={11} />
                监听端口{' '}
                <b>{formatInt(data().truncated ? ports().length : portTotal())}</b>
                <Show when={data().truncated}>
                  <span class="faint">/ 共 {formatInt(portTotal())}</span>
                </Show>
              </span>
              <span class="chip">
                <Icon name="external" size={11} />
                外联连接 <b>{formatInt(externalCount())}</b>
                <span class="faint">/ 共 {formatInt(connectionTotal())}</span>
              </span>
              <span class="chip">
                <Icon name="globe" size={11} />
                外部 IP <b>{formatInt(externalIps().length)}</b>
              </span>
            </div>

            <Show when={data().truncated}>
              <div class="detail-note">
                后端结果已截断：共 {formatInt(data().total_ports)} 个监听端口、
                {formatInt(data().total_connections)} 条连接，当前仅返回部分数据。
              </div>
            </Show>

            <div>
              <div class="section-label">监听端口</div>
              <Show
                when={visiblePorts().length > 0}
                fallback={
                  <div class="mono-sm faint" style={{ padding: '8px 0' }}>
                    未发现监听端口。
                  </div>
                }
              >
                <div style={{ 'overflow-x': 'auto' }}>
                  <table class="data-table" style={{ 'min-width': '680px' }}>
                    <thead>
                      <tr>
                        <th style={{ width: '80px' }}>协议</th>
                        <th>本地地址</th>
                        <th>进程 (PID)</th>
                        <th style={{ width: '88px' }}>暴露</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={visiblePorts()}>
                        {(port) => (
                          <tr>
                            <td>
                              <Badge tone="muted">{port.protocol || '—'}</Badge>
                            </td>
                            <td
                              class="mono-sm"
                              style={{ 'word-break': 'break-all' }}
                              title={endpoint(port.local_address, port.local_port)}
                            >
                              {endpoint(port.local_address, port.local_port)}
                            </td>
                            <td
                              class="mono-sm truncate"
                              title={processLabel(port.process, port.pid)}
                            >
                              {processLabel(port.process, port.pid)}
                            </td>
                            <td>
                              <Show
                                when={port.exposed}
                                fallback={<span class="mono-sm faint">本地</span>}
                              >
                                <Badge tone="amber">暴露</Badge>
                              </Show>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
                <Show when={ports().length > PORT_LIMIT}>
                  <div class="mono-sm faint" style={{ 'margin-top': '6px' }}>
                    仅显示前 {PORT_LIMIT} 条监听端口，共 {formatInt(ports().length)} 条。
                  </div>
                </Show>
              </Show>
            </div>

            <div>
              <div class="row between wrap" style={{ gap: '8px', 'align-items': 'center' }}>
                <div class="section-label" style={{ margin: 0 }}>
                  外部连接
                </div>
                <Checkbox
                  checked={showAll()}
                  label="显示全部连接（含内网）"
                  onChange={(checked) => setShowAll(checked)}
                />
              </div>

              <Show
                when={visibleConnections().length > 0}
                fallback={
                  <div class="mono-sm faint" style={{ padding: '8px 0' }}>
                    {showAll()
                      ? '未发现连接记录。'
                      : '当前未发现外部连接（external=true）。'}
                  </div>
                }
              >
                <div style={{ 'overflow-x': 'auto' }}>
                  <table class="data-table" style={{ 'min-width': '760px' }}>
                    <thead>
                      <tr>
                        <th style={{ width: '80px' }}>协议</th>
                        <th>本地</th>
                        <th>远端</th>
                        <th style={{ width: '110px' }}>状态</th>
                        <th>进程</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={visibleConnections()}>
                        {(conn) => (
                          <tr>
                            <td>
                              <Badge tone="muted">{conn.protocol || '—'}</Badge>
                            </td>
                            <td
                              class="mono-sm"
                              style={{ 'word-break': 'break-all' }}
                              title={endpoint(conn.local_address, conn.local_port)}
                            >
                              {endpoint(conn.local_address, conn.local_port)}
                            </td>
                            <td
                              class="mono-sm"
                              style={{ 'word-break': 'break-all' }}
                              title={endpoint(conn.remote_address, conn.remote_port)}
                            >
                              <div class="row wrap" style={{ gap: '5px' }}>
                                <Show when={conn.external}>
                                  <Badge tone="amber">外联</Badge>
                                </Show>
                                <span>
                                  {endpoint(conn.remote_address, conn.remote_port)}
                                </span>
                              </div>
                            </td>
                            <td class="mono-sm">{conn.state || '—'}</td>
                            <td
                              class="mono-sm truncate"
                              title={processLabel(conn.process, conn.pid)}
                            >
                              {processLabel(conn.process, conn.pid)}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
                <Show
                  when={
                    (showAll() ? connections().length : externalConnections().length) >
                    CONNECTION_LIMIT
                  }
                >
                  <div class="mono-sm faint" style={{ 'margin-top': '6px' }}>
                    仅显示前 {CONNECTION_LIMIT} 条连接，共{' '}
                    {formatInt(showAll() ? connections().length : externalConnections().length)}{' '}
                    条。
                  </div>
                </Show>
              </Show>
            </div>

            <div>
              <div class="section-label">外部 IP</div>
              <Show
                when={externalIps().length > 0}
                fallback={
                  <div class="mono-sm faint" style={{ padding: '8px 0' }}>
                    未记录外部 IP。
                  </div>
                }
              >
                <div class="row wrap" style={{ gap: '5px' }}>
                  <For each={externalIps()}>
                    {(ip) => (
                      <span class="chip mono-sm" title={ip}>
                        <Icon name="globe" size={10} />
                        {ip}
                      </span>
                    )}
                  </For>
                </div>
              </Show>
            </div>

            <Show when={errors().length > 0}>
              <div class="danger-note">
                <div class="row" style={{ gap: '7px', 'align-items': 'flex-start' }}>
                  <Icon name="alert" size={14} />
                  <span>采集过程中出现 {errors().length} 个错误：</span>
                </div>
                <div style={{ 'margin-top': '5px', 'max-height': '110px', overflow: 'auto' }}>
                  <For each={errors()}>
                    {(message) => (
                      <div class="mono-sm" style={{ 'margin-top': '3px', 'word-break': 'break-all' }}>
                        {message}
                      </div>
                    )}
                  </For>
                </div>
              </div>
            </Show>
          </div>
        )}
      </Show>
    </div>
  )
}
