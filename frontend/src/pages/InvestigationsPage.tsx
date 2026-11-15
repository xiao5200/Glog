import { Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { Investigation } from '../types'
import { api, errorText } from '../api'
import { currentProject, navigate, notify } from '../stores/app'
import { PageHeader } from '../components/layout/PageHeader'
import { Button, EmptyState, Icon, StatTile } from '../components/ui'
import { CreateInvestigationModal } from '../components/investigation/CreateInvestigationModal'
import { InvestigationDetail } from '../components/investigation/InvestigationDetail'
import { InvestigationList } from '../components/investigation/InvestigationList'

export function InvestigationsPage() {
  const [investigations, setInvestigations] = createSignal<Investigation[]>([])
  const [selectedId, setSelectedId] = createSignal<string | undefined>(undefined)
  const [loading, setLoading] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  const [createOpen, setCreateOpen] = createSignal(false)

  let loadSeq = 0

  const load = async (projectID: string) => {
    if (!projectID) return
    const seq = ++loadSeq
    setLoading(true)
    setLoadError('')
    try {
      const list = await api.investigations.list(projectID)
      if (seq !== loadSeq) return
      const safe = Array.isArray(list) ? list : []
      setInvestigations(safe)
      const current = selectedId()
      if (!current || !safe.some((item) => item.id === current)) {
        // 当前选择无效时自动选中：优先未结案工单，否则取第一项。
        const preferred =
          safe.find((item) => {
            const status = String(item.status ?? '').toLowerCase()
            return status === 'open' || status === 'analyzing'
          }) ?? safe[0]
        setSelectedId(preferred?.id)
      }
    } catch (err) {
      if (seq !== loadSeq) return
      const message = errorText(err)
      setLoadError(message)
      notify('error', '加载调查工单失败', message)
    } finally {
      if (seq === loadSeq) setLoading(false)
    }
  }

  createEffect(() => {
    const projectID = currentProject()?.id ?? ''
    if (!projectID) {
      loadSeq++
      setInvestigations([])
      setSelectedId(undefined)
      setLoading(false)
      setLoadError('')
      return
    }
    setSelectedId(undefined)
    void load(projectID)
  })

  const selected = createMemo(() =>
    investigations().find((item) => item.id === selectedId()),
  )

  const stats = createMemo(() => {
    let open = 0
    let analyzing = 0
    let closed = 0
    let evidence = 0
    for (const item of investigations()) {
      const status = String(item.status ?? '').toLowerCase()
      if (status === 'open') open += 1
      if (status === 'analyzing') analyzing += 1
      if (status === 'closed') closed += 1
      evidence += item.evidence_ids?.length ?? 0
    }
    return { total: investigations().length, open, analyzing, closed, evidence }
  })

  const refresh = () => {
    const projectID = currentProject()?.id
    if (!projectID) {
      notify('warn', '请先打开项目', '调查工单依赖当前项目上下文。')
      return
    }
    void load(projectID)
  }

  const replaceInvestigation = (next: Investigation) => {
    setInvestigations((prev) => prev.map((item) => (item.id === next.id ? next : item)))
  }

  const handleCreated = (created: Investigation) => {
    setInvestigations((prev) => [created, ...prev.filter((item) => item.id !== created.id)])
    setSelectedId(created.id)
  }

  const handleDeleted = (id: string) => {
    const target = investigations().find((item) => item.id === id)
    setInvestigations((prev) => prev.filter((item) => item.id !== id))
    setSelectedId(undefined)
    notify('success', '工单已删除', target?.title || undefined)
    const projectID = currentProject()?.id
    if (projectID) void load(projectID)
  }

  return (
    <div class="page wide">
      <PageHeader
        eyebrow="应急响应 / 调查工单"
        title="调查工单"
        subtitle="聚合线索、证据链、调查时间线与 AI 研判，驱动可追溯的响应流程。"
        actions={
          <>
            <Button
              variant="primary"
              disabled={!currentProject()}
              onClick={() => setCreateOpen(true)}
            >
              <Icon name="plus" size={12} />
              新建工单
            </Button>
            <Button
              loading={loading()}
              disabled={!currentProject()}
              onClick={refresh}
            >
              <Icon name="refresh" size={12} />
              刷新
            </Button>
          </>
        }
      />

      <Show
        when={currentProject()}
        fallback={
          <div class="panel">
            <div class="panel-body">
              <EmptyState
                icon="database"
                title="尚未打开项目"
                text="调查工单与证据链依赖项目上下文，请先在项目列表中选择或创建项目。"
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
            <div class="stat-grid" style={{ 'margin-bottom': '14px' }}>
              <StatTile
                label="工单总数"
                value={stats().total}
                note="当前项目全部调查工单"
              />
              <StatTile
                label="OPEN / 待处理"
                value={stats().open}
                note="等待研判与处置"
                tone="sky"
              />
              <StatTile
                label="ANALYZING / 分析中"
                value={stats().analyzing}
                note="正在补充证据或 AI 研判"
                tone="amber"
              />
              <StatTile
                label="CLOSED / 已结案"
                value={stats().closed}
                note="调查流程已关闭"
                tone="mint"
              />
              <StatTile
                label="证据总数"
                value={stats().evidence}
                note="已标记日志条目"
              />
            </div>

            <div class="split wide-left">
              <InvestigationList
                investigations={investigations()}
                selectedId={selectedId()}
                loading={loading()}
                error={loadError()}
                onSelect={(item) => setSelectedId(item.id)}
                onRetry={refresh}
              />

              <Show
                when={selected()}
                fallback={
                  <div class="panel">
                    <div class="panel-body">
                      <EmptyState
                        icon="shield-check"
                        title="未选择工单"
                        text="从左侧队列选择工单查看证据链与研判时间线，或新建工单开始调查。"
                        action={
                          <Button variant="primary" onClick={() => setCreateOpen(true)}>
                            <Icon name="plus" size={12} />
                            新建工单
                          </Button>
                        }
                      />
                    </div>
                  </div>
                }
              >
                {(investigation) => (
                  <InvestigationDetail
                    investigation={investigation()}
                    projectId={project().id}
                    onChanged={replaceInvestigation}
                    onDeleted={handleDeleted}
                  />
                )}
              </Show>
            </div>

            <CreateInvestigationModal
              open={createOpen()}
              projectId={project().id}
              onClose={() => setCreateOpen(false)}
              onCreated={handleCreated}
            />
          </>
        )}
      </Show>
    </div>
  )
}
