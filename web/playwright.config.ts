import { defineConfig, devices } from "@playwright/test";

const configuredPort = process.env.PLAYWRIGHT_PORT ?? "4173";
if (!/^[0-9]+$/.test(configuredPort) || !Number.isSafeInteger(Number(configuredPort)) || Number(configuredPort) < 1 || Number(configuredPort) > 65535) {
  throw new Error("PLAYWRIGHT_PORT must be an integer from 1 to 65535");
}
const port = Number(configuredPort);
const url = `http://127.0.0.1:${port}`;
const configuredWorkers = process.env.PLAYWRIGHT_WORKERS;
if (configuredWorkers !== undefined && (!/^[0-9]+$/.test(configuredWorkers) || !Number.isSafeInteger(Number(configuredWorkers)) || Number(configuredWorkers) < 1 || Number(configuredWorkers) > 16)) {
  throw new Error("PLAYWRIGHT_WORKERS must be an integer from 1 to 16");
}

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  workers: configuredWorkers === undefined ? undefined : Number(configuredWorkers),
  globalTimeout: 15 * 60_000,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: url,
    browserName: "chromium",
    channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
    headless: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  webServer: {
    command: `npm run build && npm exec vite preview -- --host 127.0.0.1 --port ${port} --strictPort`,
    url,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
  projects: [
    { name: "desktop-1440", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "desktop-2560", use: { ...devices["Desktop Chrome"], viewport: { width: 2560, height: 1440 } } },
  ],
});
