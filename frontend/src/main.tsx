import { render } from 'solid-js/web'
import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/500.css'
import '@fontsource/ibm-plex-sans/600.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/500.css'
import './styles.css'
import { initTheme } from './theme'
import { App } from './App'

const root = document.getElementById('root')
if (!root) throw new Error('找不到应用挂载点 #root')

initTheme()

render(() => <App />, root)
