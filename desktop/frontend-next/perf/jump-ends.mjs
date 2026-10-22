// The two floating buttons are named for the ends they go to ("back to top", "jump to latest"),
// so one click has to arrive there, whatever expanded blocks lie between. Where a scroller ends up
// is a property of the render, so it is read from the browser.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&ws=2&sess=6&turns=40";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
try {
  const page = await browser.newPage({ viewport: { width: 1100, height: 760 } });
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.waitForSelector('[data-pane="flow"]');
  const geometry = () => page.locator('[data-pane="flow"]').evaluate((el) => ({ top: Math.round(el.scrollTop), room: Math.round(el.scrollHeight - el.clientHeight - el.scrollTop) }));
  await page.locator('[data-pane="flow"]').evaluate((el) => { el.scrollTop = Math.round((el.scrollHeight - el.clientHeight) / 2); });
  await page.waitForSelector('.scrolljump button[data-value="up"]');
  await page.waitForSelector('.scrolljump button[data-value="down"]');
  await page.evaluate(() => document.querySelectorAll("details.tool-disclosure, details.think").forEach((d) => { d.open = true; }));
  await page.waitForTimeout(200);
  const open = await page.locator("details.tool-disclosure[open], details.think[open]").count();
  check("there are expanded blocks between the middle and each end", open >= 2, `${open} expanded blocks`);
  check("the conversation is long enough to scroll", (await geometry()).top > 0 && (await geometry()).room > 0);

  await page.click('.scrolljump button[data-value="up"]');
  await page.waitForTimeout(400);
  const atTop = await geometry();
  check("one click on the top button reaches the top", atTop.top <= 1, `scrollTop ${atTop.top}`);

  await page.locator('[data-pane="flow"]').evaluate((el) => { el.scrollTop = Math.round((el.scrollHeight - el.clientHeight) / 2); });
  await page.waitForSelector('.scrolljump button[data-value="down"]');
  await page.click('.scrolljump button[data-value="down"]');
  await page.waitForTimeout(800);
  const atEnd = await geometry();
  check("one click on the latest button reaches the end", atEnd.room <= 2, `${atEnd.room}px from the end`);
  await page.close();
} finally {
  await browser.close();
}
if (fails.length) {
  console.error(`\n${fails.length} failed: ${fails.join("; ")}`);
  process.exit(1);
}
