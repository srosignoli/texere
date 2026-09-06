import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  build: { target: 'esnext' }, // single modern Wails webview; preserve BigInt (mathjs) etc.

  // mathjs is imported ONLY from src/lib/compute-worker.js, and the dev
  // dependency scanner does not traverse `new Worker(new URL(...))` entries —
  // it walks the module graph from index.html, which never reaches the worker.
  // So mathjs was never pre-bundled, while the worker's import was still
  // rewritten to /node_modules/.vite/deps/mathjs.js, and that URL answered 504.
  // The compute cell reported it as "compute worker failed".
  //
  // Naming it is half the fix. The other half is that the pre-bundler refuses
  // to compile mathjs at all:
  //
  //   Big integer literals are not available in the configured target
  //   environment ("chrome87", "edge88", "es2020", "firefox78", "safari13")
  //     node_modules/mathjs/lib/esm/function/utils/isNegative.js:45:21
  //       bigint: x => x < 0n,
  //
  // That default list is the optimizer's own, unrelated to build.target above,
  // and safari13 in it is why `0n` is rejected.
  //
  // `supported` overrides ONE esbuild feature flag rather than moving the whole
  // target. Setting optimizeDeps.esbuildOptions.target to 'esnext' also clears
  // the error, but it re-compiles every dependency to a different level, and
  // doing that broke the app at startup with "Cannot set properties of
  // undefined (setting '_isSameOrigin')". This says only what is actually
  // true: the one webview this ships to has BigInt.
  optimizeDeps: {
    include: ['mathjs'],
    esbuildOptions: { supported: { bigint: true } },
  },
})
