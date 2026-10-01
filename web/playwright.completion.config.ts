import {defineConfig, devices} from "@playwright/test";

// Test the real embedded backend; no Vite preview or mocked API data.
export default defineConfig({
  testDir: "./completion-e2e",
  timeout: 90_000,
  fullyParallel: false,
  retries: 0,
  use: {
    browserName: "chromium",
    channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
    headless: process.platform !== "darwin",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  outputDir: "./test-results/completion",
  projects: [{name: "installed-product", use: {...devices["Desktop Chrome"],
    viewport: {width: 1440, height: 900}}}],
});
