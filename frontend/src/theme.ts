export type Theme = 'light' | 'dark'

const STORAGE_KEY = 'glog.theme'

function readStored(): Theme | null {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    if (value === 'light' || value === 'dark') return value
  } catch {
    /* 本地存储不可用时忽略 */
  }
  return null
}

export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
}

export function applyTheme(theme: Theme, persist = true): void {
  const root = document.documentElement
  root.dataset.theme = theme
  root.style.colorScheme = theme
  if (persist) {
    try {
      localStorage.setItem(STORAGE_KEY, theme)
    } catch {
      /* 忽略写入失败 */
    }
  }
}

/** 在首屏渲染前调用，避免深色偏好下出现白色闪屏。 */
export function initTheme(): Theme {
  const theme: Theme = 'light'
  applyTheme(theme, false)
  return theme
}

export function toggleTheme(): Theme {
  const next: Theme = currentTheme() === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  return next
}
