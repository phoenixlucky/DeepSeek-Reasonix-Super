// A host notice is a bar, not a card: one sentence with a glyph, no header strip, no box and no state wash. Severity is
// the glyph's ink, so warn and error differ only there.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width: 1000, height: 760 }, colorScheme: scheme });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector(".app");
  await page.waitForTimeout(600);
  await page.evaluate(() => {
    window.__feed({ kind: "turn_started", authoredTurn: 1, msgIndex: 0, text: "go" });
    for (const level of ["info", "warn", "error"]) window.__feed({ kind: "notice", level, text: `a ${level} notice`, detail: "the diagnostic" });
    window.__feed({ kind: "turn_done" });
  });
  await page.waitForTimeout(500);
  await page.mouse.move(990, 10);
  const m = await page.evaluate(() => {
    const read = (el) => {
      const cs = getComputedStyle(el);
      const g = el.querySelector(".nb-ic, .nb-dot");
      return {
        border: [cs.borderTopWidth, cs.borderLeftWidth, cs.borderRightWidth, cs.borderBottomWidth].join(","),
        bg: cs.backgroundColor,
        header: !!el.querySelector(".hl, .tag"),
        glyph: g ? getComputedStyle(g).color : "",
      };
    };
    return [...document.querySelectorAll(".notice-bar")].map(read);
  });
  check(`${scheme}: three notices drawn as bars`, m.length === 3, `${m.length} found`);
  check(`${scheme}: no box, wash or header strip`, m.length > 0 && m.every((x) => x.border === "0px,0px,0px,0px" && (x.bg === "rgba(0, 0, 0, 0)" || x.bg === "transparent") && !x.header), JSON.stringify(m[0] ?? {}));
  check(`${scheme}: severity is the glyph ink alone`, m.length === 3 && new Set(m.map((x) => x.glyph)).size === 3, m.map((x) => x.glyph).join(" / "));
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
