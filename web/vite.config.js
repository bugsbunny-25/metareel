import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  base: '/',
  server: {
    proxy: {
      '/api': `http://localhost:${process.env.SERVER_PORT ?? 8080}`,
    },
  },
})
