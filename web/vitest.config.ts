import {defineConfig} from 'vitest/config';

// Unit tests exercise framework-independent state helpers. Keeping the
// production StyleX compiler out of this process avoids retaining build-tool
// handles after Vitest has finished.
export default defineConfig({
  test: {
    environment: 'node',
    exclude: ['**/node_modules/**', 'e2e/**'],
  },
});
