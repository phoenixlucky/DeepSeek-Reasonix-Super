// Keyboard focus has to be seen. Every button the keyboard reaches in the workbench must look different when focused for a
// reason other than fading in: an outline, a shadow, a border or a background. A control that only changes opacity is
// invisible to anyone who cannot tell .45 from 1.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width: 1280, height: 900 }, colorScheme: scheme });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector(".app");
  await page.waitForTimeout(700);
  await page.mouse.move(1270, 5);
  await page.evaluate(() => document.activeElement?.blur());
  const look = (el) => {
    const cs = getComputedStyle(el);
    return [cs.outlineStyle === "none" ? "" : `${cs.outlineWidth} ${cs.outlineColor}`, cs.boxShadow, cs.borderColor, cs.backgroundColor, cs.textDecorationLine].join("|");
  };
  const seen = new Set();
  let stops = 0;
  const bad = [];
  for (let i = 0; i < 90; i++) {
    await page.keyboard.press("Tab");
    const r = await page.evaluate(() => {
      const el = document.activeElement;
      if (!el || el === document.body || el.tagName !== "BUTTON") return null;
      const cs = getComputedStyle(el);
      const focused = [cs.outlineStyle === "none" ? "" : `${cs.outlineWidth} ${cs.outlineColor}`, cs.boxShadow, cs.borderColor, cs.backgroundColor, cs.textDecorationLine].join("|");
      const id = (el.getAttribute("aria-label") || el.textContent || el.getAttribute("data-action") || "button").trim().slice(0, 40);
      el.blur();
      const css2 = getComputedStyle(el);
      const plain = [css2.outlineStyle === "none" ? "" : `${css2.outlineWidth} ${css2.outlineColor}`, css2.boxShadow, css2.borderColor, css2.backgroundColor, css2.textDecorationLine].join("|");
      const r = el.getBoundingClientRect();
      el.focus({ focusVisible: true });
      return { id, key: id + Math.round(r.left) + Math.round(r.top), same: focused === plain, shown: r.width > 0 && r.height > 0 };
    });
    if (!r) continue;
    if (seen.has(r.key)) break;
    seen.add(r.key);
    stops++;
    if (r.shown && r.same) bad.push(r.id);
  }
  console.log(`${bad.length ? "FAIL" : "  ok"}  ${scheme}: ${stops} buttons reached by Tab${bad.length ? ", no visible focus on: " + bad.join("; ") : ""}`);
  if (bad.length) fails.push(`${scheme}: ${bad.join("; ")}`);
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
