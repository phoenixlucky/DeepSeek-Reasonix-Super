// Interface wording the catalogue test cannot see: a Chinese label kept in a table and drawn without t() passes every
// source scan and still shows up in the English window. This opens each settings section in English, fires the probes
// that fill in more of the page, and fails on any Chinese the reader would see that is not on the fixture list below.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
// Names the demo data itself carries, and names that are written in Chinese on purpose.
const FIXTURE = [
  /并行会话演示/, /上一次的会话/, /站点改版/, /数据管线/, /跑一遍评测/, /^中文$/, /^做游戏的小鱼$/, /^DeepSeek-Reasonix官方群$/,
  /缓存命中率为什么会掉/, /Co-Authored-By/, /Generated with/, /^回复用中文$/, /^跟着用户每条消息的语言走$/, /v2 是从零重写的 Go 内核/,
  /没有 web，桌面端重做/, /问题里提到了 main-v2/, /构建用 -tags legacy/, /^迁移前的构建方式$/,
];
const fails = [];
const browser = await chromium.launch();
const ctx = await browser.newContext({ locale: "en-US", viewport: { width: 1280, height: 900 } });
const page = await ctx.newPage();
page.setDefaultTimeout(8000);
page.on("pageerror", (e) => fails.push("page error: " + e.message));
await page.goto(`${BASE}?pref=en`, { waitUntil: "networkidle" });
await page.waitForSelector(".app");
await page.waitForTimeout(800);

const scan = () => {
  const out = [];
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    const text = n.textContent.trim();
    if (text && /[一-鿿]/.test(text) && !["SCRIPT", "STYLE"].includes(n.parentElement?.tagName)) out.push(text);
  }
  for (const el of document.querySelectorAll("[aria-label],[title],[placeholder],[alt]"))
    for (const a of ["aria-label", "title", "placeholder", "alt"]) {
      const v = el.getAttribute(a);
      if (v && /[一-鿿]/.test(v)) out.push(v);
    }
  return out;
};
const seen = new Map();
const take = async (where) => {
  for (const text of await page.evaluate(scan)) if (!FIXTURE.some((re) => re.test(text)) && !seen.has(text)) seen.set(text, where);
};

await take("workbench");
await page.evaluate(() => document.querySelector('[data-action="chrome.settings"]')?.click());
await page.waitForTimeout(500);
await page.evaluate(() => [...document.querySelectorAll("button")].find((x) => /Back up|repair/i.test(x.textContent))?.click());
await page.waitForTimeout(400);
const sections = await page.evaluate(() => [...new Set([...document.querySelectorAll('[data-action="settings.section"]')].map((b) => b.getAttribute("data-value")))]);
if (sections.length < 8) fails.push(`only ${sections.length} settings sections found; the guard is looking at the wrong page`);
for (const value of sections) {
  await page.evaluate((v) => document.querySelector(`[data-action="settings.section"][data-value="${v}"]`)?.click(), value);
  await page.waitForTimeout(450);
  await page.evaluate(() => document.querySelector('[data-action="network.diagnose"]')?.click());
  await page.evaluate(() => document.querySelectorAll("details").forEach((d) => (d.open = true)));
  await page.waitForTimeout(500);
  await take(`settings:${value}`);
}
await browser.close();

for (const [text, where] of seen) fails.push(`${where}: ${text.slice(0, 80)}`);
if (fails.length) {
  console.log("Chinese in the English window:\n  " + fails.join("\n  "));
  process.exit(1);
}
console.log(`ok  ${sections.length} settings sections, no unlisted Chinese in the English window`);
