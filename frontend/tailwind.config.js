/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        ok: '#22c55e',
        warning: '#f59e0b',
        critical: '#ef4444',
        unknown: '#6b7280',
      },
    },
  },
  plugins: [],
}
