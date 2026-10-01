import {test, expect} from "@playwright/test";
import {spawn, execFileSync, type ChildProcess} from "node:child_process";
import {mkdtempSync, realpathSync, rmSync, readFileSync} from "node:fs";
import {tmpdir} from "node:os";
import {join, resolve} from "node:path";

async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  await new Promise<void>((done, fail) => {
    const kill = setTimeout(() => child.kill("SIGKILL"), 5000);
    const timeout = setTimeout(() => {clearTimeout(kill); fail(new Error("Daemon cleanup timeout"));}, 10000);
    child.once("exit", () => {clearTimeout(kill); clearTimeout(timeout); done();});
    child.kill("SIGTERM");
  });
}
async function launch(root: string) {
  const child = spawn(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),
    ["daemon", "run", "--root", root, "--service-label", "dev.aios.completion.browser"], {stdio: ["ignore", "ignore", "pipe"]});
  try {
    await expect.poll(() => {
      try {return JSON.parse(execFileSync(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),
        ["daemon", "status", "--root", root], {timeout: 5000}).toString()).running;} catch {return false;}
    }, {timeout: 15000}).toBe(true);
    return child;
  } catch (error) {await stop(child); throw error;}
}
async function fresh(root: string): Promise<string> {
  return JSON.parse(execFileSync(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),
    ["daemon", "link", "--root", root], {timeout: 5000}).toString()).url;
}

test("real headless daemon survives browser closure, restarts identity and stops from UI", async ({page, context}) => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "aios-daemon-browser-")));
  let child: ChildProcess | undefined;
  try {
    child = await launch(root); await page.goto(await fresh(root));
    await expect(page.getByRole("heading", {name: "No active generation"})).toBeVisible();
    const identity = JSON.parse(readFileSync(join(root, "data", "instance.json"), "utf8")).id;
    await page.close();
    const reopened = await context.newPage(); await reopened.goto(await fresh(root));
    await expect(reopened.getByRole("heading", {name: "No active generation"})).toBeVisible();
    await stop(child); child = await launch(root);
    expect(JSON.parse(readFileSync(join(root, "data", "instance.json"), "utf8")).id).toBe(identity);
    await reopened.goto(await fresh(root));
    await reopened.getByRole("button", {name: "Stop daemon", exact: true}).click();
    await expect(reopened.getByRole("alert")).toContainText("Daemon disconnected", {timeout: 10000});
    await expect(reopened.getByRole("alert")).toContainText("aios daemon start");
    await expect.poll(() => child!.exitCode !== null || child!.signalCode !== null).toBe(true);
  } finally {if (child) await stop(child); rmSync(root, {recursive: true, force: true});}
});

test("stop failure and session expiry are distinguished from disconnected daemon", async ({page}) => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "aios-daemon-errors-")));
  let child: ChildProcess | undefined;
  try {
    child = await launch(root); await page.goto(await fresh(root));
    await expect(page.getByRole("heading", {name: "No active generation"})).toBeVisible();
    await page.route("**/api/v1/daemon/stop", route => route.fulfill({json: {stopping: true}}));
    await page.route("**/api/v1/daemon/status", route => route.fulfill({json: {state: "failed"}}));
    await page.getByRole("button", {name: "Stop daemon", exact: true}).click();
    await expect(page.getByRole("alert")).toContainText("Stop failed", {timeout: 7000});
    await expect(page.getByRole("button", {name: "Stop daemon", exact: true})).toBeEnabled();
    await page.route("**/api/v1/session", route => route.fulfill({status: 403, json: {error: "session expired"}}));
    await expect(page.getByRole("alert")).toContainText("session has expired", {timeout: 7000});
    await expect(page.getByRole("alert")).not.toContainText("Daemon disconnected");
  } finally {if (child) await stop(child); rmSync(root, {recursive: true, force: true});}
});
