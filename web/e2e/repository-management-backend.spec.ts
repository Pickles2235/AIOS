import {test,expect} from "@playwright/test";
import {spawn,execFileSync,type ChildProcess} from "node:child_process";
import {mkdtempSync,realpathSync,writeFileSync,readFileSync,rmSync,chmodSync,readdirSync,statSync} from "node:fs";
import {tmpdir} from "node:os";
import {join,resolve} from "node:path";

function writable(path:string){if(statSync(path).isDirectory()){chmodSync(path,0o700);for(const name of readdirSync(path))writable(join(path,name))}}
async function stop(child:ChildProcess){if(child.exitCode!==null||child.signalCode!==null)return;await new Promise<void>((done,fail)=>{const kill=setTimeout(()=>child.kill("SIGKILL"),5000),timeout=setTimeout(()=>fail(new Error("backend cleanup timeout")),10000);child.once("exit",()=>{clearTimeout(kill);clearTimeout(timeout);done()});child.kill("SIGTERM")})}

for(const mode of ["local","mirror"])test(`real ${mode} repository add scope rebuild and removal controls`,async({page})=>{
  test.setTimeout(120000);
  const root=realpathSync(mkdtempSync(join(tmpdir(),"aios-management-browser-"))),source=join(root,"source"),added=join(root,"added"),data=join(root,"data");
  for(const [path,name] of [[source,"OriginalWorker"],[added,"SecondWorker"]]){
    execFileSync("git",["init","-q","--initial-branch=main",path]);writeFileSync(join(path,"Worker.java"),`class ${name} {}\n`);
    execFileSync("git",["-C",path,"add","."]);execFileSync("git",["-C",path,"-c","user.name=fixture","-c","user.email=fixture@example.test","commit","-qm","one"]);
  }
  const before=[source,added].map(p=>readFileSync(join(p,".git","index")));
  const child=spawn(process.env.AIOS_UI_BINARY||resolve("../bin/aios"),["ui","serve","--data-dir",data],{stdio:["ignore","ignore","pipe"]});
  try{
    const url=await new Promise<string>((done,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr!.on("data",chunk=>{output+=chunk;const m=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(m){clearTimeout(timer);done(m[1])}});child.once("exit",()=>{clearTimeout(timer);fail(new Error("backend exited"))})});
    await page.goto(url);if(mode==="local")await page.getByLabel("Source type").selectOption(mode);
    await page.getByLabel("id 1",{exact:true}).fill("fixture");await page.getByLabel("url 1",{exact:true}).fill(source);
    await page.getByRole("button",{name:"Preview scope",exact:true}).click();await expect(page.getByText("Sources validated",{exact:true})).toBeVisible();await page.getByRole("button",{name:"Sync and index",exact:true}).click();
    const status=()=>page.evaluate(async()=>{const r=await fetch("/api/v1/status");return r.json()});
    const jobs=()=>page.evaluate(async()=>{const r=await fetch("/api/v1/jobs");return r.json()});
    await expect.poll(async()=>(await jobs()).jobs[0]?.state,{timeout:30000}).toBe("idle");
    const identity=await page.evaluate(async()=>(await(await fetch("/api/v1/instance")).json()).id);
    const management=page.getByLabel("Repository management");await management.getByRole("button",{name:"Manage repositories",exact:true}).click();
    await management.getByLabel("New repository ID",{exact:true}).fill("added");await management.getByLabel(mode==="local"?"New workspace path":"New Git URL",{exact:true}).fill(added);await management.getByRole("button",{name:"Approve and index repository",exact:true}).click();
    await expect.poll(async()=>(await status()).repositories.find((r:{id:string})=>r.id==="added")?.active,{timeout:30000}).toBe(true);
    await expect(management.getByLabel("Manage added",{exact:true})).toBeVisible();
    const first=(await status()).repositories.find((r:{id:string})=>r.id==="added").generation;
    await management.getByLabel("Exclude patterns for added",{exact:true}).fill("Worker.java");await management.getByRole("button",{name:"Save scope for added",exact:true}).click();
    await expect.poll(async()=>(await status()).repositories.find((r:{id:string})=>r.id==="added")?.generation,{timeout:30000}).not.toBe(first);
    const scoped=(await status()).repositories.find((r:{id:string})=>r.id==="added").generation;
    await management.getByRole("button",{name:"Force rebuild added",exact:true}).click();await expect.poll(async()=>(await status()).repositories.find((r:{id:string})=>r.id==="added")?.generation,{timeout:30000}).not.toBe(scoped);
    await expect(management.getByRole("button",{name:"Rebuild knowledge base",exact:true})).toBeEnabled();
    const allBefore=(await status()).repositories.map((r:{generation:string})=>r.generation);
    await management.getByRole("button",{name:"Rebuild knowledge base",exact:true}).click();await expect.poll(async()=>(await status()).repositories.map((r:{generation:string})=>r.generation).every((g:string,i:number)=>g!==allBefore[i]),{timeout:30000}).toBe(true);
    await page.getByLabel("Query",{exact:true}).fill("OriginalWorker");await page.getByRole("button",{name:"Search",exact:true}).click();await expect(page.getByLabel("Evidence inspector")).toContainText("class OriginalWorker {}");
    await management.getByRole("button",{name:"Remove fixture",exact:true}).click();await expect(management.getByLabel("Confirm repository removal")).toContainText("Its source workspace stays on disk");await management.getByRole("button",{name:"Confirm remove fixture",exact:true}).click();
    await expect.poll(async()=>(await status()).repositories.some((r:{id:string})=>r.id==="fixture")).toBe(false);
    await expect(page.getByLabel("Evidence inspector")).toContainText("Select an entity");await expect(page.getByLabel("Evidence inspector")).not.toContainText("OriginalWorker");
    const purge=await page.evaluate(async()=>{const r=await fetch("/api/v1/repositories/purge-status?repository=fixture");return r.json()});expect(purge.owned_records).toBe(0);
    expect(await page.evaluate(async()=>(await(await fetch("/api/v1/instance")).json()).id)).toBe(identity);
    expect(readFileSync(join(source,"Worker.java"),"utf8")).toBe("class OriginalWorker {}\n");expect(readFileSync(join(added,"Worker.java"),"utf8")).toBe("class SecondWorker {}\n");for(const [i,p] of [source,added].entries())expect(readFileSync(join(p,".git","index"))).toEqual(before[i]);
  }finally{await stop(child);writable(root);rmSync(root,{recursive:true,force:true})}
});
