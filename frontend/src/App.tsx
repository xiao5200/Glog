import { ErrorBoundary, Match, Show, Switch, onMount } from 'solid-js'
import { AppShell, BootScreen } from './components/layout/AppShell'
import { Button, ToastHost } from './components/ui'
import { boot, booting, notify, route } from './stores/app'
import { ProjectListPage } from './pages/ProjectListPage'
import { CollectPage } from './pages/CollectPage'
import { LogViewPage } from './pages/LogViewPage'
import { LogEntryPage } from './pages/LogEntryPage'
import { InvestigationsPage } from './pages/InvestigationsPage'
import { ThreatHuntingPage } from './pages/ThreatHuntingPage'
import { SettingsPage } from './pages/SettingsPage'
import { errorText } from './api'

export function App() {
  onMount(() => {
    void boot()
  })

  return (
    <ErrorBoundary
      fallback={(err, reset) => (
        <div class="boot-screen" style={{ padding: '40px' }}>
          <div class="panel" style={{ 'max-width': '680px', width: '100%' }}>
            <div class="panel-head">
              <div class="panel-title">
                <span class="tick" />
                界面渲染异常
              </div>
            </div>
            <div class="panel-body stack">
              <p class="muted" style={{ 'font-size': '12px' }}>
                前端捕获到未处理错误。可尝试重试；若持续发生，请查看 data/logs 下的日志。
              </p>
              <pre class="code-block">{errorText(err)}</pre>
              <div class="row">
                <Button variant="primary" onClick={() => reset()}>
                  重试渲染
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    notify('info', '提示', '如问题持续，请重启 GLog。')
                  }}
                >
                  反馈
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    >
      <Show when={!booting()} fallback={<BootScreen />}>
        <AppShell>
          {/* 日志工作台常驻挂载：进入单条日志独立页面再返回时保留查询、筛选与滚动状态。 */}
          <div style={{ display: route() === 'logs' ? 'contents' : 'none' }}>
            <LogViewPage />
          </div>
          <Switch>
            <Match when={route() === 'projects'}>
              <ProjectListPage />
            </Match>
            <Match when={route() === 'collect'}>
              <CollectPage />
            </Match>
            <Match when={route() === 'log-detail'}>
              <LogEntryPage />
            </Match>
            <Match when={route() === 'investigations'}>
              <InvestigationsPage />
            </Match>
            <Match when={route() === 'threat'}>
              <ThreatHuntingPage />
            </Match>
            <Match when={route() === 'settings'}>
              <SettingsPage />
            </Match>
          </Switch>
        </AppShell>
      </Show>
      <ToastHost />
    </ErrorBoundary>
  )
}
