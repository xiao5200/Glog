import { createEffect, createSignal } from 'solid-js'
import type { Investigation, InvestigationUpdates, TimeRange } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { shortId } from '../../utils'
import { Button, Field, Icon, Modal, Select, TextArea, TextInput } from '../ui'
import { localInputToIso, toLocalInput } from '../log/logCommon'
import { CLUE_TYPES } from './meta'

export interface EditInvestigationModalProps {
  open: boolean
  investigation: Investigation
  onClose: () => void
  onChanged: (investigation: Investigation) => void
}

const STATUS_OPTIONS = [
  { value: 'open', label: '待处理' },
  { value: 'analyzing', label: '分析中' },
  { value: 'closed', label: '已结案' },
]

export function EditInvestigationModal(props: EditInvestigationModalProps) {
  const [title, setTitle] = createSignal('')
  const [description, setDescription] = createSignal('')
  const [clueType, setClueType] = createSignal('message')
  const [clueValue, setClueValue] = createSignal('')
  const [timeStart, setTimeStart] = createSignal('')
  const [timeEnd, setTimeEnd] = createSignal('')
  const [tags, setTags] = createSignal('')
  const [risk, setRisk] = createSignal('')
  const [assignedTo, setAssignedTo] = createSignal('')
  const [status, setStatus] = createSignal('open')
  const [busy, setBusy] = createSignal(false)
  const [formError, setFormError] = createSignal('')

  const fill = () => {
    const inv = props.investigation
    setTitle(inv.title ?? '')
    setDescription(inv.description ?? '')
    setClueType(inv.clue_type || 'message')
    setClueValue(inv.clue_value ?? '')
    // 后端允许时间范围只设置一端，未设置的一端会序列化成零值时间（0001 年），
    // 这里按“未填写”处理，避免 datetime-local 显示公元 1 年。
    const safeLocal = (value?: string) =>
      value && !value.startsWith('0001-') ? toLocalInput(value) : ''
    setTimeStart(safeLocal(inv.time_range?.start))
    setTimeEnd(safeLocal(inv.time_range?.end))
    setTags((inv.tags ?? []).join(', '))
    setRisk(inv.risk_score != null ? String(inv.risk_score) : '')
    setAssignedTo(inv.assigned_to ?? '')
    setStatus(inv.status || 'open')
    setBusy(false)
    setFormError('')
  }

  createEffect(() => {
    if (props.open) fill()
  })

  const submit = async () => {
    if (busy()) return
    const trimmedTitle = title().trim()
    if (!trimmedTitle) {
      setFormError('请输入工单标题。')
      return
    }

    const rawRisk = risk().trim()
    let score: number | undefined
    if (rawRisk) {
      const parsed = Number(rawRisk)
      if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) {
        setFormError('风险分需为 0 - 100 之间的数字。')
        return
      }
      score = Math.round(parsed)
    }

    const startInput = timeStart().trim()
    const endInput = timeEnd().trim()
    let timeRange: TimeRange | undefined
    let clearTimeRange = false
    if (!startInput && !endInput) {
      // 原工单存在时间范围而输入被清空时，显式通知后端移除。
      clearTimeRange = Boolean(props.investigation.time_range)
    } else if (!startInput || !endInput) {
      setFormError('请同时填写开始与结束时间')
      return
    } else {
      const startIso = localInputToIso(startInput)
      const endIso = localInputToIso(endInput)
      if (!startIso || !endIso) {
        setFormError('时间范围格式无效，请重新选择。')
        return
      }
      if (new Date(startIso).getTime() > new Date(endIso).getTime()) {
        setFormError('开始时间不能晚于结束时间。')
        return
      }
      timeRange = { start: startIso, end: endIso }
    }

    const updates: InvestigationUpdates = {
      title: trimmedTitle,
      description: description().trim(),
      status: status(),
      clue_type: clueType() || 'message',
      clue_value: clueValue().trim(),
      tags: tags()
        .split(/[,，;；]/)
        .map((item) => item.trim())
        .filter(Boolean),
      assigned_to: assignedTo().trim(),
    }
    if (timeRange) updates.time_range = timeRange
    if (clearTimeRange) updates.clear_time_range = true
    if (score != null) {
      updates.risk_score = score
    } else if (props.investigation.risk_score != null) {
      updates.clear_risk_score = true
    }

    setBusy(true)
    setFormError('')
    try {
      const updated = await api.investigations.update(props.investigation.id, updates)
      notify('success', '工单已更新', updated.title)
      props.onChanged(updated)
      props.onClose()
    } catch (err) {
      const message = errorText(err)
      setFormError(message)
      notify('error', '更新工单失败', message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={props.open}
      title="编辑调查工单"
      subtitle={`工单 ${shortId(props.investigation.id, 12)}`}
      size="lg"
      onClose={() => {
        if (!busy()) props.onClose()
      }}
      footer={
        <>
          <Button variant="ghost" disabled={busy()} onClick={props.onClose}>
            取消
          </Button>
          <Button variant="primary" loading={busy()} onClick={() => void submit()}>
            <Icon name="save" size={12} />
            保存修改
          </Button>
        </>
      }
    >
      <div class="form-stack">
        <Field label="标题" error={formError()}>
          <TextInput value={title()} onInput={setTitle} />
        </Field>

        <Field label="描述">
          <TextArea value={description()} rows={4} onInput={setDescription} />
        </Field>

        <div class="form-row">
          <Field label="线索类型">
            <Select
              value={clueType()}
              options={CLUE_TYPES}
              disabled={busy()}
              onInput={setClueType}
            />
          </Field>
          <Field label="线索值" hint="可作为检索起点">
            <TextInput
              value={clueValue()}
              placeholder="例如：10.20.30.40"
              mono
              disabled={busy()}
              onInput={setClueValue}
            />
          </Field>
        </div>

        <div class="form-row">
          <Field label="开始时间" hint="可选">
            <TextInput
              type="datetime-local"
              value={timeStart()}
              disabled={busy()}
              onInput={setTimeStart}
            />
          </Field>
          <Field label="结束时间" hint="可选">
            <TextInput
              type="datetime-local"
              value={timeEnd()}
              disabled={busy()}
              onInput={setTimeEnd}
            />
          </Field>
        </div>

        <div class="row between wrap" style={{ gap: '8px', 'align-items': 'center' }}>
          <span class="mono-sm faint">时间范围可留空，留空表示不限制调查时间范围。</span>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy() || (!timeStart() && !timeEnd())}
            onClick={() => {
              setTimeStart('')
              setTimeEnd('')
            }}
          >
            <Icon name="x" size={11} />
            清空时间范围
          </Button>
        </div>

        <div class="form-row">
          <Field label="状态">
            <Select value={status()} options={STATUS_OPTIONS} onInput={setStatus} />
          </Field>
          <Field label="风险分" hint="0 - 100">
            <TextInput type="number" value={risk()} onInput={setRisk} />
          </Field>
          <Field label="处理人">
            <TextInput value={assignedTo()} placeholder="留空表示未指派" onInput={setAssignedTo} />
          </Field>
        </div>

        <Field label="标签" hint="逗号分隔">
          <TextInput value={tags()} placeholder="例如：凭据访问, webshell" onInput={setTags} />
        </Field>

        <div class="detail-note">
          将状态改为“已结案”时会记录结案时间；改回待处理/分析中会清除结案时间。
        </div>
      </div>
    </Modal>
  )
}
