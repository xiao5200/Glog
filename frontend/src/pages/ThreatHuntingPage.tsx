import { Show, createEffect, createSignal } from 'solid-js'
import type { LogEntry } from '../types'
import { api, errorText } from '../api'
import { currentProject, navigate, notify } from '../stores/app'
import { PageHeader } from '../components/layout/PageHeader'
import { Button, EmptyState, Icon, Tabs } from '../components/ui'
import { LogEntryDrawer } from '../components/investigation/LogEntryDrawer'
import { ForensicsPanel } from '../components/threat/ForensicsPanel'
import { HostNetworkPanel } from '../components/threat/HostNetworkPanel'
import { ThreatAIPanel } from '../components/threat/ThreatAIPanel'

type TabKey = 'results' | 'network' | 'forensics'

export function ThreatHuntingPage() {
  const [tab, setTab] = createSignal<TabKey>('results')

  const [logOpen, setLogOpen] = createSignal(false)
  const [logEntry, setLogEntry] = createSignal<LogEntry | null>(null)
  const [logLoading, setLogLoading] = createSignal(false)
  const [logError, setLogError] = createSignal('')
  const [logNote, setLogNote] = createSignal('')
  const [logDocId, setLogDocId] = createSignal(0)

  let logSeq = 0

  const loadLog = async (docId: number) => {
    const project = currentProject()
    if (!project) {
      notify('warn', '请先打开项目', '读取日志详情需要项目上下文。')
      return
    }
    const seq = ++logSeq
    setLogDocId(docId)
    setLogOpen(true)
    setLogLoading(true)
    setLogError('')
    setLogEntry(null)
    try {
      const entry = await api.query.getEntry(project.id, docId)
      if (seq !== logSeq) return
      setLogEntry(entry)
    } catch (err) {
      if (seq !== logSeq) return
      const message = errorText(err)
      setLogError(message)
      notify('error', '加载日志详情失败', message)
    } finally {
      if (seq === logSeq) setLogLoading(false)
    }
  }

  const openLog = (docId: number, entry?: LogEntry, note?: string) => {
    logSeq++
    setLogDocId(docId)
    setLogNote(note ?? '')
    setLogError('')
    setLogOpen(true)
    if (entry) {
      setLogEntry(entry)
      setLogLoading(false)
      return
    }
    void loadLog(docId)
  }

  const closeLog = () => {
    logSeq++
    setLogOpen(false)
    setLogEntry(null)
    setLogError('')
    setLogLoading(false)
    setLogNote('')
  }

  createEffect(() => {
    void (currentProject()?.id ?? '')
    setTab('results')
    closeLog()
  })

  const tabs = () => [
    { key: 'results', label: '检测结果' },
    { key: 'network', label: '网络连接' },
    { key: 'forensics', label: '取证包' },
  ]

  return (
    <div class="page wide">
      <PageHeader
        eyebrow="应急响应 / 威胁狩猎"
        title="威胁狩猎"
        subtitle="由 AI 对日志执行检测并展示检测结果，同时查看实时网络连接暴露面并生成取证包。"
      />

      <Show
        when={currentProject()}
        fallback={
          <div class="panel">
            <div class="panel-body">
              <EmptyState
                icon="crosshair"
                title="尚未打开项目"
                text="威胁狩猎在当前项目日志上执行，请先在项目列表中选择或创建项目。"
                action={
                  <Button variant="primary" onClick={() => navigate('projects')}>
                    <Icon name="folder" size={12} />
                    前往项目列表
                  </Button>
                }
              />
            </div>
          </div>
        }
      >
        {(project) => (
          <>
            <div style={{ 'margin-bottom': '14px' }}>
              <Tabs
                items={tabs()}
                active={tab()}
                onChange={(key) => setTab(key as TabKey)}
              />
            </div>

            <Show when={tab() === 'results'}>
              <ThreatAIPanel
                projectId={project().id}
                onOpenLog={(docId) => openLog(docId)}
              />
            </Show>

            <Show when={tab() === 'network'}>
              <HostNetworkPanel />
            </Show>

            <Show when={tab() === 'forensics'}>
              <ForensicsPanel projectId={project().id} />
            </Show>
          </>
        )}
      </Show>

      <LogEntryDrawer
        open={logOpen()}
        entry={logEntry()}
        loading={logLoading()}
        error={logError()}
        note={logNote()}
        title="命中日志详情"
        onClose={closeLog}
        onRetry={() => {
          if (logDocId()) void loadLog(logDocId())
        }}
      />
    </div>
  )
}
