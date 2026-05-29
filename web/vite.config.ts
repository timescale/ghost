import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import { nodePolyfills } from 'vite-plugin-node-polyfills';
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const ghostServePort = process.env.GHOST_SERVE_DEV_PORT ?? '5174';

// The widget's results.worker.js and editor.worker.js load their DuckDB +
// Monaco sidecars via `new URL(<variable>, import.meta.url)`. Vite only emits
// referenced assets when the URL argument is a string literal, so the
// sidecars get missed and the workers 404 at runtime. We emit them manually
// — ported from web-cloud/vite.config.ts.
function copyPopsqlQueryWidgetAssets(): Plugin {
  return {
    name: 'copy-popsql-query-widget-assets',
    apply: 'build',
    async generateBundle() {
      const require = createRequire(import.meta.url);
      const widgetPkgJson = require.resolve('@timescale/popsql-query-widget/package.json');
      const widgetDir = dirname(widgetPkgJson);
      const pattern = /^(duckdb-browser-(?:eh|mvp)\.worker\.js|editor\.worker\.js|.+\.wasm)$/;
      for (const entry of readdirSync(widgetDir, { withFileTypes: true })) {
        if (!entry.isFile() || !pattern.test(entry.name)) continue;
        this.emitFile({
          type: 'asset',
          fileName: `assets/${entry.name}`,
          source: readFileSync(join(widgetDir, entry.name)),
        });
      }
    },
  };
}

export default defineConfig({
  plugins: [
    react(),
    // The widget bundle assumes Node globals (Buffer, process, etc.) exist;
    // match the shim list web-cloud uses with this widget.
    nodePolyfills({ include: ['buffer', 'crypto', 'process', 'stream'] }),
    copyPopsqlQueryWidgetAssets(),
  ],
  optimizeDeps: {
    // The widget bundle expects its workers to live next to its main chunk;
    // letting Vite pre-bundle it breaks that assumption.
    exclude: ['@timescale/popsql-query-widget'],
  },
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
