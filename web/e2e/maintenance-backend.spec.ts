import {test,expect} from "@playwright/test";
import {spawn,execFileSync,type ChildProcess} from "node:child_process";
import {mkdtempSync,realpathSync,writeFileSync,readFileSync,rmSync,chmodSync,readdirSync,statSync} from "node:fs";
import {tmpdir} from "node:os";
import {join,resolve} from "node:path";

function writable(path:string){if(statSync(path).isDirectory()){chmodSync(path,0o700);for(const name of readdirSync(path))writable(join(path,name))}}
async function stop(child:ChildProcess){if(child.exitCode!==null||child.signalCode!==null)return;await new Promise<void>((done,fail)=>{const kill=setTimeout(()=>child.kill("SIGKILL"),5000),timeout=setTimeout(()=>fail(new Error("backend cleanup timeout")),10000);child.once("exit",()=>{clearTimeout(kill);clearTimeout(timeout);done()});child.kill("SIGTERM")})}

for(const mode of ["local","mirror"])test(`real ${mode} maintenance health controls and changed evidence`,async({page})=>{
  test.setTimeout(60000);
  const root=realpathSync(mkdtempSync(join(tmpdir(),"aios-maintenance-browser-"))),source=join(root,"source"),data=join(root,"data");
  const git=(...args:string[])=>execFileSync("git",["-C",source,...args]);
  execFileSync("git",["init","-q","--initial-branch=main",source]);
  writeFileSync(join(source,"Worker.java"),"class OriginalWorker {}\n");git("add",".");git("-c","user.name=fixture","-c","user.email=fixture@example.test","commit","-qm","one");
  const index=readFileSync(join(source,".git","index"));
  const child=spawn(process.env.AIOS_UI_BINARY||resolve("../bin/aios"),["ui","serve","--data-dir",data],{stdio:["ignore","ignore","pipe"]});
  try{
    const url=await new Promise<string>((done,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr!.on("data",chunk=>{output+=chunk;const m=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(m){clearTimeout(timer);done(m[1])}});child.once("exit",()=>{clearTimeout(timer);fail(new Error("backend exited"))})});
    await page.goto(url);if(mode==="local")await page.getByLabel("Source type").selectOption(mode);
    await page.getByLabel("id 1",{exact:true}).fill("fixture");await page.getByLabel("url 1",{exact:true}).fill(source);
    await page.getByRole("button",{name:"Preview scope",exact:true}).click();await expect(page.getByText("Sources validated",{exact:true})).toBeVisible();await page.getByRole("button",{name:"Sync and index",exact:true}).click();
    const jobs=()=>page.evaluate(async()=>{const r=await fetch("/api/v1/jobs");return r.json()});
    await expect.poll(async()=>{const s=await jobs();return s.jobs[0]?.state},{timeout:30000}).toBe("idle");
    const first=(await jobs()).jobs[0];expect(first.active_generation).toBeTruthy();
    const health=page.getByLabel("Repository maintenance");await expect(health).toBeVisible();await health.locator("summary").click();
    const policy=await page.evaluate(async()=>{const r=await fetch("/api/v1/resources");return r.json()});
    expect(["normal","constrained","idle_opportunity"]).toContain(policy.state);
    expect(policy.max_workers).toBe(1);expect(policy.max_queue).toBe(100);
    expect(policy.max_owned_bytes).toBeGreaterThan(policy.owned_bytes);
    await expect(health.getByLabel("Resource policy")).toContainText("maintenance journal");
    await expect(health.getByRole("article")).toContainText(mode==="local"?"Working tree":"Mirror");
    if(mode==="mirror"){
      await expect(health.getByLabel("Mirror check interval (seconds)")).toHaveValue("900");
      await health.getByLabel("Mirror check interval (seconds)").fill("120");await health.getByRole("button",{name:"Save check interval"}).click();await expect.poll(async()=>(await jobs()).mirror_interval_seconds).toBe(120);
    }
    writeFileSync(join(source,"Worker.java"),"class UpdatedWorker {}\n");
    if(mode==="mirror"){git("add",".");git("-c","user.name=fixture","-c","user.email=fixture@example.test","commit","-qm","updated")}
    const expectedIndex=mode==="local"?index:readFileSync(join(source,".git","index"));
    await health.getByRole("button",{name:"Check now for fixture",exact:true}).click();
    await expect.poll(async()=>(await jobs()).jobs[0].active_generation,{timeout:30000}).not.toBe(first.active_generation);
    const generation=(await jobs()).jobs[0].active_generation;
    await expect(page.getByLabel("Knowledge cloud",{exact:true})).toContainText(generation,{timeout:10000});
    await page.getByLabel("Query",{exact:true}).fill("UpdatedWorker");await page.getByRole("button",{name:"Search",exact:true}).click();await expect(page.getByLabel("Evidence inspector")).toContainText("class UpdatedWorker {}");
    await expect(page.getByLabel("Evidence inspector")).toContainText(mode==="local"?"Working-tree snapshot · Git HEAD:":"Captured commit:");
    expect(readFileSync(join(source,"Worker.java"),"utf8")).toBe("class UpdatedWorker {}\n");expect(readFileSync(join(source,".git","index"))).toEqual(expectedIndex);
  }finally{await stop(child);writable(root);rmSync(root,{recursive:true,force:true})}
});
