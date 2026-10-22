// What the page holds of finished tool output is bounded: a long session of large results stays light,
// and a card says what it left out.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const RESULTS = Number(process.env.PERF_RESULTS ?? 600);
const HEAP_MAX_MB = Number(process.env.PERF_HEAP_MAX ?? 150);
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
  await page.evaluate((n) => {
    window.__feed({ kind: "turn_started" });
    const body = (i) => Array.from({ length: 300 }, (_, k) => `result ${i} line ${k}: matched in internal/pkg${k % 40}/file${k}.go — some context text`).join("\n");
    for (let i = 0; i < n; i++) {
      window.__feed({ kind: "tool_dispatch", tool: { id: `r${i}`, name: "bash", args: JSON.stringify({ command: "x" }) } });
      window.__feed({ kind: "tool_result", tool: { id: `r${i}`, name: "bash", args: JSON.stringify({ command: "x" }), output: body(i), durationMs: 200 } });
    }
  }, RESULTS);
  await page.waitForTimeout(800);
  await cdp.send("HeapProfiler.collectGarbage");
  await cdp.send("HeapProfiler.collectGarbage");
  const heap = (await cdp.send("Performance.getMetrics")).metrics.find((m) => m.name === "JSHeapUsedSize").value / 1048576;
  check(`${scheme}: ${RESULTS} results of about 30 KB leave the page's heap under ${HEAP_MAX_MB} MB`, heap < HEAP_MAX_MB, `${heap.toFixed(1)} MB`);
  await page.evaluate(() => {
    const grp = [...document.querySelectorAll("details")].filter((d) => d.querySelector('[data-call="r599"]')).pop();
    if (grp) grp.open = true;
    const card = document.querySelector('[data-call="r599"] details.tool-disclosure');
    if (card) { card.open = true; card.dispatchEvent(new Event("toggle")); }
    document.querySelector('[data-call="r599"]')?.scrollIntoView({ block: "start" });
  });
  await page.waitForTimeout(400);
  const said = await page.evaluate(() => document.body.textContent.includes("没有放在页面里"));
  check(`${scheme}: a card says what it left out`, said);
  if (shotDir) await page.screenshot({ path: `${shotDir}/${scheme}-output-cap.png`, clip: { x: 360, y: 40, width: 1000, height: 520 } });
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed:\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\nall passed");
