import {defineConfig, type ProxyOptions} from 'vite';
import react from '@vitejs/plugin-react';
import stylex from '@stylexjs/unplugin';

export function localDevBackendOrigin(value = process.env.AIOS_UI_ORIGIN): string | undefined {
  if (!value) return undefined;
  const url = new URL(value);
  if (url.protocol !== 'http:' || !['127.0.0.1', '::1', 'localhost'].includes(url.hostname) || url.pathname !== '/' || url.search || url.hash) {
    throw new Error('AIOS_UI_ORIGIN must be a loopback HTTP origin without a path, query, or fragment');
  }
  return url.origin;
}

function backendProxy(origin: string): ProxyOptions {
  return {
    target: origin,
    changeOrigin: true,
    configure(proxy) {
      // The Go facade deliberately requires its own loopback Origin on session
      // and CSRF-protected requests. Vite is a same-machine development relay.
      proxy.on('proxyReq', proxyReq => proxyReq.setHeader('Origin', origin));
    },
  };
}

const backendOrigin = localDevBackendOrigin();

export default defineConfig({
  plugins: [stylex.vite({useCSSLayers: true}), react()],
  server: backendOrigin ? {proxy: {'/api': backendProxy(backendOrigin), '/events': backendProxy(backendOrigin)}} : undefined,
  build: {outDir: '../internal/webui/dist', emptyOutDir: true},
});
