import {test,expect} from "@playwright/test";
import {spawn,execFileSync} from "node:child_process";
import {mkdtempSync,writeFileSync,rmSync,chmodSync,readdirSync,statSync} from "node:fs";
import {tmpdir} from "node:os";
import {resolve,join} from "node:path";
function writable(path:string){if(statSync(path).isDirectory()){chmodSync(path,0o700);for(const name of readdirSync(path))writable(join(path,name))}}
test("authenticated first run syncs real fixture mirrors and cites source evidence",async({page})=>{
 const root=mkdtempSync(join(tmpdir(),"homefold-browser-")),source=join(root,"source");
 execFileSync("git",["init","-q","--initial-branch=main",source]);writeFileSync(join(source,"Publish.java"),"class Publish {}\n");
 execFileSync("git",["-C",source,"add","."]);execFileSync("git",["-C",source,"-c","user.name=fixture","-c","user.email=fixture@example.test","commit","-qm","one"]);
 const before=execFileSync("git",["-C",source,"status","--porcelain=v1"]).toString();
 const child=spawn(resolve("../bin/aios"),["ui","serve","--data-dir",join(root,"data")],{env:{...process.env,PATH:`/workspace/toolchains/jdk/bin:${process.env.PATH}`},stdio:["ignore","ignore","pipe"]});
 try{
  const url=await new Promise<string>((ok,fail)=>{let output="";const timer=setTimeout(()=>fail(new Error("backend startup timeout")),15000);child.stderr.on("data",data=>{output+=data;const match=output.match(/Open local UI: (http:\/\/[^\s]+)/);if(match){clearTimeout(timer);ok(match[1])}});child.on("exit",code=>{clearTimeout(timer);fail(new Error(`backend exited ${code}`))})});
  await page.goto(url);await page.getByLabel("id 1",{exact:true}).fill("fixture");await page.getByLabel("url 1",{exact:true}).fill(source);await page.getByRole("button",{name:"Sync and index"}).click();await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible({timeout:30000});
  await page.getByLabel("Query",{exact:true}).fill("Publish");await page.getByRole("button",{name:"Search",exact:true}).click();await expect(page.getByLabel("Evidence inspector")).toContainText("Publish.java");await expect(page.getByLabel("Evidence inspector")).toContainText("class Publish {}");
  expect(execFileSync("git",["-C",source,"status","--porcelain=v1"]).toString()).toBe(before);
 }finally{child.kill("SIGTERM");await new Promise<void>(done=>{if(child.exitCode!==null)done();else child.once("exit",()=>done())});writable(root);rmSync(root,{recursive:true,force:true})}
});
