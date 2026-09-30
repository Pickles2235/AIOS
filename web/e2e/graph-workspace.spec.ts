import {expect, test} from "@playwright/test";

test("renders only canonical generation data and supports keyboard selection", async ({page}) => {
  await page.route("**/api/v1/session", route => route.fulfill({json:{csrf_token:"csrf"}}));
  await page.route("**/api/v1/status", route => route.fulfill({json:{projection_state:"ready",repositories:[{id:"repo",generation:"g",revision:"r",active:true}]}}));
  await page.route("**/api/v1/projection?repo=repo", route => route.fulfill({json:{repository:"repo",generation:"g",nodes:[{handle:"e",kind:"function",label:"Publish",identity:"",repository:"repo",path:"src/a.go",generation:"g",evidence:"v",confidence:.9,evidence_count:1,span:{start_line:1,end_line:1}}],edges:[{handle:"c",subject:"e",object:"e",predicate:"EMITS_EVENT",evidence:"v",confidence:.9,derivation:"syntax"}],truncated:false}}));
  await page.route("**/api/v1/evidence", route => route.fulfill({json:{path:"src/a.go",start_line:1,end_line:1,lines:["func Publish() {}"]}}));
  await page.goto("/#token=test");
  await expect(page.getByRole("heading",{name:"Knowledge map"})).toBeVisible();
  const entity=page.getByRole("button",{name:"Publish"});await entity.focus();await page.keyboard.press("Enter");await expect(page.getByLabel("Evidence inspector")).toContainText("src/a.go");
});
test("reports no active generation without claiming loss",async({page})=>{await page.route("**/api/v1/session",route=>route.fulfill({json:{csrf_token:"csrf"}}));await page.route("**/api/v1/status",route=>route.fulfill({json:{projection_state:"no_active_generation",repositories:[]}}));await page.goto("/#token=test");await expect(page.getByText("No repository checkout has been changed.")).toBeVisible()});
test("keeps canonical knowledge safe when a projection is unavailable",async({page})=>{await page.route("**/api/v1/session",route=>route.fulfill({json:{csrf_token:"csrf"}}));await page.route("**/api/v1/status",route=>route.fulfill({json:{projection_state:"unavailable",repositories:[{id:"repo",active:true}]}}));await page.goto("/#token=test");await expect(page.getByText("Knowledge is safe")).toBeVisible();await expect(page.getByText("Canonical source evidence remains intact")).toBeVisible()});
