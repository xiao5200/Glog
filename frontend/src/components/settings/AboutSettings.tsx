import { For, Show, createSignal, onMount } from 'solid-js'
import { api, errorText } from '../../api'
import { notify, systemInfo } from '../../stores/app'
import type { SystemInfo } from '../../types'
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  StatTile,
  type IconName,
} from '../ui'
import { CopyableValue, SettingsPanel } from './shared'
import appIcon from '../../assets/app-icon.png'

const FEATURES: Array<{ icon: IconName; title: string; text: string }> = [
  {
    icon: 'upload',
    title: '多源采集',
    text: '本地文件、Windows 事件日志与 SSH 远程读取，支持断点任务与采集进度。',
  },
  {
    icon: 'search',
    title: '高速检索',
    text: '本地索引与结构化字段查询，支持时间、主机、级别、字段过滤与聚合。',
  },
  {
    icon: 'book',
    title: '模板解析',
    text: '内置 / 自定义 / AI 模板，正则与 JSON 解析、多行合并和解析预览。',
  },
  {
    icon: 'shield-check',
    title: '威胁检测',
    text: '按时间范围调用 AI 检测日志，并结合本机端口、外联连接与外部 IP 检查暴露面。',
  },
  {
    icon: 'wand',
    title: 'AI 辅助',
    text: '可选的 AI 异常检测、根因分析与报告生成，按提供商独立配置。',
  },
  {
    icon: 'package',
    title: '取证输出',
    text: '调查工单、证据标记与 SHA-256 校验的取证包导出，便于交接复核。',
  },
]

export function AboutSettings() {
  const [info, setInfo] = createSignal<SystemInfo | null>(systemInfo())
  const [loading, setLoading] = createSignal(false)
  const [opening, setOpening] = createSignal('')

  const infoValue = (): SystemInfo | null => info() ?? systemInfo()
  const version = (): string => infoValue()?.version || '1.0.0'

  async function loadInfo(reportError = true): Promise<void> {
    setLoading(true)
    try {
      const fresh = await api.system.info()
      setInfo(fresh)
    } catch (err) {
      if (reportError || !infoValue()) {
        notify('error', '读取系统信息失败', errorText(err))
      }
    } finally {
      setLoading(false)
    }
  }

  onMount(() => {
    void loadInfo(false)
  })

  async function openDir(
    key: string,
    label: string,
    path: string | undefined,
  ): Promise<void> {
    if (!path) {
      notify('warn', `${label}不可用`, '尚未获取到路径信息。')
      return
    }
    setOpening(key)
    try {
      await api.system.showInExplorer(path)
    } catch (err) {
      notify('error', `打开${label}失败`, errorText(err))
    } finally {
      setOpening('')
    }
  }

  return (
    <div class="stack">
      <div class="panel">
        <div class="panel-body stack">
          <div
            class="row wrap between"
            style={{ 'align-items': 'flex-start', gap: '18px' }}
          >
            <div class="grow">
              <div class="eyebrow">ABOUT / GLOG</div>
              <div class="row" style={{ gap: '12px' }}>
                <div
                  class="brand-mark"
                  style={{ width: '46px', height: '46px' }}
                >
                  <img src={appIcon} alt="GLog" />
                </div>
                <div>
                  <h2
                    style={{
                      'font-size': '30px',
                      'line-height': '1',
                      'letter-spacing': '0.14em',
                    }}
                  >
                    G<span class="accent">//</span>LOG
                  </h2>
                  <div class="mono-sm faint">
                    FORENSIC LOG INTELLIGENCE CONSOLE
                  </div>
                </div>
              </div>
              <p
                class="muted"
                style={{
                  'margin-top': '12px',
                  'max-width': '720px',
                  'font-size': '12px',
                  'line-height': '1.8',
                }}
              >
                GLog 是面向本地日志取证与威胁分析的桌面控制台，强调本地优先、可复现与证据链完整性。
                设置页无需打开项目即可访问，系统配置与凭据均保存在本机。
              </p>
            </div>
            <div class="row wrap">
              <Badge tone="mint">v{version()}</Badge>
              <Badge tone="sky">GO</Badge>
              <Badge tone="violet">WAILS</Badge>
              <Badge tone="mint">SOLIDJS</Badge>
            </div>
          </div>

          <div class="stat-grid">
            <StatTile
              label="版本"
              value={version()}
              note="GLog Desktop"
              tone="mint"
            />
            <StatTile
              label="运行平台"
              value={infoValue()?.os ?? '—'}
              note={infoValue()?.arch ?? '等待系统信息'}
              tone="sky"
            />
            <StatTile
              label="数据位置"
              value="本地"
              note="SQLite · WAL · data 目录"
            />
            <StatTile
              label="密钥保护"
              value="AES-256"
              note="GCM · 本地加密"
            />
          </div>
        </div>
      </div>

      <SettingsPanel
        title="技术栈"
        note="桌面壳层 Wails；后端 Go；前端 SolidJS + TypeScript；本地索引 SQLite FTS；图表 ECharts。"
      >
        <div class="row wrap">
          <Badge tone="sky">Go</Badge>
          <Badge tone="violet">Wails</Badge>
          <Badge tone="mint">SolidJS</Badge>
          <Badge tone="muted">TypeScript</Badge>
          <Badge tone="muted">SQLite FTS</Badge>
          <Badge tone="muted">ECharts</Badge>
        </div>
        <p class="mono-sm faint" style={{ 'line-height': '1.8' }}>
          Go 后端负责采集、解析、索引与检测；Wails 提供 Windows 桌面容器与文件系统能力；
          SolidJS 前端采用深色取证控制台设计，所有界面数据均来自本地绑定接口。
        </p>
      </SettingsPanel>

      <SettingsPanel title="核心能力">
        <div class="form-row">
          <For each={FEATURES}>
            {(feature) => (
              <div
                style={{
                  border: '1px solid var(--border)',
                  'background-color': 'var(--surface-soft)',
                  padding: '12px 13px',
                }}
              >
                <div class="row" style={{ 'margin-bottom': '6px' }}>
                  <Icon name={feature.icon} size={14} />
                  <b
                    style={{
                      'font-family': 'var(--font-display)',
                      'font-size': '12.5px',
                      'letter-spacing': '0.05em',
                    }}
                  >
                    {feature.title}
                  </b>
                </div>
                <p
                  class="mono-sm faint"
                  style={{ 'line-height': '1.7' }}
                >
                  {feature.text}
                </p>
              </div>
            )}
          </For>
        </div>
      </SettingsPanel>

      <SettingsPanel
        title="数据安全"
        note="GLog 不要求云端账号；仅在用户主动启用并调用 AI 功能时产生网络请求。"
      >
        <div class="success-note">
          所有项目、日志、索引与导出默认保存在本地 data 目录，不依赖云端服务，也不会自动上传。
        </div>
        <div class="detail-note">
          AI 分析为可选能力。未配置提供商或未主动发起请求时，日志不会离开本机；发送样本前请先脱敏。
        </div>
        <div class="success-note">
          AI API 密钥使用 AES-256-GCM 加密存储于本地 data 目录；界面仅展示是否已配置，不回显明文。
        </div>
      </SettingsPanel>

      <SettingsPanel
        title="当前系统信息"
        actions={
          <>
            <Button
              size="sm"
              variant="ghost"
              loading={loading()}
              onClick={() => void loadInfo(true)}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
            <Button
              size="sm"
              loading={opening() === 'data'}
              disabled={!infoValue()?.data_path}
              onClick={() =>
                void openDir('data', '数据目录', infoValue()?.data_path)
              }
            >
              <Icon name="folder" size={12} />
              打开数据目录
            </Button>
            <Button
              size="sm"
              loading={opening() === 'log'}
              disabled={!infoValue()?.log_path}
              onClick={() =>
                void openDir('log', '日志目录', infoValue()?.log_path)
              }
            >
              <Icon name="file" size={12} />
              打开日志目录
            </Button>
          </>
        }
      >
        <Show
          when={infoValue()}
          fallback={
            <EmptyState
              icon="cpu"
              title="系统信息不可用"
              text="无法从后端读取当前运行环境信息。"
              action={
                <Button variant="primary" onClick={() => void loadInfo(true)}>
                  重新加载
                </Button>
              }
            />
          }
        >
          {(sys) => (
            <div class="stack">
              <div class="row wrap">
                <Badge tone="mint">
                  {sys().os} / {sys().arch}
                </Badge>
                <Badge tone="sky">v{sys().version || version()}</Badge>
                <Badge tone="muted">桌面本地运行</Badge>
              </div>
              <dl class="kv">
                <dt>操作系统</dt>
                <dd>
                  {sys().os} / {sys().arch}
                </dd>
                <dt>应用版本</dt>
                <dd>v{sys().version || version()}</dd>
                <dt>数据目录</dt>
                <dd>
                  <CopyableValue value={sys().data_path} label="数据目录" />
                </dd>
                <dt>项目目录</dt>
                <dd>
                  <CopyableValue value={sys().projects_path} label="项目目录" />
                </dd>
                <dt>日志目录</dt>
                <dd>
                  <CopyableValue value={sys().log_path} label="日志目录" />
                </dd>
                <dt>数据库</dt>
                <dd>
                  <CopyableValue value={sys().db_path} label="数据库路径" />
                </dd>
                <dt>可执行文件</dt>
                <dd>
                  <CopyableValue value={sys().executable} label="可执行文件" />
                </dd>
              </dl>
            </div>
          )}
        </Show>
      </SettingsPanel>

      <div class="panel">
        <div class="panel-body stack">
          <div class="danger-note" style={{ 'line-height': '1.8' }}>
            <b>免责声明：</b>
            GLog 为个人日志分析工具。自动检测、模板解析与 AI 生成结论可能存在误报或遗漏，
            不能替代专业司法鉴定；任何取证结果均需人工复核。
            请仅在获得合法授权的系统和数据上使用本工具。
          </div>
          <div class="row wrap between">
            <span class="mono-sm faint">
              GLog v{version()} · 本地取证控制台 · 结果需人工复核
            </span>
            <span class="mono-sm faint">
              {infoValue()?.os ?? '未知系统'} / {infoValue()?.arch ?? '未知架构'}
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
