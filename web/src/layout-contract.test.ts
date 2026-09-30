import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

describe("desktop single-page layout contract", () => {
  const css = readFileSync(new URL("./style.css", import.meta.url), "utf8");

  it("locks the document only on a genuinely fitting desktop and lets the app reflow otherwise", () => {
    expect(css).toContain("@media (min-width: 1024px) and (min-height: 900px)");
    expect(css).toContain("html, body, #root { height: 100dvh; overflow: hidden; }");
    expect(css).toContain("[data-testid=\"onboarding\"], [data-testid=\"knowledge-dashboard\"] { height: 100dvh; max-height: 100dvh; min-height: 0; overflow: hidden;");
  });

  it("uses a visible compact queue without hiding dashboard content", () => {
    expect(css).toContain("grid-template-columns: 1fr");
    expect(css).not.toContain("[data-testid=\"knowledge-dashboard\"] > section:nth-of-type(n+5) { display: none; }");
    expect(css).not.toContain(".mcp-dialog");
    expect(css).not.toContain("@keyframes");
    expect(css).toContain("grid-template-rows: 56px minmax(0, 1fr)");
  });

  it("keeps the graph shell fixed while overlay panels retain stable desktop dimensions", () => {
    expect(css).toContain('[data-testid="knowledge-dashboard"] { height: 100dvh; max-height: 100dvh; min-height: 0; overflow: hidden;');
    expect(css).toContain('@media (max-width: 1023px), (max-height: 899px)');
    expect(css).not.toContain('[data-testid="knowledge-dashboard"] > section:nth-of-type(1) p { display: none; }');
  });
});
