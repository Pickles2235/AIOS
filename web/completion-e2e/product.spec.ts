import { acPowerEnvironment } from "../e2e/fixture-power";
import type { CloudPage } from "../src/cloud-types";
import { test, expect, type Page } from "@playwright/test";
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import {
  mkdtempSync,
  writeFileSync,
  mkdirSync,
  realpathSync,
  rmSync,
  chmodSync,
  readdirSync,
  statSync,
  readFileSync,
  unlinkSync,
} from "node:fs";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";

function removeOwned(root: string) {
  if (statSync(root).isDirectory()) {
    chmodSync(root, 0o700);
    for (const name of readdirSync(root)) removeOwned(join(root, name));
  }
}
async function start(
  root: string,
): Promise<{ child: ChildProcess; url: string }> {
  const child = spawn(
    process.env.AIOS_COMPLETION_BINARY || resolve("../bin/aios"),
    ["ui", "serve", "--data-dir", join(root, "data")],
    { env: acPowerEnvironment(root), stdio: ["ignore", "ignore", "pipe"] },
  );
  let url: string;
  try {
    url = await new Promise<string>((ok, fail) => {
      const timer = setTimeout(() => {
        fail(new Error("Backend startup deadline"));
      }, 15000);
      let output = "";
      child.stderr!.on("data", (bytes) => {
        output += bytes;
        const match = output.match(/Open local UI: (http:\/\/[^\s]+)/);
        if (match) {
          clearTimeout(timer);
          ok(match[1]);
        }
      });
      child.on("error", (error) => {
        clearTimeout(timer);
        fail(error);
      });
      child.on("exit", (code) => {
        clearTimeout(timer);
        fail(new Error(`Backend exited ${code}`));
      });
    });
  } catch (error) {
    await stop(child);
    throw error;
  }
  return { child, url };
}
async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  await new Promise<void>((done, fail) => {
    const force = setTimeout(() => child.kill("SIGKILL"), 10000);
    const deadline = setTimeout(() => {
      cleanup();
      fail(new Error("Backend cleanup deadline"));
    }, 15000);
    function cleanup() {
      clearTimeout(force);
      clearTimeout(deadline);
    }
    child.once("exit", () => {
      cleanup();
      done();
    });
    child.kill("SIGTERM");
  });
}
async function build(page: Page, source: string) {
  await page.getByLabel("Source type").selectOption("local");
  await page.getByLabel("id 1", { exact: true }).fill("fixture");
  await page.getByLabel("url 1", { exact: true }).fill(source);
  await page.getByRole("button", { name: "Preview scope" }).click();
  await expect(
    page.getByRole("button", { name: /Build|Sync and index/, exact: true }),
  ).toBeEnabled();
  await page
    .getByRole("button", { name: /Build|Sync and index/, exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Knowledge map" }),
  ).toBeVisible({ timeout: 30000 });
}

async function api<T>(page: Page, path: string, body?: unknown): Promise<T> {
  return page.evaluate(
    async ({ path, body }) => {
      const session = await (await fetch("/api/v1/session")).json();
      const response = await fetch(
        path,
        body === undefined
          ? {}
          : {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                "X-CSRF-Token": session.csrf_token,
              },
              body: JSON.stringify(body),
            },
      );
      if (!response.ok) throw new Error(`API ${response.status}`);
      return response.json();
    },
    { path, body },
  );
}
async function drillWorker(page: Page) {
  await page
    .getByRole("button", { name: "Estate overview", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Explore fixture", exact: true })
    .click();
  await page
    .getByRole("button", { name: /^Explore / })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Explore Worker.java", exact: true })
    .click();
  await expect(page.getByLabel("Cloud scope")).toContainText("Symbols");
  return page.locator("canvas[data-knowledge-cloud]");
}

test.describe("real installed-product UI", () => {
  let root: string,
    source: string,
    backend: { child: ChildProcess; url: string };
  test.beforeEach(async ({ page }) => {
    backend = undefined as unknown as typeof backend;
    root = realpathSync(
      mkdtempSync(join(tmpdir(), "aios-completion-browser-")),
    );
    source = join(root, "source");
    execFileSync("git", ["init", "-q", "-b", "main", source]);
    mkdirSync(join(source, "src"));
    writeFileSync(
      join(source, "src", "Worker.java"),
      "package example; class Worker {\n" +
        Array.from(
          { length: 1000 },
          (_, i) => `void work${i}() {${i === 0 ? "work1();" : ""}}\n`,
        ).join("") +
        "}\n",
    );
    writeFileSync(
      join(source, "src", "client.ts"),
      'export function requestOrder() { return fetch("/api/orders"); }\n',
    );
    writeFileSync(
      join(source, "src", "other.ts"),
      'export function requestOrder() { return "local"; }\n',
    );
    execFileSync("git", ["-C", source, "add", "."]);
    execFileSync("git", [
      "-C",
      source,
      "-c",
      "user.name=Fixture",
      "-c",
      "user.email=fixture@example.invalid",
      "commit",
      "-qm",
      "dense generic fixture",
    ]);
    backend = await start(root);
    await page.goto(backend.url);
  });
  test.afterEach(async () => {
    if (backend) await stop(backend.child);
    if (root) {
      removeOwned(root);
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("volumetric truth and semantic zoom", async ({ page }, info) => {
    await expect(page.locator("canvas[data-knowledge-cloud]")).toHaveCount(0);
    await build(page, source);
    const canvas = page.locator("canvas[data-knowledge-cloud]");
    await expect(canvas).toBeVisible();
    await expect(page.getByLabel("Cloud colour legend")).toBeVisible();
    const estate = await api<CloudPage>(page, "/api/v1/cloud?limit=256");
    expect(estate.total_files).toBe(3);
    expect(estate.nodes).toHaveLength(1);
    expect(estate.nodes[0].member_count).toBe(estate.total_entities);
    const repo = await api<CloudPage>(
      page,
      "/api/v1/cloud?" +
        new URLSearchParams({
          scope: estate.nodes[0].child_scope!,
          snapshot: estate.snapshot,
        }),
    );
    expect(repo.nodes.reduce((sum, n) => sum + n.member_count, 0)).toBe(
      repo.total_entities,
    );
    const group = await api<CloudPage>(
      page,
      "/api/v1/cloud?" +
        new URLSearchParams({
          scope: repo.nodes[0].child_scope!,
          snapshot: estate.snapshot,
        }),
    );
    expect(group.nodes.reduce((sum, n) => sum + n.member_count, 0)).toBe(
      group.total_entities,
    );
    const file = group.nodes.find((n) => n.label === "Worker.java")!;
    const symbols = await api<CloudPage>(
      page,
      "/api/v1/cloud?" +
        new URLSearchParams({
          scope: file.child_scope!,
          snapshot: estate.snapshot,
          limit: "256",
        }),
    );
    expect(symbols.nodes.length).toBe(256);
    expect(symbols.edges.length).toBeGreaterThan(0);
    for (const edge of symbols.edges) {
      expect(symbols.nodes.some((n) => n.handle === edge.subject)).toBe(true);
      expect(symbols.nodes.some((n) => n.handle === edge.object)).toBe(true);
      const excerpt = await api<{
        path: string;
        generation: string;
        lines: string[];
      }>(page, "/api/v1/evidence", {
        evidence: edge.evidence,
        before: 0,
        after: 0,
        max_lines: 20,
      });
      expect(excerpt.path).toBe("src/Worker.java");
      expect(excerpt.generation).toBe(symbols.generations[0].generation);
      expect(excerpt.lines.length).toBeGreaterThan(0);
    }

    expect(symbols.total_entities).toBeGreaterThan(1000);
    const handles = new Set<string>();
    let part = symbols;
    do {
      for (const node of part.nodes) {
        expect(handles.has(node.handle)).toBe(false);
        handles.add(node.handle);
        expect(node.entity!.generation).toBe(symbols.generations[0].generation);
      }
      if (!part.next_cursor) break;
      part = await api<CloudPage>(
        page,
        "/api/v1/cloud?" +
          new URLSearchParams({
            scope: file.child_scope!,
            snapshot: estate.snapshot,
            cursor: part.next_cursor,
            limit: "256",
          }),
      );
    } while (true);
    expect(handles.size).toBe(file.member_count);
    await drillWorker(page);
    const before = await canvas.screenshot(),
      box = (await canvas.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width * 0.7, box.y + box.height * 0.4, {
      steps: 12,
    });
    await page.mouse.up();
    expect((await canvas.screenshot()).equals(before)).toBe(false);
    const camera = JSON.parse((await canvas.getAttribute("data-camera"))!);
    await page.getByRole("button", { name: "Pan right", exact: true }).click();
    await page.getByRole("button", { name: "Zoom in", exact: true }).click();
    const moved = JSON.parse((await canvas.getAttribute("data-camera"))!);
    expect(moved.panX).toBeGreaterThan(camera.panX);
    expect(moved.zoom).toBeGreaterThan(camera.zoom);
    await canvas.focus();
    await canvas.press("ArrowLeft");
    expect(
      JSON.parse((await canvas.getAttribute("data-camera"))!).yaw,
    ).toBeLessThan(moved.yaw);
    await page
      .getByLabel("Evidence navigation")
      .getByRole("listitem")
      .filter({ has: page.getByText("symbol:class", { exact: true }) })
      .getByRole("button", { name: "Worker", exact: true })
      .click();
    await expect(page.getByLabel("Evidence inspector")).toContainText(
      "Captured commit",
    );
    await expect(
      page.getByLabel("Revision-aware source actions"),
    ).toContainText("Validated generation");
    const selected = symbols.nodes.find(
      (n) => n.entity?.label === "Worker" && n.entity.kind === "symbol:class",
    )!;
    const canonical = await api<{ handle: string; generation: string }>(
      page,
      "/api/v1/entity",
      { handle: selected.handle },
    );
    expect(canonical.handle).toBe(selected.handle);
    const actions = await api<{
      actions: { kind: string; available: boolean; url?: string }[];
    }>(page, "/api/v1/source-actions", { handle: selected.handle });
    expect(actions.actions.find((a) => a.kind === "finder")?.available).toBe(
      true,
    );
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect(
      page.getByLabel("Knowledge cloud", { exact: true }),
    ).toHaveAttribute("data-motion", "reduced");
    await canvas.evaluate((element: HTMLCanvasElement) => {
      const gl = element.getContext("webgl")!;
      const ext = gl.getExtension("WEBGL_lose_context")!;
      ext.loseContext();
      setTimeout(() => ext.restoreContext(), 1000);
    });
    await expect(page.getByText(/Graphics context lost/)).toBeVisible();
    await expect(
      page.getByText("Graphics restored from the current canonical page."),
    ).toBeVisible();
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    const query = page.getByRole("combobox", { name: "Spotlight query" });
    await query.fill("Worker");
    await query.press("Enter");
    await expect(
      page.getByRole("dialog").getByRole("option").first(),
    ).toContainText("Worker");
    await query.press("Enter");
    await expect(page.getByLabel("Cloud query disclosure")).toContainText(
      "actual returned",
    );
    await canvas.screenshot({
      path: info.outputPath("cloud-symbols-accessible.png"),
    });
    // The next real immutable capture changes knowledge; old cursor must fail.
    writeFileSync(
      join(source, "src", "Worker.java"),
      "package example; class Worker { void replacement() {} }\n",
    );
    await page
      .getByRole("button", { name: "Manage repositories", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Force rebuild fixture", exact: true })
      .click();
    await expect
      .poll(
        async () => (await api<CloudPage>(page, "/api/v1/cloud")).snapshot,
        { timeout: 30000 },
      )
      .not.toBe(estate.snapshot);
    const stale = await page.evaluate(
      async (query) => (await fetch("/api/v1/cloud?" + query)).status,
      new URLSearchParams({
        scope: file.child_scope!,
        snapshot: estate.snapshot,
        cursor: symbols.next_cursor!,
      }).toString(),
    );
    expect(stale).toBe(409);
    await expect(page.getByLabel("Cloud generation change")).toContainText(
      /promotion|Generation changed/,
      { timeout: 10000 },
    );
    await expect(page.getByLabel("Staged cloud activity")).toContainText(
      "not queryable",
    );
    await page
      .getByRole("button", { name: "Refresh cloud", exact: true })
      .click();
    await expect(
      page
        .getByLabel("Evidence navigation")
        .getByRole("listitem")
        .filter({ has: page.getByText("symbol:class", { exact: true }) })
        .getByRole("button", { name: "Worker", exact: true }),
    ).toBeVisible();
    await page
      .getByLabel("Evidence navigation")
      .getByRole("listitem")
      .filter({ has: page.getByText("symbol:class", { exact: true }) })
      .getByRole("button", { name: "Worker", exact: true })
      .click();
    await expect(page.getByLabel("Evidence inspector")).toContainText(
      "replacement",
    );
    unlinkSync(join(source, "src", "Worker.java"));
    await page
      .getByRole("button", { name: "Force rebuild fixture", exact: true })
      .click();
    await expect(page.getByLabel("Cloud scope")).toContainText("Repositories", {
      timeout: 30000,
    });
    await expect(page.getByLabel("Evidence inspector")).toContainText(
      "Select an entity",
    );
    await expect(page.getByLabel("Evidence inspector")).not.toContainText(
      "replacement",
    );
    await expect(page.getByLabel("Cloud generation change")).toContainText(
      "previous evidence was cleared",
    );
    await info.attach("canonical-membership", {
      body: JSON.stringify({
        snapshot: estate.snapshot,
        entities: estate.total_entities,
        files: estate.total_files,
        enumeratedFileEntities: handles.size,
      }),
      contentType: "application/json",
    });
  });

  test("investigation copy and local history", async ({ page, context }) => {
    await build(page, source);
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    const query = page.getByRole("combobox", { name: "Spotlight query" });
    await query.fill("/symbol @fixture Worker");
    await query.press("Enter");
    const result = page.getByLabel("Investigation result");
    await expect(result).toContainText("generation");
    await expect(
      page.getByRole("listbox", { name: "Spotlight results" }),
    ).toContainText("Worker");
    await page
      .getByRole("button", { name: "Copy investigation payload" })
      .click();
    await expect(
      page.getByText("Copied the displayed version 1 evidence payload."),
    ).toBeVisible();
    const copied = JSON.parse(
      await page.evaluate(() => navigator.clipboard.readText()),
    );
    expect(copied.schema_version).toBe(1);
    expect(copied.findings[0].entity.path).toBe(
      copied.canonical_evidence[0].path,
    );
    expect(copied.findings[0].entity.generation).toBe(
      copied.canonical_evidence[0].generation,
    );
    expect(copied.coverage.generations).toContain(
      copied.findings[0].entity.generation,
    );
    await page.getByRole("button", { name: "Close Spotlight" }).click();
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    await expect(page.getByLabel("Search history")).toContainText(
      "/symbol @fixture Worker",
    );
    await page.getByRole("button", { name: "Clear history" }).click();
    await expect(page.getByLabel("Search history")).not.toContainText(
      "/symbol @fixture Worker",
    );
    await query.fill("/unknown Worker");
    await query.press("Enter");
    await expect(result).toContainText("Unknown");
    await query.press("Escape");
    await expect(page.getByRole("dialog")).not.toBeVisible();
    await expect(
      page.getByRole("button", { name: "Open Spotlight" }),
    ).toBeFocused();
  });

  test("Spotlight cancellation, truncation and stale generation", async ({
    page,
  }) => {
    await build(page, source);
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    const query = page.getByRole("combobox", { name: "Spotlight query" });
    await query.fill("/symbol @fixture requestOrder");
    await query.press("Enter");
    const options = page
      .getByRole("listbox", { name: "Spotlight results" })
      .getByRole("option");
    await expect.poll(() => options.count()).toBeGreaterThanOrEqual(2);
    await query.press("ArrowDown");
    await expect(query).toHaveAttribute(
      "aria-activedescendant",
      "spotlight-result-1",
    );
    await query.press("ArrowUp");
    await expect(query).toHaveAttribute(
      "aria-activedescendant",
      "spotlight-result-0",
    );
    await query.press("ArrowDown");
    const selected = await options.nth(1).getByRole("button").textContent();
    const path = selected!.match(/fixture\/(src\/[^:]+):/)![1];
    await query.press("Enter");
    await expect(page.getByLabel("Evidence inspector")).toContainText(path);
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    await page.route("**/api/v1/investigation", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1200));
      await route.continue().catch(() => {});
    });
    await query.fill("/symbol @fixture Worker");
    await query.press("Enter");
    await page.getByRole("button", { name: "Cancel search" }).click();
    await expect(page.getByRole("alert")).toContainText("Search cancelled");
    await expect(page.getByLabel("Investigation result")).toHaveCount(0);
    await page.unroute("**/api/v1/investigation");
    await query.fill("/symbol @fixture Worker");
    await query.press("Enter");
    await expect(page.getByLabel("Investigation result")).toContainText(
      "truncated",
      { timeout: 30000 },
    );
    await page.route("**/api/v1/status", async (route) => {
      const response = await route.fetch();
      const status = await response.json();
      status.repositories[0].generation = "promoted-generation";
      await route.fulfill({ response, json: status });
    });
    await expect(page.getByRole("alert")).toContainText("Stale result", {
      timeout: 10000,
    });
    await expect(
      page.getByRole("button", { name: "Copy investigation payload" }),
    ).toBeDisabled();
  });

  test("contextual modules and reconnect", async ({ page }) => {
    await expect(page.getByLabel("Visible operational modules").locator("[data-context-module]")).toHaveCount(0);
    await build(page, source);
    await expect.poll(async () => (await api<{ jobs: { repository: string; state: string; active_generation: string }[] }>(page, "/api/v1/jobs")).jobs.find(job => job.repository === "fixture")?.active_generation, { timeout: 30000 }).toBeTruthy();
    const controls = page.getByRole("navigation", { name: "Operational modules" });
    const labels = ["Indexing", "Health", "Storage and size", "Performance", "Coverage", "Scheduled jobs", "Active query"];
    for (const label of labels) {
      await controls.getByRole("button", { name: label, exact: true }).click();
      await expect(page.getByLabel(`${label} module`, { exact: true })).toBeVisible();
    }
    await expect(page.getByLabel("Health module", { exact: true })).toContainText("fixture", { timeout: 30000 });
    await expect(page.getByLabel("Storage and size module", { exact: true })).toContainText("Owned data");
    await expect(page.getByLabel("Performance module", { exact: true })).toContainText("Observed load");
    await expect(page.getByLabel("Coverage module", { exact: true })).toContainText("active of");
    await expect(page.getByLabel("Scheduled jobs module", { exact: true })).toContainText("fixture");
    await expect(page.getByLabel("Active query module", { exact: true })).toContainText("No active query");
    await page.route("**/api/v1/query", async route => { await new Promise(done => setTimeout(done, 350)); await route.continue(); });
    await page.getByLabel("Query", { exact: true }).fill("Worker");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page.getByLabel("Active query module", { exact: true })).toContainText("Query in progress");
    await expect(page.getByLabel("Active query module", { exact: true })).toContainText("Last direct query: found");
    await page.unroute("**/api/v1/query");
    await page.getByRole("button", { name: "Open Spotlight" }).click();
    await page.route("**/api/v1/investigation", async route => { await new Promise(done => setTimeout(done, 350)); await route.continue(); });
    await page.getByLabel("Spotlight query").fill("Worker");
    await page.getByRole("button", { name: "Search captured evidence" }).click();
    await expect(page.getByLabel("Active query module", { exact: true })).toContainText("Spotlight query in progress");
    await expect(page.getByLabel("Active query module", { exact: true })).toContainText("Spotlight result: found");
    await page.unroute("**/api/v1/investigation");
    await page.getByRole("button", { name: "Close Spotlight" }).click();
    const panel = page.getByLabel("Indexing module", { exact: true });
    await expect(panel).toBeVisible();
    const original = (await panel.boundingBox())!;
    const title = (await panel.locator("header strong").boundingBox())!;
    await page.mouse.move(title.x + title.width / 2, title.y + title.height / 2);
    await page.mouse.down();
    await page.mouse.move(title.x + title.width / 2 + 32, title.y + title.height / 2 + 24, { steps: 4 });
    await page.mouse.up();
    await expect(panel).toHaveAttribute("data-detached", "true");
    expect((await panel.boundingBox())!.x).toBeGreaterThan(original.x);
    await panel.getByRole("button", { name: "Reset position" }).click();
    await expect(panel).not.toHaveAttribute("data-detached", "true");
    const grip = page.getByRole("button", { name: "Move Indexing module" });
    await grip.focus();
    await grip.press("ArrowRight");
    await expect
      .poll(async () => (await panel.boundingBox())!.x)
      .toBeGreaterThan(original.x);
    const moved = (await panel.boundingBox())!;
    await page.reload();
    await controls.getByRole("button", { name: "Indexing", exact: true }).click();
    await expect(panel).toBeVisible();
    expect((await panel.boundingBox())!.x).toBeCloseTo(moved.x, 0);
    await page.setViewportSize({ width: 320, height: 300 });
    const bounded = (await panel.boundingBox())!;
    expect(bounded.x).toBeGreaterThanOrEqual(0);
    expect(bounded.x + bounded.width).toBeLessThanOrEqual(320);
    expect(bounded.y).toBeGreaterThanOrEqual(0);
    expect(bounded.y + bounded.height).toBeLessThanOrEqual(300);
    await panel.getByRole("button", { name: "Reset position" }).click();
    await expect(panel).not.toHaveAttribute("data-detached", "true");
    await page.getByRole("button", { name: "Refresh", exact: true }).click();
    await page.evaluate(() => localStorage.setItem("aios.contextual-placement.v1", JSON.stringify({ indexing: { x: 999999, y: -999999 } })));
    await page.reload();
    await controls.getByRole("button", { name: "Indexing", exact: true }).click();
    const repaired = (await panel.boundingBox())!;
    expect(repaired.x).toBeGreaterThanOrEqual(0);
    expect(repaired.x + repaired.width).toBeLessThanOrEqual(320);
    expect(repaired.y).toBeGreaterThanOrEqual(0);
    expect(repaired.y + repaired.height).toBeLessThanOrEqual(300);
    await page.route("**/api/v1/activity", route => route.fulfill({ status: 503, json: { error: "activity temporarily unavailable" } }));
    await expect(panel.getByRole("alert")).toContainText("activity temporarily unavailable");
    await page.unroute("**/api/v1/activity");
    await expect(panel.getByRole("alert")).toHaveCount(0, { timeout: 10000 });
    const observed = await api<{ stream_id: string; sequence: number }>(page, "/api/v1/activity");
    const missed = observed.sequence + 5;
    await page.route("**/api/v1/activity", route => route.fulfill({ json: { stream_id: observed.stream_id, sequence: missed, oldest_sequence: missed, retention_events: 512, events: [{ sequence: missed, at: new Date().toISOString(), stage: "discovered", repository: "fixture", files: 3, queryable: false }] } }));
    await expect(panel.getByRole("alert")).toContainText("gap", { timeout: 10000 });
    await page.route("**/api/v1/onboarding", route => route.fulfill({ json: { state: "failed", error: "fixture setup failed", completed_repositories: 0 } }));
    await expect(panel.getByRole("alert").filter({ hasText: "fixture setup failed" })).toBeVisible();
    await expect(panel.getByRole("alert").filter({ hasText: "gap" })).toBeVisible();
    await page.unroute("**/api/v1/onboarding");
    await page.route("**/api/v1/activity", route => route.fulfill({ json: { stream_id: "replacement-stream", sequence: missed + 1, oldest_sequence: missed + 1, retention_events: 512, events: [{ sequence: missed + 1, at: new Date().toISOString(), stage: "staged", repository: "fixture", files: 3, queryable: false }] } }));
    await expect(panel.getByRole("alert").filter({ hasText: "restarted" })).toBeVisible();
    await page.unroute("**/api/v1/activity");
    await stop(backend.child);
    await expect(page.getByRole("alert").filter({ hasText: /disconnected|stopped/i })).toContainText(
      /disconnected|stopped/i,
    );
    backend = await start(root);
    await page.goto(backend.url);
    await expect(
      page.getByRole("heading", { name: "Knowledge map" }),
    ).toBeVisible();
  });

  test("native GPU dense cloud evidence", async ({ page }, info) => {
    expect(process.platform).toBe("darwin");
    expect(process.arch).toBe("arm64");
    expect(process.env.AIOS_COMPLETION_REFERENCE_HARDWARE).toBeTruthy();
    await build(page, source);
    const canvas = page.locator("canvas[data-knowledge-cloud]");
    await expect(canvas).toBeVisible();
    const renderer = await canvas.evaluate((element: HTMLCanvasElement) => {
      const gl = element.getContext("webgl2") || element.getContext("webgl");
      if (!gl) return "unavailable";
      const extension = gl.getExtension("WEBGL_debug_renderer_info");
      return extension
        ? String(gl.getParameter(extension.UNMASKED_RENDERER_WEBGL))
        : "unknown";
    });
    expect(renderer).toMatch(/Apple|Metal/i);
    expect(renderer).not.toMatch(/SwiftShader|software/i);
    await drillWorker(page);
    const measurements = await canvas.evaluate(
      async (element: HTMLCanvasElement) => {
        const initial = JSON.parse(element.dataset.renderStats!);
        const frames: number[] = [];
        let previous = performance.now();
        for (let i = 0; i < 180; i++)
          await new Promise<void>((done) =>
            requestAnimationFrame((now) => {
              element.dispatchEvent(
                new KeyboardEvent("keydown", {
                  key: "ArrowRight",
                  bubbles: true,
                }),
              );
              frames.push(now - previous);
              previous = now;
              done();
            }),
          );
        const final = JSON.parse(element.dataset.renderStats!);
        return { initial, final, frames };
      },
    );
    expect(measurements.initial.nodes).toBe(256);
    expect(
      measurements.final.frames - measurements.initial.frames,
    ).toBeGreaterThanOrEqual(180);
    expect(measurements.final.context).toBe("ready");
    writeFileSync(
      info.outputPath("native-frame-measurements.json"),
      JSON.stringify({
        renderer,
        ...measurements,
        hardware: process.env.AIOS_COMPLETION_REFERENCE_HARDWARE,
        over16_7ms: measurements.frames.filter((ms) => ms > 16.7).length,
      }),
    );
    await info.attach("native-frame-measurements", {
      body: JSON.stringify({
        renderer,
        ...measurements,
        hardware: process.env.AIOS_COMPLETION_REFERENCE_HARDWARE,
        over16_7ms: measurements.frames.filter((ms) => ms > 16.7).length,
        meaning:
          "Frame intervals during 180 real camera updates; last_ms/max_ms are CPU submission, not GPU duration; bounded 256 of 1000+ entities.",
      }),
      contentType: "application/json",
    });
    await canvas.screenshot({
      path: info.outputPath("dense-native-cloud.png"),
    });
    await expect(page.getByLabel("Cloud performance")).toContainText(
      /detail|frame|LOD/i,
    );
    const found = await api<{ entities: { handle: string }[] }>(
      page,
      "/api/v1/query",
      { text: "Worker", repository: "fixture", limit: 1 },
    );
    const sourceBefore = readFileSync(join(source, "src", "Worker.java"));
    const opened = await api<{ opened: boolean }>(
      page,
      "/api/v1/source-actions/open",
      { handle: found.entities[0].handle, kind: "finder" },
    );
    expect(opened.opened).toBe(true);
    await expect
      .poll(() =>
        execFileSync(
          "osascript",
          [
            "-e",
            'tell application "Finder" to get POSIX path of (item 1 of (get selection) as alias)',
          ],
          { encoding: "utf8", timeout: 5000 },
        ).trim(),
      )
      .toBe(join(source, "src", "Worker.java"));
    expect(
      readFileSync(join(source, "src", "Worker.java")).equals(sourceBefore),
    ).toBe(true);
    writeFileSync(
      info.outputPath("native-finder-proof.json"),
      JSON.stringify({
        action: "finder_reveal",
        validatedCanonicalHandle: true,
        selectedCapturedFixtureFile: true,
        selectedSourceBytesUnchanged: true,
      }),
    );
  });
});
