import { JSX, Show, createEffect, createSignal, on } from 'solid-js'
import type { Query } from '../../types'
import { notify } from '../../stores/app'
import { Button, Field, Modal, TextArea, TextInput } from '../ui'
import { formatInt } from '../../utils'

export interface SnapshotModalProps {
  open: boolean
  query: Query
  resultCount: number
  defaultName: string
  saving: boolean
  error: string | null
  onClose: () => void
  onSave: (name: string, description: string) => void
}

export function SnapshotModal(props: SnapshotModalProps): JSX.Element {
  const [name, setName] = createSignal('')
  const [description, setDescription] = createSignal('')

  createEffect(
    on(
      () => props.open,
      (open) => {
        if (!open) return
        setName(props.defaultName)
        setDescription('')
      },
    ),
  )

  const querySummary = () => {
    const query = props.query
    const parts: string[] = []
    parts.push(query.text?.trim() ? `文本：${query.text.trim()}` : '文本：全部日志')
    if (query.levels?.length) parts.push(`级别：${query.levels.join(' / ')}`)
    if (query.hosts?.length) parts.push(`主机：${query.hosts.length} 个`)
    if (query.sources?.length) parts.push(`日志源：${query.sources.length} 个`)
    if (query.event_ids?.length) parts.push(`事件 ID：${query.event_ids.join(', ')}`)
    if (query.field_filters?.length) parts.push(`字段条件：${query.field_filters.length} 个`)
    if (query.start_time || query.end_time) parts.push('时间范围：已限定')
    return parts.join(' · ')
  }

  function submit() {
    if (props.saving) return
    const value = name().trim()
    if (!value) {
      notify('warn', '请输入快照名称', '名称为空时无法保存查询快照。')
      return
    }
    props.onSave(value, description().trim())
  }

  return (
    <Modal
      open={props.open}
      title="保存查询快照"
      subtitle="保存当前查询条件，便于后续快速恢复分析现场"
      size="md"
      onClose={props.onClose}
      footer={
        <>
          <Button size="sm" variant="ghost" onClick={props.onClose} disabled={props.saving}>
            取消
          </Button>
          <Button
            size="sm"
            variant="primary"
            loading={props.saving}
            disabled={!name().trim()}
            onClick={submit}
          >
            保存快照
          </Button>
        </>
      }
    >
      <div class="form-stack" style={{ gap: '12px' }}>
        <Show when={props.error}>
          <div class="danger-note">{props.error}</div>
        </Show>

        <Field label="快照名称" hint="必填">
          <TextInput
            value={name()}
            placeholder="例如：web-01 错误登录排查"
            autofocus
            disabled={props.saving}
            onInput={setName}
            onEnter={submit}
          />
        </Field>

        <Field label="描述" hint="可选">
          <TextArea
            value={description()}
            rows={3}
            placeholder="记录本次查询目的、关注点或排查结论…"
            disabled={props.saving}
            onInput={setDescription}
          />
        </Field>

        <div>
          <div class="mono-sm faint" style={{ 'margin-bottom': '3px' }}>
            快照内容
          </div>
          <div class="detail-note" style={{ 'line-height': '1.7', 'word-break': 'break-word' }}>
            {querySummary()}
          </div>
          <div class="mono-sm faint" style={{ 'margin-top': '5px' }}>
            当前结果 {formatInt(props.resultCount)} 条日志。快照仅保存查询条件与结果数量，不复制日志数据。
          </div>
        </div>
      </div>
    </Modal>
  )
}
