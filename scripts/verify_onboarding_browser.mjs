// Actual installed daemon browser origins; private launch links arrive via stdin.
import {chromium} from "../web/node_modules/playwright/index.mjs";
import {execFileSync} from "node:child_process";
let input="";
for await (const chunk of process.stdin) {input+=chunk;if(input.length>16384)throw new Error("bounded private input");}
const args=JSON.parse(input);
let stage="launch",browser;
const cli=(...words)=>JSON.parse(execFileSync(args.binary,words,{encoding:"utf8",timeout:30000,stdio:["ignore","pipe","pipe"]}));
try {
  browser=await chromium.launch({headless:true});
  const page=await browser.newPage();
  page.setDefaultTimeout(15000);page.setDefaultNavigationTimeout(15000);
  async function open(link){
    await page.goto(link);
    await page.getByLabel("Knowledge instance").waitFor();
  }
  async function api(path,body){
    return page.evaluate(async({path,body})=>{
      const session=await fetch("/api/v1/session");if(!session.ok)throw new Error("browser session unavailable");
      const csrf=(await session.json()).csrf_token;
      const response=await fetch(path,body===undefined?{}:{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf},body:JSON.stringify(body)});
      if(!response.ok)throw new Error("authenticated browser API failed");
      return response.json();
    },{path,body});
  }
  stage="installed_named_origin";
  await open(args.launch_url);
  if(new URL(page.url()).hostname!==args.name+".local")throw new Error("named origin changed");
  const first=await api("/api/v1/namespace");
  if(!first.active||!first.native_dns_sd||!first.local_only)throw new Error("inactive native namespace");
  if((await api("/api/v1/query",{repository:"fixture",text:"Worker"})).status!=="found")throw new Error("last-good evidence unavailable");
  stage="old_named_origin_to_new";
  const next=await api("/api/v1/namespace",{namespace:args.name+"-next"});
  await open(next.launch_url);
  if(new URL(page.url()).hostname!==args.name+"-next.local")throw new Error("new origin not delivered");
  stage="localhost_recovery";
  await open(cli("daemon","link","--root",args.root,"--recovery").url);
  if(new URL(page.url()).hostname!=="localhost")throw new Error("recovery origin changed");
  if((await api("/api/v1/namespace")).namespace!==args.name+"-next")throw new Error("recovery renamed namespace");
  stage="native_restart_status";
  const identity=cli("daemon","status","--root",args.root).instance_id;
  stage="native_restart_stop";
  cli("daemon","stop","--root",args.root);
  stage="native_restart_start";
  if(cli("daemon","start","--root",args.root).instance_id!==identity)throw new Error("restart identity changed");
  stage="native_restart_link";
  const restartedLink=cli("daemon","link","--root",args.root).url;
  stage="native_restart_named_origin";
  await open(restartedLink);
  stage="native_restart_hostname";
  if(new URL(page.url()).hostname!==args.name+"-next.local")throw new Error("restart fell back to recovery");
  stage="native_restart_namespace";
  const restored=await api("/api/v1/namespace");
  if(!restored.active||!restored.native_dns_sd||restored.namespace!==args.name+"-next")throw new Error("native name not restored");
  stage="native_restart_knowledge";
  if((await api("/api/v1/query",{repository:"fixture",text:"Worker"})).status!=="found")throw new Error("restart lost knowledge");
  process.stdout.write(JSON.stringify({passed:true,client:"real_chromium_browser",checks:["installed_named_origin_authenticated","old_name_to_new_name_capability_migration","localhost_recovery","active_native_namespace_identity_knowledge_restart"]})+"\n");
} catch(error) {
  // Browser errors can include launch URLs. Never persist their raw text.
  process.stderr.write("Native browser verification failed at "+stage+"\n");
  process.exitCode=1;
} finally {if(browser)await browser.close();}
