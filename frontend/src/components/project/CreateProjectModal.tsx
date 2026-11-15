import { For, JSX, Show, createEffect, createMemo, createSignal } from 'solid-js'
import type { Project } from '../../types'
import { notify } from '../../stores/app'
import {
  Button,
  Field,
  Modal,
  Select,
  TextArea,
  TextInput,
} from '../ui'

export interface ProjectFormValues {
  name: string
  description: string
  tags: string[]
  priority: string
}

const PRIORITY_OPTIONS = [
  { value: 'high', label: '高 / high' },
  { value: 'medium', label: '中 / medium' },
  { value: 'low', label: '低 / low' },
]

interface ProjectFormModalProps {
  mode: 'create' | 'edit'
  open: boolean
  initial?: Project | null
  busy?: boolean
  error?: string
  onClose: () => void
  onSubmit: (values: ProjectFormValues) => void
}

function ProjectFormModal(props: ProjectFormModalProps): JSX.Element {
  const [name, setName] = createSignal('')
  const [description, setDescription] = createSignal('')
  const [tagsText, setTagsText] = createSignal('')
  const [priority, setPriority] = createSignal('medium')
  const [localError, setLocalError] = createSignal('')
  let wasOpen = false

  const parsedTags = createMemo(() => {
    const seen = new Set<string>()
    return tagsText()
      .split(/[,，]/)
      .map((tag) => tag.trim())
      .filter((tag) => {
        if (!tag || seen.has(tag)) return false
        seen.add(tag)
        return true
      })
  })

  createEffect(() => {
    if (props.open && !wasOpen) {
      const initial = props.initial
      setName(initial?.name ?? '')
      setDescription(initial?.description ?? '')
      setTagsText(Array.isArray(initial?.tags) ? initial.tags.join(', ') : '')
      const initialPriority = String(initial?.priority ?? 'medium').toLowerCase()
      setPriority(
        initialPriority === 'high' || initialPriority === 'low'
          ? initialPriority
          : 'medium',
      )
      setLocalError('')
    }
    wasOpen = props.open
  })

  const close = () => {
    if (!props.busy) props.onClose()
  }

  const submit = () => {
    if (!props.open || props.busy) return
    const trimmedName = name().trim()
    if (!trimmedName) {
      const message = '请输入项目名称。'
      setLocalError(message)
      notify('error', props.mode === 'create' ? '无法创建项目' : '无法保存项目', message)
      return
    }
    setLocalError('')
    props.onSubmit({
      name: trimmedName,
      description: description().trim(),
      tags: parsedTags(),
      priority: priority(),
    })
  }

  return (
    <Modal
      open={props.open}
      title={props.mode === 'create' ? '创建项目档案' : '编辑项目档案'}
      subtitle="PROJECT REGISTRY / WORKSPACE ENTRY"
      size="md"
      onClose={close}
      footer={
        <>
          <Button variant="ghost" disabled={props.busy} onClick={close}>
            取消
          </Button>
          <Button variant="primary" loading={props.busy} onClick={submit}>
            {props.mode === 'create' ? '创建项目' : '保存修改'}
          </Button>
        </>
      }
    >
      <div class="form-stack">
        <Show when={props.error}>
          <div class="danger-note">{props.error}</div>
        </Show>

        <Field label="项目名称" hint="必填" error={localError() || undefined}>
          <TextInput
            value={name()}
            placeholder="例如：生产集群安全审计"
            disabled={props.busy}
            autofocus={props.open}
            onInput={(value) => {
              setName(value)
              if (localError()) setLocalError('')
            }}
            onEnter={submit}
          />
        </Field>

        <Field label="项目描述" hint="可选">
          <TextArea
            value={description()}
            rows={3}
            placeholder="记录项目范围、数据来源或调查目标…"
            disabled={props.busy}
            onInput={(value) => setDescription(value)}
          />
        </Field>

        <Field label="标签" hint="使用逗号分隔">
          <TextInput
            value={tagsText()}
            placeholder="例如：安全审计, 生产环境, Windows"
            disabled={props.busy}
            onInput={(value) => setTagsText(value)}
            onEnter={submit}
          />
          <Show when={parsedTags().length > 0}>
            <div class="row wrap" style={{ gap: '4px', 'margin-top': '6px' }}>
              <For each={parsedTags()}>{(tag) => <span class="tag">#{tag}</span>}</For>
            </div>
          </Show>
        </Field>

        <Field label="优先级" hint="卡片左侧信号条">
          <Select
            value={priority()}
            options={PRIORITY_OPTIONS}
            disabled={props.busy}
            onInput={(value) => setPriority(value)}
          />
        </Field>

        <Show when={props.mode === 'edit'}>
          <div class="detail-note">仅修改项目元数据，日志、索引与调查证据不会被改动。</div>
        </Show>
      </div>
    </Modal>
  )
}

export function CreateProjectModal(props: {
  open: boolean
  busy?: boolean
  error?: string
  onClose: () => void
  onSubmit: (values: ProjectFormValues) => void
}): JSX.Element {
  return (
    <ProjectFormModal
      mode="create"
      open={props.open}
      busy={props.busy}
      error={props.error}
      onClose={props.onClose}
      onSubmit={props.onSubmit}
    />
  )
}

export function EditProjectModal(props: {
  open: boolean
  project: Project | null
  busy?: boolean
  error?: string
  onClose: () => void
  onSubmit: (values: ProjectFormValues) => void
}): JSX.Element {
  return (
    <ProjectFormModal
      mode="edit"
      open={props.open}
      initial={props.project}
      busy={props.busy}
      error={props.error}
      onClose={props.onClose}
      onSubmit={props.onSubmit}
    />
  )
}
