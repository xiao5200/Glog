import { Show, createMemo, createSignal, type JSX } from 'solid-js'
import { api, errorText } from '../../api'
import { notify } from '../../stores/app'
import type { ImportFileRequest, ParseConfig } from '../../types'
import { formatBytes } from '../../utils'
import { Badge, Button, Field, Icon } from '../ui'
import { FilePreview } from './FilePreview'
import { TaskProgressInline } from './TaskProgressInline'
import {
  fileBaseName,
  fileParentDir,
  parseModeLabel,
  type BadgeTone,
} from './shared'

export interface ImportFileProps {
  projectId: string
  parse: () => ParseConfig
  onSamples: (lines: string[], source: string) => void
}

/** 单文件导入：文本、.gz 与 .evtx 由后端自动识别。 */
export function ImportFile(props: ImportFileProps): JSX.Element {
  const [filePath, setFilePath] = createSignal('')
  const [fileSize, setFileSize] = createSignal<number | null>(null)
  const [picking, setPicking] = createSignal(false)
  const [starting, setStarting] = createSignal(false)
  const [lastTaskId, setLastTaskId] = createSignal<string | undefined>(undefined)

  const fileKind = createMemo<{ label: string; tone: BadgeTone }>(() => {
    const name = fileBaseName(filePath()).toLowerCase()
    if (name.endsWith('.evtx')) return { label: 'EVTX 事件文件', tone: 'amber' }
    if (name.endsWith('.gz') || name.endsWith('.gzip')) return { label: 'Gzip 压缩', tone: 'violet' }
    return { label: '文本日志', tone: 'sky' }
  })

  async function loadFileSize(path: string) {
    setFileSize(null)
    const parent = fileParentDir(path)
    if (!parent) return
    try {
      const entries = await api.collect.browse(parent)
      const name = fileBaseName(path)
      const normalizedPath = path.replace(/\//g, '\\').toLowerCase()
      const found = (Array.isArray(entries) ? entries : []).find(
        (entry) =>
          !entry.is_dir &&
          (entry.name === name || entry.path.replace(/\//g, '\\').toLowerCase() === normalizedPath),
      )
      if (found) setFileSize(found.size)
    } catch {
      /* 大小探测失败不阻塞导入，界面显示“大小未知” */
    }
  }

  async function pickFile() {
    if (picking()) return
    setPicking(true)
    try {
      const files = await api.system.selectFiles()
      if (Array.isArray(files) && files.length > 0) {
        if (files.length > 1) {
          notify(
            'info',
            '仅导入首个文件',
            `共选择 ${files.length} 个文件，当前使用 ${fileBaseName(files[0])}。`,
          )
        }
        setFilePath(files[0])
        void loadFileSize(files[0])
      }
    } catch (err) {
      notify('error', '选择文件失败', errorText(err))
    } finally {
      setPicking(false)
    }
  }

  async function start() {
    const path = filePath().trim()
    if (!props.projectId) {
      notify('warn', '未打开项目', '请先在项目总览打开一个项目。')
      return
    }
    if (!path) {
      notify('warn', '请选择文件', '文件导入需要指定一个文件。')
      return
    }
    setStarting(true)
    try {
      const request: ImportFileRequest = {
        project_id: props.projectId,
        file_path: path,
        parse: props.parse(),
      }
      const task = await api.collect.importFile(request)
      if (task?.id) setLastTaskId(task.id)
      notify(
        'success',
        '文件导入已启动',
        task?.message || `任务 ${task?.id ?? ''} 已提交，进度见右侧任务队列。`,
      )
    } catch (err) {
      notify('error', '启动文件导入失败', errorText(err))
    } finally {
      setStarting(false)
    }
  }

  return (
    <div class="stack" style={{ gap: '14px' }}>
      <div class="detail-note">
        支持文本、.gz 压缩日志与 .evtx 事件文件；后端会自动解压并识别格式。
      </div>

      <Field label="导入文件" hint="单个文件">
        <div class="row" style={{ gap: '6px' }}>
          <div class="grow">
            <div
              class="row"
              style={{
                gap: '8px',
                padding: '7px 10px',
                border: '1px solid var(--border)',
                background: 'var(--surface)',
                'min-height': '32px',
              }}
            >
              <Icon name="file" size={12} />
              <Show
                when={filePath()}
                fallback={<span class="mono-sm faint">尚未选择文件</span>}
              >
                <span class="mono-sm truncate grow" title={filePath()}>
                  {filePath()}
                </span>
              </Show>
            </div>
          </div>
          <Button size="sm" loading={picking()} onClick={() => void pickFile()}>
            <Show when={!picking()}>
              <Icon name="folder" size={11} />
            </Show>
            选择文件
          </Button>
        </div>
      </Field>

      <Show when={filePath()}>
        <div class="row wrap" style={{ gap: '8px' }}>
          <Badge tone={fileKind().tone}>{fileKind().label}</Badge>
          <span class="mono-sm faint">
            大小 {fileSize() != null ? formatBytes(fileSize() ?? 0) : '未知'}
          </span>
          <span class="mono-sm faint">· 解析 {parseModeLabel(props.parse().mode)}</span>
        </div>
      </Show>

      <FilePreview
        label="文件预览"
        hint="最多 100 行 · 可作解析样本"
        path={filePath()}
        onPathChange={(value) => {
          setFilePath(value)
          setFileSize(null)
        }}
        onLoad={(lines, path) => {
          if (path && path !== filePath()) setFilePath(path)
          void loadFileSize(path)
          props.onSamples(lines, path)
        }}
        placeholder="选择文件后自动填入，或手动输入路径"
      />

      <div class="row between wrap" style={{ gap: '10px' }}>
        <span class="mono-sm faint truncate" title={filePath()}>
          {filePath() ? fileBaseName(filePath()) : '待选择文件'}
        </span>
        <Button
          variant="primary"
          loading={starting()}
          disabled={!filePath().trim()}
          onClick={() => void start()}
        >
          <Show when={!starting()}>
            <Icon name="upload" size={12} />
          </Show>
          开始导入
        </Button>
      </div>

      <Show when={lastTaskId()}>
        <TaskProgressInline taskId={lastTaskId()} label="文件导入" />
      </Show>
    </div>
  )
}
