/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        ink: {
          950: '#f5f7fb',
          900: '#eef2f7',
          850: '#ffffff',
          800: '#f8fafc',
          750: '#f2f4f7',
          700: '#e4e7ec',
          600: '#d0d5dd',
        },
        mint: {
          DEFAULT: '#2563eb',
          dim: '#1d4ed8',
          deep: '#eff4ff',
        },
        amber: {
          DEFAULT: '#f79009',
          dim: '#b54708',
        },
        ember: { DEFAULT: '#f04438', dim: '#b42318' },
        sky: { DEFAULT: '#0ba5ec', dim: '#026aa2' },
        paper: { DEFAULT: '#1d2939', muted: '#667085', faint: '#98a2b3' },
      },
      fontFamily: {
        display: ['"IBM Plex Sans"', '"Microsoft YaHei UI"', '"Microsoft YaHei"', 'sans-serif'],
        sans: ['"IBM Plex Sans"', '"Microsoft YaHei UI"', '"Microsoft YaHei"', 'sans-serif'],
        mono: ['"IBM Plex Mono"', '"Cascadia Code"', 'Consolas', 'monospace'],
      },
      letterSpacing: {
        widest2: '0.18em',
      },
      animation: {
        'sweep': 'sweep 2.4s linear infinite',
        'pulse-soft': 'pulseSoft 2.8s ease-in-out infinite',
      },
      keyframes: {
        sweep: {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(300%)' },
        },
        pulseSoft: {
          '0%, 100%': { opacity: '0.45' },
          '50%': { opacity: '1' },
        },
      },
    },
  },
  plugins: [],
}
