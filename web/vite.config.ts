import { defineConfig } from 'vite'
import preact from '@preact/preset-vite'

// `npm run dev` proxies API and neko traffic to a running Go server.
const server = process.env.COZYCAST_SERVER ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [preact()],
  build: {
    outDir: '../server/webui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': { target: server, ws: true },
      '/neko': { target: server, ws: true },
    },
  },
})
