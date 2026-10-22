// A settings row is a label and a control. When the sheet is narrow the label must keep a readable column and the
// control must drop below it, never squeeze the text to a few pixels beside it.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const FLOOR = 96;

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [390, 768, 1024]) {
    const page = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: scheme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.goto(`${BASE}?pref=en&turns=2`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    await page.evaluate(() => document.fonts.ready);
    await page.evaluate(() => document.querySelector('[data-action="chrome.settings"]')?.click());
    await page.waitForSelector(".prefs-nav button");
    const tabs = await page.evaluate(() => document.querySelectorAll(".prefs-nav button").length);
    const squeezed = [];
    for (let i = 0; i < tabs; i++) {
      const id = await page.evaluate((n) => {
        const b = document.querySelectorAll(".prefs-nav button")[n];
        b.click();
        return b.id || b.textContent.trim().slice(0, 12);
      }, i);
      await page.waitForTimeout(250);
      const bad = await page.evaluate((floor) =>
        [...document.querySelectorAll(".lrow > .tx, .fold-row > .tx, .storage .item > .l, .srv .load > .hn")]
          .filter((el) => el.getBoundingClientRect().width > 0 && el.getBoundingClientRect().width < floor)
          .map((el) => `${el.parentElement.className.split(" ")[0]} ${Math.round(el.getBoundingClientRect().width)}px "${(el.textContent || "").trim().slice(0, 18)}"`),
      FLOOR);
      for (const b of bad) squeezed.push(`${id}: ${b}`);
    }
    const cut = await page.evaluate(() =>
      [...document.querySelectorAll(".cfgbad .term")].map((t) => t.scrollWidth - t.clientWidth).filter((d) => d > 1),
    );
    check(`${scheme} ${width}px: the config error block shows its lines whole`, cut.length === 0, cut.map((d) => `${d}px hidden`).join(", "));
    check(`${scheme} ${width}px: no settings label is squeezed under ${FLOOR}px`, squeezed.length === 0, squeezed.slice(0, 4).join("; "));
    await page.close();
  }
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
