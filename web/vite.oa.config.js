import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

export default defineConfig({
  base: './',
  publicDir: false,
  appType: 'mpa',
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    port: 8080,
    strictPort: true,
    proxy: {
      '^/api/oa/v1(?:/|$)': {
        target: process.env.OA_API_TARGET || 'http://127.0.0.1:8888',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist-oa',
    rolldownOptions: {
      input: [
        fileURLToPath(new URL('./oa.html', import.meta.url)),
        fileURLToPath(new URL('./h5.html', import.meta.url))
      ]
    }
  },
  test: {
    include: ['src/oa/**/*.test.js', 'src/h5/**/*.test.js'],
    environment: 'jsdom'
  }
})
