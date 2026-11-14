import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The user app is served at `/`; the admin console lives at /admin.
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  server: {
    port: 5174,
    proxy: {
      '/api': `http://localhost:${process.env.SERVER_PORT ?? 8080}`,
    },
  },
})
