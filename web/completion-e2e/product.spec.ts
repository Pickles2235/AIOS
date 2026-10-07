import {test, expect, type Page} from "@playwright/test";
import {spawn, execFileSync, type ChildProcess} from "node:child_process";
import {mkdtempSync, writeFileSync, mkdirSync, realpathSync, rmSync, chmodSync, readdirSync, statSync} from "node:fs";
import {join, resolve} from "node:path";
import {tmpdir} from "node:os";

function removeOwned(root: string) {
  if (statSync(root).isDirectory()) {
    chmodSync(root, 0o700);
    for (const name of readdirSync(root)) removeOwned(join(root, name));
  }
}
async function start(root: string): Promise<{child: ChildProcess; url: string}> {
  const child = spawn(process.env.AIOS_COMPLETION_BINARY || resolve("../bin/aios"),
    ["ui", "serve", "--data-dir", join(root, "data")], {stdio: ["ignore", "ignore", "pipe"]});
  let url: string;
  try {url = await new Promise<string>((ok, fail) => {
    const timer = setTimeout(() => {fail(new Error("Backend startup deadline"));}, 15000);
    let output = "";
    child.stderr!.on("data", bytes => {
      output += bytes;
      const match = output.match(/Open local UI: (http:\/\/[^\s]+)/);
      if (match) {clearTimeout(timer); ok(match[1]);}
    });
    child.on("error", error => {clearTimeout(timer); fail(error);});
    child.on("exit", code => {clearTimeout(timer); fail(new Error(`Backend exited ${code}`));});
  });} catch (error) {await stop(child); throw error;}
  return {child, url};
}
async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  await new Promise<void>((done, fail) => {
    const force = setTimeout(() => child.kill("SIGKILL"), 10000);
    const deadline = setTimeout(() => {cleanup(); fail(new Error("Backend cleanup deadline"));}, 15000);
    function cleanup() {clearTimeout(force); clearTimeout(deadline);}
    child.once("exit", () => {cleanup(); done();});
    child.kill("SIGTERM");
  });
}
async function build(page: Page, source: string) {
  await page.getByLabel("Source type").selectOption("local");
  await page.getByLabel("id 1", {exact: true}).fill("fixture");
  await page.getByLabel("url 1", {exact: true}).fill(source);
  await page.getByRole("button", {name: "Preview scope"}).click();
  await expect(page.getByRole("button", {name: /Build|Sync and index/, exact: true})).toBeEnabled();
  await page.getByRole("button", {name: /Build|Sync and index/, exact: true}).click();
  await expect(page.getByRole("heading", {name: "Knowledge map"})).toBeVisible({timeout: 30000});
}

test.describe("real installed-product UI", () => {
  let root: string, source: string, backend: {child: ChildProcess; url: string};
  test.beforeEach(async ({page}) => {
    backend = undefined as unknown as typeof backend;
    root = realpathSync(mkdtempSync(join(tmpdir(), "aios-completion-browser-")));
    source = join(root, "source");
    execFileSync("git", ["init", "-q", "-b", "main", source]);
    mkdirSync(join(source, "src"));
    writeFileSync(join(source, "src", "Worker.java"), "package example; class Worker {\n" +
      Array.from({length: 1000}, (_, i) => `void work${i}() {}\n`).join("") + "}\n");
    writeFileSync(join(source, "src", "client.ts"), 'export function requestOrder() { return fetch("/api/orders"); }\n');
    execFileSync("git", ["-C", source, "add", "."]);
    execFileSync("git", ["-C", source, "-c", "user.name=Fixture", "-c",
      "user.email=fixture@example.invalid", "commit", "-qm", "dense generic fixture"]);
    backend = await start(root);
    await page.goto(backend.url);
  });
  test.afterEach(async () => {
    if (backend) await stop(backend.child);
    if (root) {removeOwned(root); rmSync(root, {recursive: true, force: true});}
  });

  test("volumetric truth and semantic zoom", async ({page}) => {
    await expect(page.locator("canvas[data-knowledge-cloud]")).toHaveCount(0);
    await build(page, source);
    const canvas = page.locator("canvas[data-knowledge-cloud]");
    await expect(canvas).toBeVisible();
    await expect(page.getByLabel("Cloud colour legend")).toBeVisible();
    const before = await canvas.screenshot();
    const box = (await canvas.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down(); await page.mouse.move(box.x + box.width * .7, box.y + box.height * .4,
      {steps: 12}); await page.mouse.up();
    await expect.poll(async () => (await canvas.screenshot()).equals(before)).toBe(false);
    for (const level of ["Repositories", "Packages", "Files", "Symbols"]) {
      await page.getByRole("button", {name: level, exact: true}).click();
      await expect(page.getByLabel("Cloud scope")).toContainText(level);
      await expect(page.getByLabel("Cloud membership disclosure")).toContainText(/generation|revision/);
    }
    await page.getByRole("button", {name: "Accessible evidence navigation"}).click();
    await expect(page.getByLabel("Evidence navigation")).toContainText("Worker");
    await page.emulateMedia({reducedMotion: "reduce"});
    await page.getByRole("button", {name: "Open Spotlight"}).click();
    await page.getByRole("combobox", {name: "Spotlight query"}).fill("Worker");
    await page.getByRole("combobox", {name: "Spotlight query"}).press("Enter");
    await expect(page.getByRole("dialog").getByRole("option").first()).toContainText("Worker");
    await page.getByRole("combobox", {name: "Spotlight query"}).press("Enter");
    await expect(page.getByLabel("Evidence inspector")).toContainText("Captured commit");
  });

  test("investigation copy and local history", async ({page, context}) => {
    await build(page, source);
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.getByRole("button", {name: "Open Spotlight"}).click();
    const query = page.getByRole("combobox", {name: "Spotlight query"});
    await query.fill("/symbol @fixture Worker");
    await query.press("Enter");
    const result = page.getByLabel("Investigation result");
    await expect(result).toContainText("generation");
    await expect(page.getByRole("listbox", {name: "Spotlight results"})).toContainText("Worker");
    await page.getByRole("button", {name: "Copy investigation payload"}).click();
    await expect(page.getByText("Copied the displayed version 1 evidence payload.")).toBeVisible();
    const copied = JSON.parse(await page.evaluate(() => navigator.clipboard.readText()));
    expect(copied.schema_version).toBe(1);
    expect(copied.findings[0].entity.path).toBe(copied.canonical_evidence[0].path);
    expect(copied.findings[0].entity.generation).toBe(copied.canonical_evidence[0].generation);
    expect(copied.coverage.generations).toContain(copied.findings[0].entity.generation);
    await page.getByRole("button", {name: "Close Spotlight"}).click();
    await page.getByRole("button", {name: "Open Spotlight"}).click();
    await expect(page.getByLabel("Search history")).toContainText("/symbol @fixture Worker");
    await page.getByRole("button", {name: "Clear history"}).click();
    await expect(page.getByLabel("Search history")).not.toContainText("/symbol @fixture Worker");
    await query.fill("/unknown Worker"); await query.press("Enter");
    await expect(result).toContainText("Unknown");
    await query.press("Escape");
    await expect(page.getByRole("dialog")).not.toBeVisible();
    await expect(page.getByRole("button", {name: "Open Spotlight"})).toBeFocused();
  });

  test("Spotlight cancellation, truncation and stale generation", async ({page}) => {
    await build(page, source);
    await page.getByRole("button", {name: "Open Spotlight"}).click();
    const query = page.getByRole("combobox", {name: "Spotlight query"});
    await page.route("**/api/v1/investigation", async route => {
      await new Promise(resolve => setTimeout(resolve, 1200));
      await route.continue().catch(() => {});
    });
    await query.fill("/symbol @fixture Worker"); await query.press("Enter");
    await page.getByRole("button", {name: "Cancel search"}).click();
    await expect(page.getByRole("alert")).toContainText("Search cancelled");
    await expect(page.getByLabel("Investigation result")).toHaveCount(0);
    await page.unroute("**/api/v1/investigation");
    await query.fill("/symbol @fixture Worker"); await query.press("Enter");
    await expect(page.getByLabel("Investigation result")).toContainText("truncated", {timeout: 30000});
    await page.route("**/api/v1/status", async route => {
      const response = await route.fetch(); const status = await response.json();
      status.repositories[0].generation = "promoted-generation";
      await route.fulfill({response, json: status});
    });
    await expect(page.getByRole("alert")).toContainText("Stale result", {timeout: 10000});
    await expect(page.getByRole("button", {name: "Copy investigation payload"})).toBeDisabled();
  });

  test("contextual modules and reconnect", async ({page}) => {
    await build(page, source);
    await page.getByRole("button", {name: "Show indexing health"}).click();
    const panel = page.getByLabel("Indexing health module", {exact: true});
    await expect(panel).toBeVisible();
    const original = (await panel.boundingBox())!;
    const grip = page.getByRole("button", {name: "Move indexing health"});
    await grip.focus(); await grip.press("ArrowRight");
    await expect.poll(async () => (await panel.boundingBox())!.x).toBeGreaterThan(original.x);
    const moved = (await panel.boundingBox())!;
    await page.reload(); await expect(panel).toBeVisible();
    expect((await panel.boundingBox())!.x).toBeCloseTo(moved.x, 0);
    await page.setViewportSize({width: 800, height: 600});
    const bounded = (await panel.boundingBox())!;
    expect(bounded.x).toBeGreaterThanOrEqual(0); expect(bounded.x + bounded.width).toBeLessThanOrEqual(800);
    await page.getByRole("button", {name: "Reset module positions"}).click();
    await stop(backend.child);
    await expect(page.getByRole("alert")).toContainText(/disconnected|stopped/i);
    backend = await start(root); await page.goto(backend.url);
    await expect(page.getByRole("heading", {name: "Knowledge map"})).toBeVisible();
  });

  test("native GPU dense cloud evidence", async ({page}, info) => {
    expect(process.platform).toBe("darwin"); expect(process.arch).toBe("arm64");
    expect(process.env.AIOS_COMPLETION_REFERENCE_HARDWARE).toBeTruthy();
    await build(page, source);
    const canvas = page.locator("canvas[data-knowledge-cloud]");
    await expect(canvas).toBeVisible();
    const renderer = await canvas.evaluate((element: HTMLCanvasElement) => {
      const gl = element.getContext("webgl2") || element.getContext("webgl");
      if (!gl) return "unavailable";
      const extension = gl.getExtension("WEBGL_debug_renderer_info");
      return extension ? String(gl.getParameter(extension.UNMASKED_RENDERER_WEBGL)) : "unknown";
    });
    expect(renderer).toMatch(/Apple|Metal/i); expect(renderer).not.toMatch(/SwiftShader|software/i);
    const frames = await page.evaluate(async () => {
      const times: number[] = []; let previous = performance.now();
      for (let i = 0; i < 300; i++) await new Promise<void>(done => requestAnimationFrame(now => {
        times.push(now - previous); previous = now; done();
      }));
      return times;
    });
    expect(frames.length).toBe(300);
    await info.attach("native-frame-measurements", {body: JSON.stringify({renderer, frames}),
      contentType: "application/json"});
    await canvas.screenshot({path: info.outputPath("dense-native-cloud.png")});
    await expect(page.getByLabel("Cloud performance")).toContainText(/detail|frame|LOD/i);
  });
});
