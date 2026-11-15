import { ErrorBoundary, Match, Switch, createSignal } from 'solid-js'
import { errorText } from '../api'
import { PageHeader } from '../components/layout/PageHeader'
import { AboutSettings } from '../components/settings/AboutSettings'
import { AISettings } from '../components/settings/AISettings'
import { GeneralSettings } from '../components/settings/GeneralSettings'
import { SSHSettings } from '../components/settings/SSHSettings'
import { TemplateSettings } from '../components/settings/TemplateSettings'
import { Badge, Button, Tabs } from '../components/ui'
import { systemInfo } from '../stores/app'

const TABS = [
  { key: 'general', label: '通用' },
  { key: 'ai', label: 'AI 提供商' },
  { key: 'ssh', label: 'SSH 主机' },
  { key: 'templates', label: '解析模板' },
  { key: 'about', label: '关于' },
]

export function SettingsPage() {
  const [tab, setTab] = createSignal('general')

  return (
    <div class="page">
      <PageHeader
        eyebrow="系统 / 设置"
        title="系统设置"
        subtitle="运行参数、AI 提供商、SSH 主机与解析模板。设置页无需打开项目，所有配置均保存在本机。"
        actions={
          <div class="row wrap">
            <Badge tone="mint">LOCAL-FIRST</Badge>
            <Badge tone="muted">无需项目</Badge>
            <Badge tone="sky">v{systemInfo()?.version ?? '1.0.0'}</Badge>
          </div>
        }
      />

      <div class="panel" style={{ 'margin-bottom': '14px' }}>
        <div style={{ padding: '0 14px' }}>
          <Tabs
            items={TABS}
            active={tab()}
            onChange={(key) => setTab(key)}
          />
        </div>
      </div>

      <ErrorBoundary
        fallback={(err, reset) => (
          <div class="panel">
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                设置页渲染异常
              </div>
            </div>
            <div class="panel-body stack">
              <div class="danger-note">
                当前设置页签渲染失败，其他功能不受影响。错误信息：{errorText(err)}
              </div>
              <div class="row">
                <Button
                  variant="primary"
                  onClick={() => {
                    setTab('general')
                    reset()
                  }}
                >
                  重置到通用设置
                </Button>
              </div>
            </div>
          </div>
        )}
      >
        <Switch fallback={<GeneralSettings />}>
          <Match when={tab() === 'ai'}>
            <AISettings />
          </Match>
          <Match when={tab() === 'ssh'}>
            <SSHSettings />
          </Match>
          <Match when={tab() === 'templates'}>
            <TemplateSettings />
          </Match>
          <Match when={tab() === 'about'}>
            <AboutSettings />
          </Match>
        </Switch>
      </ErrorBoundary>
    </div>
  )
}
