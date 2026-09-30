import { describe, expect, it } from "vitest";
import { buttonVariant, noticeRole } from "./component-state";

describe("component variants", () => {
  it("keeps primary and secondary actions distinct", () => {
    expect(buttonVariant("primary")).toBe("primary");
    expect(buttonVariant("secondary")).toBe("secondary");
    expect(buttonVariant("quiet")).toBe("quiet");
  });

  it("only announces errors urgently", () => {
    expect(noticeRole("error")).toBe("alert");
    expect(noticeRole("info")).toBeUndefined();
  });
});
