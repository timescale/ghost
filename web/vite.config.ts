import { readdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';
import { nodePolyfills } from 'vite-plugin-node-polyfills';

const ghostServePort = process.env.GHOST_SERVE_DEV_PORT ?? '5174';

// The widget's results.worker.js references its DuckDB .wasm sidecars via
// `new URL(<variable>, import.meta.url)`, so Vite's static analysis can't
// follow them and they don't get emitted automatically. We copy them here.
// (Worker .js sidecars are referenced as string literals, so Vite handles
// those itself — no manual copy needed.)
//
// We also drop the `mvp` DuckDB bundle entirely. The widget's selectBundle()
// prefers the `eh` (WebAssembly exception-handling) bundle on every browser
// shipped since late 2021 (Chrome/Edge 95+, Firefox 100+, Safari 15.2+);
// `mvp` is only a fallback for older browsers, which `ghost serve` doesn't
// support. Dropping it saves ~41 MiB on every embedded ghost binary.
function popsqlQueryWidgetAssets(): Plugin {
  return {
    name: 'popsql-query-widget-assets',
    apply: 'build',
    enforce: 'post',
    async generateBundle(_options, bundle) {
      const require = createRequire(import.meta.url);
      const widgetPkgJson = require.resolve(
        '@timescale/popsql-query-widget/package.json',
      );
      const widgetDir = dirname(widgetPkgJson);

      // results.worker.js holds the canonical wasm filename mapping. Parse
      // it so we don't have to hard-code a hash that changes whenever the
      // widget bumps its DuckDB dependency.
      const widgetWorker = readFileSync(
        join(widgetDir, 'results.worker.js'),
        'utf8',
      );
      const mvpMatch = widgetWorker.match(
        /duckdb_mvp_default\s*=\s*"\.\/([^"]+\.wasm)"/,
      );
      if (!mvpMatch) {
        throw new Error(
          "Could not find duckdb_mvp_default in @timescale/popsql-query-widget's results.worker.js — has its bundle layout changed?",
        );
      }
      const mvpWasm = mvpMatch[1];

      // Prune the mvp worker chunk Vite emitted from its static analysis of
      // results.worker.js. The widget still constructs the URL at runtime,
      // but only takes that branch on browsers without WASM exception
      // handling — which we don't support.
      for (const fileName of Object.keys(bundle)) {
        if (
          /(^|\/)duckdb-browser-mvp\.worker[-A-Za-z0-9_.]*\.js$/.test(fileName)
        ) {
          delete bundle[fileName];
        }
      }

      // Emit the non-mvp .wasm files that Vite missed.
      for (const entry of readdirSync(widgetDir, { withFileTypes: true })) {
        if (
          !entry.isFile() ||
          !entry.name.endsWith('.wasm') ||
          entry.name === mvpWasm
        ) {
          continue;
        }
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
    popsqlQueryWidgetAssets(),
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
      '/api': {
        target: `http://127.0.0.1:${ghostServePort}`,
        changeOrigin: true,
      },
      '/healthz': {
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
