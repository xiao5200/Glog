import { For, JSX, Show, createEffect, createSignal, onCleanup, onMount } from 'solid-js'
import { Portal } from 'solid-js/web'
import { dismissToast, toasts, type ToastKind } from '../stores/app'
import { copyText } from '../utils'

export type IconName =
  | 'folder'
  | 'database'
  | 'search'
  | 'shield'
  | 'shield-check'
  | 'settings'
  | 'activity'
  | 'upload'
  | 'download'
  | 'plus'
  | 'x'
  | 'trash'
  | 'copy'
  | 'refresh'
  | 'play'
  | 'stop'
  | 'chevron-down'
  | 'chevron-right'
  | 'arrow-right'
  | 'clock'
  | 'filter'
  | 'file'
  | 'cpu'
  | 'terminal'
  | 'crosshair'
  | 'alert'
  | 'check'
  | 'info'
  | 'external'
  | 'book'
  | 'layers'
  | 'zap'
  | 'eye'
  | 'save'
  | 'server'
  | 'lock'
  | 'user'
  | 'globe'
  | 'hash'
  | 'calendar'
  | 'wand'
  | 'list'
  | 'package'
  | 'branch'
  | 'chart'
  | 'menu'
  | 'edit'
  | 'chevron-left'
  | 'chevron-up'
  | 'arrow-left'
  | 'more'
  | 'key'
  | 'tag'
  | 'sliders'
  | 'wifi'
  | 'sun'
  | 'moon'

const ICON_PATHS: Record<IconName, string> = {
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h6a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  database:
    'M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3zM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.35-4.35',
  shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z',
  'shield-check': 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6zM8.5 12l2.5 2.5 4.5-5',
  settings:
    'M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7zM19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.1 2.1-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6V21h-3v-.2a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L6.6 17.5l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H5v-3h.4a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1 2.1-2.1.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6V5h3v.2a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1 2.1 2.1-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.4v3h-.4a1.7 1.7 0 0 0-1.6 1z',
  activity: 'M3 12h4l2 7 4-16 2 9h6',
  upload: 'M12 16V4m-5 5 5-5 5 5M4 20h16',
  download: 'M12 4v12m-5-5 5 5 5-5M4 20h16',
  plus: 'M12 5v14M5 12h14',
  x: 'M6 6l12 12M18 6L6 18',
  trash: 'M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14M10 11v6M14 11v6',
  copy: 'M9 9h11v11H9zM4 15V4h11',
  refresh: 'M21 12a9 9 0 1 1-2.64-6.36M21 4v5h-5',
  play: 'M7 4l13 8-13 8z',
  stop: 'M6 6h12v12H6z',
  'chevron-down': 'M6 9l6 6 6-6',
  'chevron-right': 'M9 6l6 6-6 6',
  'arrow-right': 'M4 12h16m-6-6 6 6-6 6',
  clock: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2',
  filter: 'M3 5h18l-7 8v6l-4 2v-8z',
  file: 'M6 2h8l4 4v16H6zM14 2v5h5',
  cpu: 'M6 6h12v12H6zM9 9h6v6H9zM9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4',
  terminal: 'M4 4h16v16H4zM7 9l3 3-3 3M13 15h4',
  crosshair:
    'M12 2v4M12 18v4M2 12h4M18 12h4M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  alert: 'M12 3 2 21h20zM12 9v5M12 17h.01',
  check: 'M4 12l5 5L20 6',
  info: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 11v6M12 7h.01',
  external: 'M14 3h7v7M21 3l-9 9M10 5H5v14h14v-5',
  book: 'M4 4h7a3 3 0 0 1 3 3v13a3 3 0 0 0-3-3H4zM20 4h-3a3 3 0 0 0-3 3v13a3 3 0 0 1 3-3h3z',
  layers: 'M12 2 2 7l10 5 10-5zM2 12l10 5 10-5M2 17l10 5 10-5',
  zap: 'M13 2 3 14h7l-1 8 10-12h-7z',
  eye: 'M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  save: 'M5 3h11l3 3v15H5zM8 3v6h8V3M8 21v-7h8v7',
  server: 'M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01',
  lock: 'M6 10h12v11H6zM8 10V7a4 4 0 0 1 8 0v3',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21c0-4 3.6-7 8-7s8 3 8 7',
  globe: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM2 12h20M12 2c3 3 4 6 4 10s-1 7-4 10c-3-3-4-6-4-10s1-7 4-10z',
  hash: 'M4 9h16M4 15h16M10 3 8 21M16 3l-2 18',
  calendar: 'M4 5h16v16H4zM4 9h16M8 2v6M16 2v6',
  wand: 'M15 4l1 3 3 1-3 1-1 3-1-3-3-1 3-1zM4 20l9-9M14 14l6 6',
  list: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  package: 'M12 2 2 7l10 5 10-5-10-5zM2 7v10l10 5 10-5V7M12 12v10',
  branch: 'M6 3v12M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 9c0 4-6 3-6 7v2',
  chart: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
  menu: 'M4 6h16M4 12h16M4 18h16',
  edit: 'M4 20h4L20 8l-4-4L4 16zM14 6l4 4',
  'chevron-left': 'M15 6l-6 6 6 6',
  'chevron-up': 'M6 15l6-6 6 6',
  'arrow-left': 'M20 12H4m6-6-6 6 6 6',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  key: 'M15 7a4 4 0 1 0-3 3.9L4 19v2h3v-2h2v-2h2l2.1-2.1A4 4 0 0 0 15 7zM16 7h.01',
  tag: 'M20 12l-8 8-8-8V4h8zM7.5 7.5h.01',
  sliders: 'M4 6h16M4 12h16M4 18h16M8 4v4M16 10v4M10 16v4',
  wifi: 'M5 12a10 10 0 0 1 14 0M8.5 15.5a5 5 0 0 1 7 0M12 19h.01',
  sun: 'M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6L17 7M7 17l-1.4 1.4',
  moon: 'M20 14.5A8.6 8.6 0 0 1 9.5 4a8.6 8.6 0 1 0 10.5 10.5z',
}

export function Icon(props: { name: IconName; size?: number; class?: string }): JSX.Element {
  return (
    <svg
      width={props.size ?? 14}
      height={props.size ?? 14}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      stroke-linejoin="round"
      class={props.class}
      aria-hidden="true"
    >
      <path d={ICON_PATHS[props.name]} />
    </svg>
  )
}

export function Spinner(props: { large?: boolean }): JSX.Element {
  return <span class={props.large ? 'spinner lg' : 'spinner'} aria-label="加载中" />
}

type ButtonVariant = 'default' | 'primary' | 'danger' | 'ghost'

export interface ButtonProps {
  variant?: ButtonVariant
  size?: 'sm' | 'md' | 'lg'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  class?: string
  title?: string
  onClick?: (e: MouseEvent) => void
  children?: JSX.Element
}

export function Button(props: ButtonProps): JSX.Element {
  const cls = () =>
    [
      'btn',
      props.variant === 'primary' ? 'btn-primary' : '',
      props.variant === 'danger' ? 'btn-danger' : '',
      props.variant === 'ghost' ? 'btn-ghost' : '',
      props.size === 'sm' ? 'btn-sm' : '',
      props.size === 'lg' ? 'btn-lg' : '',
      props.class ?? '',
    ]
      .filter(Boolean)
      .join(' ')
  return (
    <button
      type={props.type ?? 'button'}
      class={cls()}
      disabled={props.disabled || props.loading}
      title={props.title}
      onClick={(e) => props.onClick?.(e)}
    >
      <Show when={props.loading}>
        <Spinner />
      </Show>
      {props.children}
    </button>
  )
}

export function IconButton(props: {
  icon: IconName
  title: string
  onClick?: (e: MouseEvent) => void
  disabled?: boolean
  class?: string
}): JSX.Element {
  return (
    <button
      type="button"
      class={`icon-btn ${props.class ?? ''}`}
      title={props.title}
      aria-label={props.title}
      disabled={props.disabled}
      onClick={(e) => props.onClick?.(e)}
    >
      <Icon name={props.icon} size={14} />
    </button>
  )
}

export function Badge(props: {
  tone?: 'mint' | 'amber' | 'ember' | 'sky' | 'violet' | 'muted' | 'default'
  children: JSX.Element
  class?: string
}): JSX.Element {
  const tone = () => (props.tone && props.tone !== 'default' ? `badge-${props.tone}` : '')
  return <span class={`badge ${tone()} ${props.class ?? ''}`}>{props.children}</span>
}

export function LevelBadge(props: { level?: string }): JSX.Element {
  const level = () => String(props.level ?? 'UNKNOWN').toUpperCase()
  return <span class={`level-badge level-${level()}`}>{level()}</span>
}

export function Field(props: {
  label?: string
  hint?: string
  error?: string
  children: JSX.Element
  class?: string
}): JSX.Element {
  return (
    <label class={`field ${props.class ?? ''}`}>
      <Show when={props.label}>
        <span class="label">
          <span>{props.label}</span>
          <Show when={props.hint}>
            <span class="hint">{props.hint}</span>
          </Show>
        </span>
      </Show>
      {props.children}
      <Show when={props.error}>
        <span class="mono-sm ember">{props.error}</span>
      </Show>
    </label>
  )
}

export function TextInput(props: {
  value?: string
  placeholder?: string
  mono?: boolean
  type?: string
  disabled?: boolean
  onInput?: (value: string) => void
  onEnter?: () => void
  class?: string
  autofocus?: boolean
}): JSX.Element {
  return (
    <input
      type={props.type ?? 'text'}
      class={`input ${props.mono ? 'mono' : ''} ${props.class ?? ''}`}
      value={props.value ?? ''}
      placeholder={props.placeholder}
      disabled={props.disabled}
      autofocus={props.autofocus}
      onInput={(e) => props.onInput?.(e.currentTarget.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') props.onEnter?.()
      }}
    />
  )
}

export function TextArea(props: {
  value?: string
  placeholder?: string
  rows?: number
  mono?: boolean
  disabled?: boolean
  onInput?: (value: string) => void
  class?: string
}): JSX.Element {
  return (
    <textarea
      class={`textarea ${props.mono ? 'mono' : ''} ${props.class ?? ''}`}
      rows={props.rows ?? 4}
      value={props.value ?? ''}
      placeholder={props.placeholder}
      disabled={props.disabled}
      onInput={(e) => props.onInput?.(e.currentTarget.value)}
    />
  )
}

export function Select(props: {
  value?: string
  options: Array<{ value: string; label: string }>
  onInput?: (value: string) => void
  disabled?: boolean
  class?: string
}): JSX.Element {
  let selectEl: HTMLSelectElement | undefined
  // 选项可能异步加载；选项更新后同步一次 value，避免原生 select 回退到首项/占位项。
  createEffect(() => {
    const value = props.value ?? ''
    // 读取 options 长度以便选项加载完成时重新同步。
    void props.options.length
    queueMicrotask(() => {
      if (selectEl && selectEl.value !== value) selectEl.value = value
    })
  })
  return (
    <select
      ref={selectEl}
      class={`select ${props.class ?? ''}`}
      value={props.value ?? ''}
      disabled={props.disabled}
      onInput={(e) => props.onInput?.(e.currentTarget.value)}
    >
      <For each={props.options}>
        {(opt) => (
          <option value={opt.value} selected={opt.value === (props.value ?? '')}>
            {opt.label}
          </option>
        )}
      </For>
    </select>
  )
}

export function Checkbox(props: {
  checked: boolean
  label?: string
  disabled?: boolean
  onChange?: (checked: boolean) => void
}): JSX.Element {
  return (
    <label class="row" style={{ gap: '8px', cursor: props.disabled ? 'not-allowed' : 'pointer' }}>
      <input
        type="checkbox"
        class="checkbox"
        checked={props.checked}
        disabled={props.disabled}
        onChange={(e) => props.onChange?.(e.currentTarget.checked)}
      />
      <Show when={props.label}>
        <span style={{ 'font-size': '11.5px', color: 'var(--text-muted)' }}>{props.label}</span>
      </Show>
    </label>
  )
}

export function Toggle(props: {
  checked: boolean
  disabled?: boolean
  label?: string
  onChange?: (checked: boolean) => void
}): JSX.Element {
  return (
    <button
      type="button"
      class={`toggle ${props.checked ? 'on' : ''}`}
      role="switch"
      aria-checked={props.checked}
      aria-label={props.label}
      disabled={props.disabled}
      onClick={() => props.onChange?.(!props.checked)}
    >
      <span />
    </button>
  )
}

export function Tabs(props: {
  items: Array<{ key: string; label: string; count?: number }>
  active: string
  onChange: (key: string) => void
  class?: string
}): JSX.Element {
  return (
    <div class={`tabs ${props.class ?? ''}`} role="tablist">
      <For each={props.items}>
        {(item) => (
          <button
            type="button"
            role="tab"
            aria-selected={props.active === item.key}
            class={`tab ${props.active === item.key ? 'active' : ''}`}
            onClick={() => props.onChange(item.key)}
          >
            {item.label}
            <Show when={item.count != null}>
              <span class="tabs-count">{item.count}</span>
            </Show>
          </button>
        )}
      </For>
    </div>
  )
}

export function Segmented(props: {
  options: Array<{ value: string; label: string }>
  value: string
  onChange: (value: string) => void
}): JSX.Element {
  return (
    <div class="segmented">
      <For each={props.options}>
        {(opt) => (
          <button
            type="button"
            class={props.value === opt.value ? 'active' : ''}
            onClick={() => props.onChange(opt.value)}
          >
            {opt.label}
          </button>
        )}
      </For>
    </div>
  )
}

export function Modal(props: {
  open: boolean
  title: string
  subtitle?: string
  size?: 'sm' | 'md' | 'lg'
  onClose: () => void
  children: JSX.Element
  footer?: JSX.Element
}): JSX.Element {
  createEffect(() => {
    if (!props.open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') props.onClose()
    }
    document.addEventListener('keydown', onKey)
    onCleanup(() => document.removeEventListener('keydown', onKey))
  })

  return (
    <Show when={props.open}>
      <Portal>
        <div
          class="overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) props.onClose()
          }}
        >
          <div
            class={`modal ${props.size === 'lg' ? 'lg' : props.size === 'sm' ? 'sm' : ''}`}
            role="dialog"
            aria-modal="true"
            aria-label={props.title}
          >
            <div class="modal-head">
              <div>
                <div class="modal-title">{props.title}</div>
                <Show when={props.subtitle}>
                  <div class="mono-sm faint" style={{ 'margin-top': '2px' }}>
                    {props.subtitle}
                  </div>
                </Show>
              </div>
              <IconButton icon="x" title="关闭" onClick={props.onClose} />
            </div>
            <div class="modal-body">{props.children}</div>
            <Show when={props.footer}>
              <div class="modal-foot">{props.footer}</div>
            </Show>
          </div>
        </div>
      </Portal>
    </Show>
  )
}

export function Drawer(props: {
  open: boolean
  title: string
  subtitle?: string
  wide?: boolean
  onClose: () => void
  children: JSX.Element
  footer?: JSX.Element
}): JSX.Element {
  createEffect(() => {
    if (!props.open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') props.onClose()
    }
    document.addEventListener('keydown', onKey)
    onCleanup(() => document.removeEventListener('keydown', onKey))
  })

  return (
    <Show when={props.open}>
      <Portal>
        <div class="drawer-overlay" onMouseDown={props.onClose} />
        <aside class={`drawer ${props.wide ? 'wide' : ''}`} role="dialog" aria-modal="true">
          <div class="drawer-head">
            <div class="grow">
              <div class="modal-title">{props.title}</div>
              <Show when={props.subtitle}>
                <div class="mono-sm faint" style={{ 'margin-top': '2px' }}>
                  {props.subtitle}
                </div>
              </Show>
            </div>
            <IconButton icon="x" title="关闭" onClick={props.onClose} />
          </div>
          <div class="drawer-body">{props.children}</div>
          <Show when={props.footer}>
            <div class="modal-foot">{props.footer}</div>
          </Show>
        </aside>
      </Portal>
    </Show>
  )
}

export function ConfirmDialog(props: {
  open: boolean
  title: string
  message: string
  confirmText?: string
  danger?: boolean
  loading?: boolean
  onConfirm: () => void
  onCancel: () => void
}): JSX.Element {
  return (
    <Modal
      open={props.open}
      title={props.title}
      size="sm"
      onClose={props.onCancel}
      footer={
        <>
          <Button variant="ghost" onClick={props.onCancel}>
            取消
          </Button>
          <Button
            variant={props.danger ? 'danger' : 'primary'}
            loading={props.loading}
            onClick={props.onConfirm}
          >
            {props.confirmText ?? '确认'}
          </Button>
        </>
      }
    >
      <p class="muted" style={{ 'font-size': '12px', 'line-height': '1.7' }}>
        {props.message}
      </p>
    </Modal>
  )
}

export function ProgressBar(props: { value: number; class?: string }): JSX.Element {
  const pct = () => Math.max(0, Math.min(100, (props.value || 0) * 100))
  return (
    <div class={`progress ${props.class ?? ''}`} role="progressbar" aria-valuenow={pct()}>
      <span style={{ width: `${pct()}%` }} />
    </div>
  )
}

export function EmptyState(props: {
  icon?: IconName
  title: string
  text?: string
  action?: JSX.Element
}): JSX.Element {
  return (
    <div class="empty">
      <div class="empty-icon">
        <Icon name={props.icon ?? 'crosshair'} size={20} />
      </div>
      <div class="empty-title">{props.title}</div>
      <Show when={props.text}>
        <div class="empty-text">{props.text}</div>
      </Show>
      <Show when={props.action}>{props.action}</Show>
    </div>
  )
}

export function StatTile(props: {
  label: string
  value: string | number
  note?: string
  unit?: string
  tone?: 'mint' | 'amber' | 'ember' | 'sky' | 'violet' | 'muted' | 'default'
  onClick?: () => void
}): JSX.Element {
  const color = () =>
    props.tone === 'amber'
      ? 'var(--amber)'
      : props.tone === 'ember'
        ? 'var(--ember)'
        : props.tone === 'sky'
          ? 'var(--sky)'
          : props.tone === 'violet'
            ? 'var(--violet)'
            : props.tone === 'muted'
              ? 'var(--text-faint)'
              : 'var(--text)'
  return (
    <div
      class="stat-tile"
      role={props.onClick ? 'button' : undefined}
      style={{ cursor: props.onClick ? 'pointer' : 'default' }}
      onClick={props.onClick}
    >
      <div class="stat-label">{props.label}</div>
      <div class="stat-value" style={{ color: color() }}>
        {props.value}
        <Show when={props.unit}>
          <small>{props.unit}</small>
        </Show>
      </div>
      <Show when={props.note}>
        <div class="stat-note">{props.note}</div>
      </Show>
    </div>
  )
}

export function CodeBlock(props: { text: string; maxHeight?: string }): JSX.Element {
  const [copied, setCopied] = createSignal(false)
  return (
    <div style={{ position: 'relative' }}>
      <button
        type="button"
        class="icon-btn"
        style={{ position: 'absolute', right: '6px', top: '6px', 'z-index': 2 }}
        title="复制"
        onClick={async () => {
          await copyText(props.text)
          setCopied(true)
          window.setTimeout(() => setCopied(false), 1200)
        }}
      >
        <Icon name={copied() ? 'check' : 'copy'} size={12} />
      </button>
      <pre class="code-block" style={{ 'max-height': props.maxHeight ?? '320px' }}>
        {props.text}
      </pre>
    </div>
  )
}

export function Pagination(props: {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}): JSX.Element {
  const pages = () => Math.max(1, Math.ceil(props.total / Math.max(1, props.pageSize)))
  const windowPages = () => {
    const total = pages()
    const current = props.page
    const start = Math.max(1, Math.min(current - 2, total - 4))
    const end = Math.min(total, start + 4)
    return Array.from({ length: Math.max(0, end - start + 1) }, (_, i) => start + i)
  }
  return (
    <div class="row" style={{ gap: '4px' }}>
      <Button size="sm" variant="ghost" disabled={props.page <= 1} onClick={() => props.onChange(props.page - 1)}>
        上一页
      </Button>
      <For each={windowPages()}>
        {(p) => (
          <button
            type="button"
            class={`chip ${p === props.page ? 'selected' : ''}`}
            onClick={() => props.onChange(p)}
          >
            {p}
          </button>
        )}
      </For>
      <Button
        size="sm"
        variant="ghost"
        disabled={props.page >= pages()}
        onClick={() => props.onChange(props.page + 1)}
      >
        下一页
      </Button>
    </div>
  )
}

const TOAST_ICON: Record<ToastKind, IconName> = {
  success: 'check',
  error: 'alert',
  warn: 'alert',
  info: 'info',
}

export function ToastHost(): JSX.Element {
  return (
    <Portal>
      <div class="toast-host" aria-live="polite">
        <For each={toasts()}>
          {(item) => (
            <div class={`toast ${item.kind}`}>
              <span
                class="row"
                style={{
                  color:
                    item.kind === 'success'
                      ? 'var(--accent)'
                      : item.kind === 'error'
                        ? 'var(--ember)'
                        : item.kind === 'warn'
                          ? 'var(--amber)'
                          : 'var(--sky)',
                }}
              >
                <Icon name={TOAST_ICON[item.kind]} size={15} />
              </span>
              <div class="grow">
                <div class="toast-title">{item.title}</div>
                <Show when={item.message}>
                  <div class="toast-message">{item.message}</div>
                </Show>
              </div>
              <IconButton icon="x" title="关闭" onClick={() => dismissToast(item.id)} />
            </div>
          )}
        </For>
      </div>
    </Portal>
  )
}

export function SectionLabel(props: { children: JSX.Element }): JSX.Element {
  return <div class="section-label">{props.children}</div>
}

export function PageBusy(props: { visible: boolean; text?: string }): JSX.Element {
  return (
    <Show when={props.visible}>
      <div
        class="row"
        style={{
          gap: '8px',
          padding: '10px 0',
          color: 'var(--text-faint)',
          'font-family': 'var(--font-mono)',
          'font-size': '10px',
        }}
      >
        <Spinner />
        {props.text ?? '加载中…'}
      </div>
    </Show>
  )
}

export function useClickOutside(
  ref: () => HTMLElement | undefined,
  onOutside: () => void,
): void {
  onMount(() => {
    const handler = (e: MouseEvent) => {
      const el = ref()
      if (el && e.target instanceof Node && !el.contains(e.target)) onOutside()
    }
    document.addEventListener('mousedown', handler)
    onCleanup(() => document.removeEventListener('mousedown', handler))
  })
}
