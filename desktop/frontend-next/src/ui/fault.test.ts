import { afterEach, describe, expect, it, vi } from "vitest";
import { faultDetail, noteFault } from "./fault";

afterEach(() => vi.restoreAllMocks());

describe("noting a fault of the page", () => {
  it("writes the name, a cut message and the stack frames for a TypeError", () => {
    const log = vi.spyOn(console, "error").mockImplementation(() => {});
    const e = new TypeError("Cannot read properties of undefined (reading 'filter')");
    e.stack = "TypeError: Cannot read properties of undefined (reading 'filter')\n    at foo (app.js:1:2)\n    at bar (app.js:3:4)";
    noteFault(e);
    expect(log).toHaveBeenCalledOnce();
    const line = String(log.mock.calls[0][0]);
    expect(line).toContain("TypeError");
    expect(line).toContain("(reading 'filter')");
    expect(line).toContain("at foo (app.js:1:2)");
  });

  it("says nothing for a refusal or anything that is not a defect of the page", () => {
    const log = vi.spyOn(console, "error").mockImplementation(() => {});
    noteFault(new Error("model refused"));
    noteFault("plain text");
    noteFault(undefined);
    expect(log).not.toHaveBeenCalled();
  });

  it("cuts a long message and keeps the frames bounded", () => {
    const log = vi.spyOn(console, "error").mockImplementation(() => {});
    const e = new RangeError("x".repeat(5000));
    e.stack = ["RangeError: x", ...Array.from({ length: 50 }, (_, i) => `    at f${i} (a.js:${i}:1)`)].join("\n");
    noteFault(e);
    const line = String(log.mock.calls[0][0]);
    expect(line.length).toBeLessThan(1500);
    expect(line.match(/\n/g)?.length).toBe(12);
  });
});

function fault(message: string, stack: string): TypeError {
  const e = new TypeError(message);
  e.stack = `TypeError: ${message}\n${stack}`;
  return e;
}

describe("what a page fault leaves for a report", () => {
  it("is the error's name, its message and the first frames' own locations", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const shown = "Cannot read properties of undefined (reading 'filter')";
    noteFault(fault(shown, [
      "    at waiting (https://app.example.test/assets/index-9f3a.js:12:345)",
      "    at Pane (file:///C:/Users/alice/AppData/Local/Programs/Reasonix/resources/app.asar/dist/assets/index-9f3a.js:1:2)",
      "    at https://app.example.test/assets/chunk-1.js?token=abc123:7:8",
    ].join("\n")), shown);

    const detail = faultDetail(shown);
    expect(detail).toContain("TypeError");
    expect(detail).toContain("waiting (index-9f3a.js:12:345)");
    expect(detail).toContain("Pane (index-9f3a.js:1:2)");
    expect(detail).toContain("chunk-1.js:7:8");
    for (const leak of ["https://", "file://", "alice", "token=abc123", "app.example.test"]) expect(detail).not.toContain(leak);
  });

  it("keeps at most six frames and cuts a long message", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const frames = Array.from({ length: 12 }, (_, i) => `    at f${i} (https://x.test/a.js:${i}:1)`).join("\n");
    noteFault(fault("m".repeat(500), frames), "shown");
    const lines = faultDetail("shown").split("\n");
    expect(lines.filter((l) => l.startsWith("  at "))).toHaveLength(6);
    expect(lines[0]!.length).toBeLessThan(260);
  });

  it("says nothing for words it did not record, or for an error that is not the page's own", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    noteFault(new Error("the kernel refused"), "the kernel refused");
    expect(faultDetail("the kernel refused")).toBe("");
    expect(faultDetail("never seen")).toBe("");
  });
});
