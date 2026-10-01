import {expect, test} from "@playwright/test";

test.beforeEach(async({page})=>{await page.route("**/api/v1/jobs",route=>route.fulfill({json:{jobs:[],mirror_interval_seconds:900,durable:true}}))});

test.beforeEach(async({page})=>{await page.route("**/api/v1/onboarding",route=>route.fulfill({json:{state:"unconfigured",completed_repositories:0}}));await page.route("**/api/v1/instance",route=>route.fulfill({json:{id:"instance",name:"AgentOS",seed_colour:"#5865f2"}}))});

test("renders only canonical generation data and supports keyboard selection", async ({page}) => {
  await page.route("**/api/v1/session", route => route.fulfill({json:{csrf_token:"csrf"}}));
  await page.route("**/api/v1/status", route => route.fulfill({json:{projection_state:"ready",repositories:[{id:"repo",generation:"g",revision:"r",active:true}]}}));
  await page.route("**/api/v1/projection?repo=repo", route => route.fulfill({json:{repository:"repo",generation:"g",nodes:[{handle:"e",kind:"function",label:"Publish",identity:"",repository:"repo",path:"src/a.go",generation:"g",evidence:"v",confidence:.9,evidence_count:1,span:{start_line:1,end_line:1}}],edges:[{handle:"c",subject:"e",object:"e",predicate:"EMITS_EVENT",evidence:"v",confidence:.9,derivation:"syntax"}],truncated:false}}));
  await page.route("**/api/v1/entity", route => route.fulfill({json:{handle:"e",kind:"function",label:"Publish",repository:"repo",path:"src/a.go",generation:"g",evidence:"v",span:{start_line:1,end_line:1}}}));
  await page.route("**/api/v1/evidence", route => route.fulfill({json:{path:"src/a.go",start_line:1,end_line:1,lines:["func Publish() {}"]}}));
  await page.goto("/#token=test");
  await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible();
  const entity=page.getByRole("button",{name:"Publish",exact:true});await entity.focus();await page.keyboard.press("Enter");await expect(page.getByLabel("Evidence inspector")).toContainText("src/a.go");
});
test("reports no active generation without claiming loss",async({page})=>{await page.route("**/api/v1/session",route=>route.fulfill({json:{csrf_token:"csrf"}}));await page.route("**/api/v1/status",route=>route.fulfill({json:{projection_state:"no_active_generation",repositories:[]}}));await page.goto("/#token=test");await expect(page.getByText("No repository checkout has been changed.")).toBeVisible()});
test("keeps canonical knowledge safe when a projection is unavailable",async({page})=>{await page.route("**/api/v1/session",route=>route.fulfill({json:{csrf_token:"csrf"}}));await page.route("**/api/v1/status",route=>route.fulfill({json:{projection_state:"unavailable",repositories:[{id:"repo",active:true}]}}));await page.goto("/#token=test");await expect(page.getByText("Knowledge is safe")).toBeVisible();await expect(page.getByText("Canonical source evidence remains intact")).toBeVisible()});

test("saves and displays instance branding in the live entry point",async({page})=>{
 let instance={id:"stable",name:"AgentOS",seed_colour:"#5865f2"};
 await page.route("**/api/v1/instance",route=>{if(route.request().method()==="POST")instance={...instance,...route.request().postDataJSON()};return route.fulfill({json:instance})});
 await page.route("**/api/v1/session",r=>r.fulfill({json:{csrf_token:"csrf"}}));
 await page.route("**/api/v1/status",r=>r.fulfill({json:{projection_state:"no_active_generation",repositories:[]}}));
 await page.goto("/#token=test");await page.getByRole("button",{name:"Edit instance"}).click();await page.getByLabel("Instance name").fill("Research");await page.getByRole("button",{name:"Save instance"}).click();await expect(page.getByLabel("Knowledge instance")).toContainText("Research");expect(instance.id).toBe("stable");
});

test("first-run setup renders staged status before promotion",async({page})=>{
 let state="unconfigured",ready=false;
 await page.route("**/api/v1/session",r=>r.fulfill({json:{csrf_token:"csrf"}}));
 await page.route("**/api/v1/status",r=>r.fulfill({json:{projection_state:ready?"ready":"no_active_generation",repositories:[]}}));
 await page.route("**/api/v1/onboarding/configure",r=>{expect(r.request().headers()["x-csrf-token"]).toBe("csrf");return r.fulfill({json:{state:"configured"}})});
 await page.route("**/api/v1/onboarding/preview",r=>r.fulfill({json:{valid:true,repositories:[{id:"repo",valid:true,files:1,languages:["java"],frameworks:[],exclusions:{}}]}}));
 await page.route("**/api/v1/activity",r=>r.fulfill({json:{events:[]}}));
 await page.route("**/api/v1/onboarding/start",r=>{state="ingesting";return r.fulfill({status:202,json:{state,completed_repositories:1}})});
 await page.route("**/api/v1/onboarding",r=>r.fulfill({json:{state,completed_repositories:state==="ingesting"?1:0}}));
 await page.goto("/#token=test");await page.getByLabel("id 1",{exact:true}).fill("repo");await page.getByLabel("url 1",{exact:true}).fill("https://github.com/example/approved.git");await page.getByRole("button",{name:"Preview scope"}).click();await expect(page.getByText("Sources validated")).toBeVisible();await page.getByRole("button",{name:"Sync and index"}).click();await expect(page.getByText("1 mirrors synchronized. Compiling snapshots…")).toBeVisible();await expect(page.getByLabel("Repository setup")).not.toBeVisible();state="ready";ready=true;await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible();
});

test("Spotlight keyboard, truthful result states, cloud boundaries and stale evidence", async({page}) => {
 const entity={handle:"e",kind:"function",label:"Publish",identity:"",repository:"repo",path:"src/a.go",generation:"g",evidence:"v",confidence:.9,evidence_count:1,span:{start_line:1,end_line:1}};
 let state="found",stale=false;
 await page.route("**/api/v1/session",r=>r.fulfill({json:{csrf_token:"csrf"}}));
 await page.route("**/api/v1/status",r=>r.fulfill({json:{projection_state:"ready",repositories:[{id:"repo",generation:"g",active:true}]}}));
 await page.route("**/api/v1/projection?repo=repo",r=>r.fulfill({json:{repository:"repo",generation:"g",nodes:[entity],edges:[],truncated:true,applied_limits:{nodes:20,edges:20}}}));
 await page.route("**/api/v1/entity",r=>r.fulfill(stale?{status:409,json:{error:"handle is stale or unknown"}}:{json:entity}));
 await page.route("**/api/v1/evidence",r=>r.fulfill({json:{path:entity.path,generation:"g",git_commit:"commit",sha256:"hash",start_line:1,end_line:1,lines:["func Publish() {}"]}}));
 await page.route("**/api/v1/query",r=>{
  expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
  const kind=state==="empty"?"empty_query":state==="unsupported"?"unsupported_query":state==="unavailable"?"projection_unavailable":state==="unknown"?"coverage_incomplete":"deterministic_planner";
  return r.fulfill({json:{status:["found","truncated"].includes(state)?"found":state==="not_found"?"not_found":"unknown",entities:["found","truncated"].includes(state)?[entity]:[],trace:[{kind,detail:kind}],truncated:state==="truncated",coverage:{complete:!["unknown","unavailable"].includes(state),generations:["g"],repositories:["repo"]},applied_limits:{results:20}}});
 });
 await page.goto("/#token=test");await expect(page.getByLabel("Knowledge cloud",{exact:true})).toContainText("generation g · 1 nodes · 0 evidence-backed edges");
 const opener=page.getByRole("button",{name:"Open Spotlight"});await opener.focus();await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
 const dialog=page.getByRole("dialog"),input=page.getByRole("combobox",{name:"Spotlight query"});await expect(input).toBeFocused();
 await page.keyboard.press("Escape");await expect(dialog).not.toBeVisible();await expect(opener).toBeFocused();
 await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");await input.fill("Publish");await input.press("Enter");await expect(page.getByRole("dialog").getByRole("option")).toContainText("Publish");await input.press("ArrowDown");await input.press("Enter");await expect(dialog).not.toBeVisible();await expect(opener).toBeFocused();await expect(page.getByLabel("Evidence inspector")).toContainText("Captured commit: commit · SHA256: hash");
 for(const [next,feedback] of [["empty","Enter a query"],["unsupported","Unsupported query:"],["unavailable","Search projection unavailable or stale"],["unknown","Unknown: coverage is incomplete"],["not_found","Not found:"],["truncated","Found bounded results"]]) {
  state=next;await opener.click();await input.fill(next==="empty"?"":next);await dialog.getByRole("button",{name:"Search captured evidence"}).click();await expect(dialog.getByRole("status")).toContainText(feedback);await page.keyboard.press("Escape");
 }
 stale=true;await page.getByRole("button",{name:"Publish",exact:true}).click();await expect(page.getByRole("alert")).toContainText("Stale selection");await expect(page.getByLabel("Evidence inspector")).not.toContainText("func Publish");
});
