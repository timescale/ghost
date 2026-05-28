import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const ghostServePort = process.env.GHOST_SERVE_DEV_PORT ?? '5174';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: false,
    proxy: {
      '/api': { target: `http://127.0.0.1:${ghostServePort}`, changeOrigin: true },
      '/healthz': { target: `http://127.0.0.1:${ghostServePort}`, changeOrigin: true },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
  },
});
