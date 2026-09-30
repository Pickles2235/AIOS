import {test,expect} from "@playwright/test";
import {spawn,execFileSync} from "node:child_process";
import {mkdtempSync,writeFileSync,rmSync,chmodSync,readdirSync,statSync,realpathSync,readFileSync} from "node:fs";
import {tmpdir} from "node:os";
import {resolve,join} from "node:path";
function writable(path:string){if(statSync(path).isDirectory()){chmodSync(path,0o700);for(const name of readdirSync(path))writable(join(path,name))}}
for(const mode of ["mirror","local"]) test(`authenticated first run captures real ${mode} fixtures and cites source evidence`,async({page})=>{
 const root=realpathSync(mkdtempSync(join(tmpdir(),"homefold-browser-"))),source=join(root,"source");
 execFileSync("git",["init","-q","--initial-branch=main",source]);writeFileSync(join(source,"Publish.java"),"class Publish {}\n");
 execFileSync("git",["-C",source,"add","."]);execFileSync("git",["-C",source,"-c","user.name=fixture","-c","user.email=fixture@example.test","commit","-qm","one"]);
 const before=execFileSync("git",["-C",source,"status","--porcelain=v1"]).toString();
 let child=spawn(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),["ui","serve","--data-dir",join(root,"data")],{env:{...process.env,PATH:`/workspace/toolchains/jdk/bin:${process.env.PATH}`},stdio:["ignore","ignore","pipe"]});
 try{
  const url=await new Promise<string>((ok,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr.on("data",data=>{output+=data;const match=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(match){clearTimeout(timer);ok(match[1])}});child.on("exit",code=>{clearTimeout(timer);fail(new Error(`backend exited ${code}`))})});
  await page.goto(url);if(mode==="local")await page.getByLabel("Source type").selectOption("local");await page.getByLabel("id 1",{exact:true}).fill("fixture");await page.getByLabel("url 1",{exact:true}).fill(source);await page.getByRole("button",{name:"Sync and index"}).click();await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible({timeout:30000});
  await page.getByRole("button",{name:"Edit instance"}).click();await page.getByLabel("Instance name").fill("Native fixture");await page.getByRole("button",{name:"Save instance"}).click();
  await page.getByLabel("Query",{exact:true}).fill("Publish");await page.getByRole("button",{name:"Search",exact:true}).click();await expect(page.getByLabel("Evidence inspector")).toContainText("Publish.java");await expect(page.getByLabel("Evidence inspector")).toContainText("class Publish {}");
  const opener=page.getByRole("button",{name:"Open Spotlight"});await opener.focus();await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
  const input=page.getByRole("combobox",{name:"Spotlight query"});await expect(input).toBeFocused();await input.fill("Publish");await input.press("Enter");await expect(page.getByRole("dialog").getByRole("option").first()).toContainText("Publish");await input.press("ArrowDown");await input.press("Enter");await expect(page.getByRole("dialog")).not.toBeVisible();await expect(opener).toBeFocused();await expect(page.getByLabel("Evidence inspector")).toContainText("Captured commit:");
  await page.reload();await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible();await expect(page.getByLabel("Knowledge cloud",{exact:true})).toContainText("fixture · generation");
  const instanceID=JSON.parse(readFileSync(join(root,"data","instance.json"),"utf8")).id;
  child.kill("SIGTERM");await new Promise<void>(done=>{if(child.exitCode!==null)done();else child.once("exit",()=>done())});
  execFileSync(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),["projections","rebuild","--data-dir",join(root,"data")]);
  child=spawn(process.env.AIOS_UI_BINARY || resolve("../bin/aios"),["ui","serve","--data-dir",join(root,"data")],{env:process.env,stdio:["ignore","ignore","pipe"]});
  const restarted=await new Promise<string>((ok,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("restart timeout")),15000);child.stderr.on("data",data=>{output+=data;const match=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(match){clearTimeout(timer);ok(match[1])}});child.on("exit",code=>{clearTimeout(timer);fail(new Error(`backend exited ${code}`))})});
  await page.goto(restarted);await expect(page.getByLabel("Knowledge instance")).toContainText("Native fixture");await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible();
  await page.getByLabel("Query",{exact:true}).fill("Publish");await page.getByRole("button",{name:"Search",exact:true}).click();await expect(page.getByLabel("Evidence inspector")).toContainText("class Publish {}");
  expect(JSON.parse(readFileSync(join(root,"data","instance.json"),"utf8")).id).toBe(instanceID);
  expect(execFileSync("git",["-C",source,"status","--porcelain=v1"]).toString()).toBe(before);
 }finally{child.kill("SIGTERM");await new Promise<void>(done=>{if(child.exitCode!==null)done();else child.once("exit",()=>done())});writable(root);rmSync(root,{recursive:true,force:true})}
});
