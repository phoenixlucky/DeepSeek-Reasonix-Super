// A command that never stops printing does not grow the page: only the end of its output is kept
// while it runs, and the card says how much came before.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const KEPT_MAX = 70_000;
const CHUNKS = Number(process.env.PERF_CHUNKS ?? 150);
const HEAP_MAX_MB = Number(process.env.PERF_HEAP_MAX ?? 60);
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const shotDir = process.env.SHOTS;

const browser = await chromium.launch({ args: ["--js-flags=--expose-gc"] });
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", colorScheme: scheme, viewport: { width: 1400, height: 900 }, reducedMotion: "reduce" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  const cdp = await ctx.newCDPSession(page);
  await cdp.send("Performance.enable");
  await page.goto(`${BASE}?turns=2&pref=zh`, { waitUntil: "networkidle" });
  await page.waitForSelector(".compose");
  await page.evaluate(() => {
    window.__feed({ kind: "turn_started" });
    window.__feed({ kind: "tool_dispatch", tool: { id: "long", name: "bash", args: JSON.stringify({ command: "tail -f build.log" }) } });
  });
  for (let i = 0; i < CHUNKS; i++) {
    await page.evaluate((i) => {
      const line = `2026-10-19 12:00:00 INFO worker-${i} processing a batch with a fairly long log line ` + "x".repeat(60) + "\n";
      window.__feed({ kind: "tool_progress", tool: { id: "long", name: "bash", args: "", output: line.repeat(1500) } });
    }, i);
  }
  await page.waitForTimeout(500);
  await cdp.send("HeapProfiler.collectGarbage");
  const heap = (await cdp.send("Performance.getMetrics")).metrics.find((m) => m.name === "JSHeapUsedSize").value / 1048576;
  const shown = await page.evaluate(() => document.querySelector('[data-call="long"] .out')?.textContent?.length ?? -1);
  const note = await page.evaluate(() => document.body.textContent.includes("此前省略了"));
  check(`${scheme}: the running card holds only the end of ${((CHUNKS * 1500 * 141) / 1048576).toFixed(0)} MB of output`, shown >= 0 && shown <= KEPT_MAX, `${shown} characters`);
  check(`${scheme}: it says how much came before`, note);
  check(`${scheme}: the page's heap stays bounded while it runs`, heap < HEAP_MAX_MB, `${heap.toFixed(1)} MB`);
  if (shotDir) {
    await page.evaluate(() => {
      const card = document.querySelector('[data-call="long"] details.tool-disclosure');
      if (card) { card.open = true; card.dispatchEvent(new Event("toggle")); }
      document.querySelector('[data-call="long"]')?.scrollIntoView({ block: "start" });
    });
    await page.waitForTimeout(300);
    await page.screenshot({ path: `${shotDir}/${scheme}-progress-tail.png`, clip: { x: 360, y: 80, width: 1000, height: 560 } });
  }
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed:\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\nall passed");
