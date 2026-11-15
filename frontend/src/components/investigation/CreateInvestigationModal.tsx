import { createEffect, createSignal } from 'solid-js'
import type { CreateInvestigationRequest, Investigation, TimeRange } from '../../types'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { Button, Field, Icon, Modal, Select, TextArea, TextInput } from '../ui'
import { localInputToIso } from '../log/logCommon'
import { CLUE_TYPES } from './meta'

export interface CreateInvestigationModalProps {
  open: boolean
  projectId: string
  onClose: () => void
  onCreated: (investigation: Investigation) => void
}

function cluePlaceholder(type: string): string {
  switch (type) {
    case 'ip':
      return '例如：10.20.30.40'
    case 'user':
      return '例如：svc_backup'
    case 'pid':
      return '例如：4812'
    case 'file':
      return '例如：C:\\Windows\\Temp\\payload.exe'
    case 'domain':
      return '例如：evil.example.com'
    case 'event_id':
      return '例如：4625'
    case 'log_entry':
      return '例如：120（DocID）'
    default:
      return '例如：failed password for root'
  }
}

/** 解析逗号/空格分隔的 DocID，返回去重后的 ID 与非法令牌。 */
function parseDocIDs(text: string): { ids: number[]; invalid: string[] } {
  const ids: number[] = []
  const invalid: string[] = []
  const seen = new Set<number>()
  for (const token of text.split(/[\s,;，、]+/)) {
    if (!token) continue
    if (!/^\d+$/.test(token)) {
      invalid.push(token)
      continue
    }
    const value = Number(token)
    if (!Number.isSafeInteger(value) || value < 0) {
      invalid.push(token)
      continue
    }
    if (seen.has(value)) continue
    seen.add(value)
    ids.push(value)
  }
  return { ids, invalid }
}

export function CreateInvestigationModal(props: CreateInvestigationModalProps) {
  const [title, setTitle] = createSignal('')
  const [description, setDescription] = createSignal('')
  const [clueType, setClueType] = createSignal('message')
  const [clueValue, setClueValue] = createSignal('')
  const [timeStart, setTimeStart] = createSignal('')
  const [timeEnd, setTimeEnd] = createSignal('')
  const [docIDs, setDocIDs] = createSignal('')
  const [tags, setTags] = createSignal('')
  const [risk, setRisk] = createSignal('')
  const [assignedTo, setAssignedTo] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [formError, setFormError] = createSignal('')

  const reset = () => {
    setTitle('')
    setDescription('')
    setClueType('message')
    setClueValue('')
    setTimeStart('')
    setTimeEnd('')
    setDocIDs('')
    setTags('')
    setRisk('')
    setAssignedTo('')
    setBusy(false)
    setFormError('')
  }

  createEffect(() => {
    if (props.open) reset()
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
    if (startInput || endInput) {
      if (!startInput || !endInput) {
        setFormError('请同时填写开始与结束时间')
        return
      }
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

    const parsedDocIDs = parseDocIDs(docIDs())
    if (parsedDocIDs.invalid.length > 0) {
      setFormError(
        `关联 DocID 含无效值“${parsedDocIDs.invalid[0]}”，请使用逗号或空格分隔的非负整数。`,
      )
      return
    }

    const req: CreateInvestigationRequest = {
      project_id: props.projectId,
      title: trimmedTitle,
      description: description().trim(),
      clue_type: clueType(),
      clue_value: clueValue().trim(),
      tags: tags()
        .split(/[,，;；]/)
        .map((item) => item.trim())
        .filter(Boolean),
      risk_score: score,
      time_range: timeRange,
      doc_ids: parsedDocIDs.ids.length > 0 ? parsedDocIDs.ids : undefined,
      assigned_to: assignedTo().trim() || undefined,
    }

    setBusy(true)
    setFormError('')
    try {
      const created = await api.investigations.create(req)
      notify('success', '工单已创建', created.title)
      props.onCreated(created)
      props.onClose()
    } catch (err) {
      const message = errorText(err)
      setFormError(message)
      notify('error', '创建工单失败', message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={props.open}
      title="新建调查工单"
      subtitle="登记线索并建立可追溯的证据链"
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
            <Icon name="plus" size={12} />
            创建工单
          </Button>
        </>
      }
    >
      <div class="form-stack">
        <Field label="标题" error={formError()}>
          <TextInput
            value={title()}
            placeholder="简要描述调查主题，例如：异常登录与提权行为"
            autofocus
            onInput={setTitle}
          />
        </Field>

        <div class="form-row">
          <Field label="线索类型">
            <Select value={clueType()} options={CLUE_TYPES} onInput={setClueType} />
          </Field>
          <Field label="线索值" hint="可作为检索起点">
            <TextInput
              value={clueValue()}
              placeholder={cluePlaceholder(clueType())}
              mono
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

        <Field label="关联 DocID" hint="逗号或空格分隔，可选">
          <TextInput
            value={docIDs()}
            placeholder="例如：120, 125, 131"
            mono
            disabled={busy()}
            onInput={setDocIDs}
          />
        </Field>

        <Field label="描述" hint="调查背景、现象与初步判断">
          <TextArea
            value={description()}
            rows={4}
            placeholder="记录事件背景、已观察到的异常现象与调查目标…"
            onInput={setDescription}
          />
        </Field>

        <div class="form-row">
          <Field label="标签" hint="逗号分隔">
            <TextInput
              value={tags()}
              placeholder="横向移动, 凭据访问, linux"
              onInput={setTags}
            />
          </Field>
          <Field label="风险分" hint="0 - 100">
            <TextInput
              type="number"
              value={risk()}
              placeholder="可选，例如：75"
              onInput={setRisk}
            />
          </Field>
          <Field label="处理人">
            <TextInput
              value={assignedTo()}
              placeholder="可选，例如：张工"
              onInput={setAssignedTo}
            />
          </Field>
        </div>

        <div class="detail-note">
          创建后可在工单详情中标记日志证据、运行 AI 研判并导出证据包。证据需先在日志分析页标记。
        </div>
      </div>
    </Modal>
  )
}
