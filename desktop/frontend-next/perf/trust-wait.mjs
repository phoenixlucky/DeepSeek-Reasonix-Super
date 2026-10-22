// A conversation out of sight that is asking whether to trust its folder says so in the
// sidebar; the one in front shows the card itself and carries no such mark.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const shotDir = process.env.SHOTS;

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", colorScheme: scheme, viewport: { width: 1400, height: 800 }, reducedMotion: "reduce" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(`${BASE}?pref=zh&panes=2&turns=1&trustpending=1`, { waitUntil: "networkidle" });
  await page.waitForSelector(".compose");
  await page.waitForTimeout(800);
  const rows = await page.evaluate(() => [...document.querySelectorAll(".sessrow[data-run]")].map((r) => r.getAttribute("data-run")));
  const waiting = rows.filter((r) => r === "halt").length;
  check(`${scheme}: exactly one conversation, the one out of sight, is marked as waiting`, waiting === 1, JSON.stringify(rows));
  const cards = await page.evaluate(() => document.querySelectorAll('.posturenote[data-kind="trust"]').length);
  check(`${scheme}: each conversation still carries its own trust card`, cards === 2, String(cards));
  if (shotDir) await page.screenshot({ path: `${shotDir}/${scheme}-trust-wait.png`, clip: { x: 0, y: 0, width: 700, height: 460 } });
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed:\n  ` + fails.join("\n  "));
  process.exit(1);
}
console.log("\nall passed");
