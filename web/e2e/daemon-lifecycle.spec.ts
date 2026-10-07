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

test("real logo and namespace selection migrate origins, recover and persist without automatic rename", async ({page,context}) => {
  test.setTimeout(90000);
  const root=realpathSync(mkdtempSync(join(tmpdir(),"aios-onboarding-browser-")));
  const other=realpathSync(mkdtempSync(join(tmpdir(),"aios-namespace-collision-")));
  let child:ChildProcess|undefined,second:ChildProcess|undefined;
  try {
    child=await launch(root);await page.goto(await fresh(root));
    await page.getByRole("button",{name:"Edit instance"}).click();
    await page.getByRole("button",{name:"Generate local logo"}).click();
    await expect(page.getByLabel("Knowledge instance").locator("img")).toBeVisible();
    await page.getByLabel("Instance name").fill("Onboarding fixture");
    await page.getByRole("button",{name:"Save instance"}).click();
    const saved=JSON.parse(readFileSync(join(root,"data","instance.json"),"utf8"));
    expect(saved.logo).toMatch(/^data:image\/png;base64,/);
    const name=`aios-browser-${process.pid}-${Date.now()}`;
    await expect(page.getByLabel("Namespace",{exact:true})).toHaveValue("");
    await page.getByLabel("Namespace",{exact:true}).fill(name);
    await page.getByRole("button",{name:"Use namespace",exact:true}).click();
    await expect.poll(()=>new URL(page.url()).hostname).toBe(`${name}.${process.platform==="darwin"?"local":"localhost"}`);
    await expect(page.getByRole("heading",{name:"No active generation"})).toBeVisible();
    // Rename from the old named origin, after its registration is released.
    await expect(page.getByLabel("Namespace",{exact:true})).toHaveValue(name);
    await page.getByLabel("Namespace",{exact:true}).fill(name+"-next");
    await page.getByRole("button",{name:"Use namespace",exact:true}).click();
    await expect.poll(()=>new URL(page.url()).hostname).toBe(`${name}-next.${process.platform==="darwin"?"local":"localhost"}`);
    await expect(page.getByLabel("Knowledge instance")).toContainText("Onboarding fixture");
    second=await launch(other);const alternative=await context.newPage();await alternative.goto(await fresh(other));
    await expect(alternative.getByLabel("Namespace",{exact:true})).toHaveValue("");
    await alternative.getByLabel("Namespace",{exact:true}).fill(name+"-next");
    await alternative.getByRole("button",{name:"Use namespace",exact:true}).click();
    await expect(alternative.getByLabel("Namespace alternatives")).toBeVisible();
    expect(()=>readFileSync(join(other,"data","namespace.json"))).toThrow();
    const option=alternative.getByLabel("Namespace alternatives").getByRole("button").first();
    const selected=(await option.textContent())!.replace("Select ","");await option.click();
    expect(()=>readFileSync(join(other,"data","namespace.json"))).toThrow();
    await alternative.getByRole("button",{name:"Use namespace",exact:true}).click();
    await expect.poll(()=>new URL(alternative.url()).hostname).toBe(`${selected}.${process.platform==="darwin"?"local":"localhost"}`);
    const recovery=JSON.parse(execFileSync(process.env.AIOS_UI_BINARY||resolve("../bin/aios"),["daemon","link","--root",root,"--recovery"],{timeout:5000}).toString()).url;
    await page.goto(recovery);await expect(page.getByLabel("Namespace",{exact:true})).toHaveValue(name+"-next");
    await stop(child);child=await launch(root);await page.goto(await fresh(root));
    await expect(page.getByLabel("Knowledge instance")).toContainText("Onboarding fixture");
    await expect(page.getByLabel("Knowledge instance").locator("img")).toBeVisible();
    expect(JSON.parse(readFileSync(join(root,"data","instance.json"),"utf8")).id).toBe(saved.id);
    await expect(page.getByLabel("Namespace",{exact:true})).toHaveValue(name+"-next");
  } finally {
    if(child)await stop(child);if(second)await stop(second);
    rmSync(root,{recursive:true,force:true});rmSync(other,{recursive:true,force:true});
  }
});
