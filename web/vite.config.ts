import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { nodePolyfills } from 'vite-plugin-node-polyfills';
import svgr from 'vite-plugin-svgr';

const ghostServePort = process.env.GHOST_SERVE_DEV_PORT ?? '5174';

export default defineConfig({
  plugins: [
    react(),
    // Import .svg files as React components (via `?react`) so the single
    // <Icon> component can render them inline with currentColor + sizing.
    svgr(),
    // The widget bundle assumes Node globals (Buffer, process, etc.) exist;
    // match the shim list web-cloud uses with this widget.
    nodePolyfills({ include: ['buffer', 'crypto', 'process', 'stream'] }),
  ],
  optimizeDeps: {
    // The widget bundle expects its workers to live next to its main chunk;
    // letting Vite pre-bundle it breaks that assumption.
    exclude: ['@timescale/popsql-query-widget-cdn'],
  },
  server: {
    port: 5173,
    strictPort: false,
    proxy: {
      '/api': {
        target: `http://127.0.0.1:${ghostServePort}`,
        changeOrigin: true,
      },
      '/health': {
        target: `http://127.0.0.1:${ghostServePort}`,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
  },
});
