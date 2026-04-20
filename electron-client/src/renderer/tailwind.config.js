/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,ts,js}'],
  theme: {
    extend: {
      colors: {
        // wnwn brand palette — muted, focused, GTD-appropriate
        surface: {
          DEFAULT: '#1a1a1a',
          raised: '#242424',
          border: '#333333',
        },
        accent: {
          DEFAULT: '#7c5cbf',
          hover: '#9575d4',
          muted: '#4a3970',
        },
        text: {
          primary: '#e8e8e8',
          secondary: '#999999',
          muted: '#666666',
        },
        state: {
          'next-action': '#6db36d',
          'waiting-for': '#d4a843',
          'someday': '#6699bb',
          'done': '#555555',
          'canceled': '#555555',
          'overdue': '#cc5555',
        },
      },
      fontFamily: {
        sans: ['-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
};
