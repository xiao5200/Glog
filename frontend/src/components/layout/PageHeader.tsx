import { JSX, Show } from 'solid-js'

export function PageHeader(props: {
  eyebrow?: string
  title: string
  subtitle?: string
  actions?: JSX.Element
}): JSX.Element {
  return (
    <header class="page-header">
      <div class="grow">
        <Show when={props.eyebrow}>
          <div class="eyebrow">{props.eyebrow}</div>
        </Show>
        <h1 class="page-title">{props.title}</h1>
        <Show when={props.subtitle}>
          <p class="page-subtitle">{props.subtitle}</p>
        </Show>
      </div>
      <Show when={props.actions}>
        <div class="page-actions">{props.actions}</div>
      </Show>
    </header>
  )
}
