// An opened step shows its detail as a panel in place, bordered on all four sides by one hairline. A rule on one side only
// is a rail, and the transcript has no rails.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width: 1000, height: 900 }, colorScheme: scheme });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector(".app");
  await page.waitForTimeout(600);
  await page.evaluate(() => {
    const f = window.__feed;
    f({ kind: "turn_started", authoredTurn: 1, msgIndex: 0, text: "go" });
    f({ kind: "tool_dispatch", tool: { id: "t1", name: "bash", args: '{"command":"go test ./..."}' } });
    f({ kind: "tool_result", tool: { id: "t1", name: "bash", args: '{"command":"go test ./..."}', output: "ok\n", durationMs: 900 } });
    f({ kind: "text", text: "done" });
    f({ kind: "turn_done" });
  });
  await page.waitForTimeout(500);
  await page.evaluate(() => document.querySelectorAll("details").forEach((d) => (d.open = true)));
  await page.waitForTimeout(400);
  await page.mouse.move(995, 5);
  const m = await page.evaluate(() =>
    [...document.querySelectorAll(".flow .tool-disclosure > .out")].map((el) => {
      const cs = getComputedStyle(el);
      return { sides: [cs.borderTopWidth, cs.borderRightWidth, cs.borderBottomWidth, cs.borderLeftWidth].join(","), color: [cs.borderTopColor, cs.borderRightColor, cs.borderBottomColor, cs.borderLeftColor] };
    }));
  check(`${scheme}: an opened step has a detail panel`, m.length > 0, `${m.length} found`);
  check(`${scheme}: it is bordered on all four sides`, m.length > 0 && m.every((x) => x.sides === "1px,1px,1px,1px" && new Set(x.color).size === 1), JSON.stringify(m[0] ?? {}));
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
