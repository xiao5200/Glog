import { Show, createSignal, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import { Button, CodeBlock, Field, Icon, TextInput } from '../ui'

/** 可复用的文件预览控件：路径输入 + 后端预览 + 代码块。 */
export function FilePreview(props: {
  path: string
  onPathChange: (path: string) => void
  onLoad?: (lines: string[], path: string) => void
  label?: string
  hint?: string
  placeholder?: string
  disabled?: boolean
  maxHeight?: string
}): JSX.Element {
  const [lines, setLines] = createSignal<string[]>([])
  const [loadedPath, setLoadedPath] = createSignal('')
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal('')

  const load = async () => {
    const path = props.path.trim()
    if (!path) {
      notify('warn', '请填写文件路径', '预览需要指定一个具体文件。')
      return
    }
    setLoading(true)
    setError('')
    try {
      const result = await api.collect.previewFile(path, 100)
      const list = Array.isArray(result) ? result : []
      setLines(list)
      setLoadedPath(path)
      props.onLoad?.(list, path)
      if (!list.length) {
        notify('info', '预览为空', `${path} 没有可显示的内容。`)
      }
    } catch (err) {
      const message = errorText(err)
      setError(message)
      setLines([])
      notify('error', '文件预览失败', message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '8px' }}>
      <Field label={props.label ?? '文件预览'} hint={props.hint ?? '最多读取 100 行'}>
        <div class="row" style={{ gap: '6px' }}>
          <div class="grow">
            <TextInput
              mono
              value={props.path}
              placeholder={props.placeholder ?? '输入一个具体文件路径'}
              disabled={props.disabled}
              onInput={(value) => props.onPathChange(value)}
              onEnter={() => void load()}
            />
          </div>
          <Button
            size="sm"
            loading={loading()}
            disabled={props.disabled}
            onClick={() => void load()}
          >
            <Show when={!loading()}>
              <Icon name="eye" size={11} />
            </Show>
            预览
          </Button>
        </div>
      </Field>

      <Show when={error()}>
        <div class="danger-note mono-sm">{error()}</div>
      </Show>

      <Show when={lines().length > 0} fallback={<div class="mono-sm faint">尚未加载预览。支持文本日志与 .gz；EVTX 请使用文件导入解析。</div>}>
        <div class="row between wrap" style={{ gap: '6px' }}>
          <span class="mono-sm faint truncate" title={loadedPath()}>
            {loadedPath()} · {lines().length} 行
          </span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => props.onLoad?.(lines(), loadedPath())}
          >
            <Icon name="file" size={11} />
            用作解析样本
          </Button>
        </div>
        <CodeBlock text={lines().join('\n')} maxHeight={props.maxHeight ?? '260px'} />
      </Show>
    </div>
  )
}
