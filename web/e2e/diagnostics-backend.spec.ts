import {test, expect} from "@playwright/test";
import {spawn, type ChildProcess} from "node:child_process";
import {mkdtempSync, realpathSync, rmSync, readFileSync} from "node:fs";
import {tmpdir} from "node:os";
import {join, resolve} from "node:path";

async function stop(child:ChildProcess) {
  if(child.exitCode!==null||child.signalCode!==null)return;
  await new Promise<void>((done,fail)=>{const timer=setTimeout(()=>fail(new Error("backend stop timeout")),10000);child.once("exit",()=>{clearTimeout(timer);done()});child.kill("SIGTERM")});
}

test("live diagnostic details and warned redacted archive",async({page})=>{
  const root=realpathSync(mkdtempSync(join(tmpdir(),"aios-diagnostics-browser-")));
  const child=spawn(process.env.AIOS_UI_BINARY||resolve("../bin/aios"),["ui","serve","--data-dir",join(root,"data")],{stdio:["ignore","ignore","pipe"]});
  try {
    const url=await new Promise<string>((done,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr!.on("data",chunk=>{output+=chunk;const match=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(match){clearTimeout(timer);done(match[1])}});child.once("exit",()=>{clearTimeout(timer);fail(new Error("backend exited"))})});
    await page.goto(url);
    await expect(page.getByRole("heading",{name:"No active generation"})).toBeVisible();
    const panel=page.getByLabel("Local diagnostics");
    await panel.getByRole("button",{name:"Diagnostics"}).click();
    await expect(panel).toContainText("Telemetry stays on this Mac");
    await expect(panel).toContainText("knowledge-ir-v10");
    await expect(panel).toContainText("Coverage: 0 active");
    const exportButton=panel.getByRole("button",{name:"Download diagnostic archive"});
    await expect(exportButton).toBeEnabled();
    await panel.getByLabel("Include expanded developer diagnostics").check();
    await expect(panel.getByRole("alert")).toContainText("Review the archive before sharing it");
    await expect(exportButton).toBeDisabled();
    await panel.getByLabel("I understand the expanded diagnostic warning").check();
    const download=page.waitForEvent("download");await exportButton.click();
    const saved=await download;expect(saved.suggestedFilename()).toBe("aios-diagnostics.zip");
    const path=await saved.path();expect(path).toBeTruthy();expect(readFileSync(path!).subarray(0,2).toString()).toBe("PK");
    await expect(panel.getByRole("status")).toContainText("downloaded");
    const rejected=await page.evaluate(async()=>{const response=await fetch("/api/v1/diagnostics/export",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({expanded:true,acknowledge_warning:true})});return response.status});
    expect(rejected).toBe(403);
  } finally {await stop(child);rmSync(root,{recursive:true,force:true})}
});

test("a delayed onboarding read preserves the user's direct-source choice",async({page})=>{
  const root=realpathSync(mkdtempSync(join(tmpdir(),"aios-setup-intent-browser-")));
  const child=spawn(process.env.AIOS_UI_BINARY||resolve("../bin/aios"),["ui","serve","--data-dir",join(root,"data")],{stdio:["ignore","ignore","pipe"]});
  try {
    const url=await new Promise<string>((done,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr!.on("data",chunk=>{output+=chunk;const match=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(match){clearTimeout(timer);done(match[1])}});child.once("exit",()=>{clearTimeout(timer);fail(new Error("backend exited"))})});
    let reads=0;let release!:()=>void;const held=new Promise<void>(done=>{release=done});
    await page.route("**/api/v1/onboarding",async route=>{reads++;if(reads===2)await held;await route.continue()});
    await page.goto(url);
    await expect(page.getByRole("heading",{name:"No active generation"})).toBeVisible();
    await expect.poll(()=>reads).toBe(2);
    const mode=page.getByLabel("Source type");
    await mode.selectOption("local");
    await expect(mode).toHaveValue("local");
    const restored=page.waitForResponse(response=>response.url().endsWith("/api/v1/onboarding")&&response.status()===200);
    release();
    await restored;
    await expect(mode).toHaveValue("local");
    await expect(page.getByLabel("Repository setup")).toContainText("Absolute workspace path");
  } finally {await stop(child);rmSync(root,{recursive:true,force:true})}
});
