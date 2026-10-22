// The rail's search box stays open while it holds a query: pressing a result takes focus off the box, and a
// box that folds away on blur moves the list up under the pointer, so the first press never became a click.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const shotDir = process.env.SHOTS;
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, locale: "zh-CN", colorScheme: scheme, reducedMotion: "reduce" });
  page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
  await page.goto(`${BASE}?pref=zh&turns=2&search=1`, { waitUntil: "networkidle" });
  await page.waitForSelector(".app");
  const state = () => page.evaluate(() => {
    const box = document.querySelector(".wsfind");
    const cs = getComputedStyle(box);
    return { shown: cs.opacity === "1" && cs.position === "relative", top: Math.round(box.getBoundingClientRect().top) };
  });

  await page.getByRole("searchbox").fill("轮换");
  await page.waitForSelector(".content-matches li");
  const hit = page.locator(".content-matches li button[data-action='session.open-match']").first();
  const before = await hit.boundingBox();
  if (shotDir) await page.screenshot({ path: `${shotDir}/${scheme}-typed.png`, clip: { x: 0, y: 0, width: 420, height: 520 } });

  await page.evaluate(() => {
    window.__events = [];
    for (const ev of ["mousedown", "mouseup", "click"]) {
      window.addEventListener(ev, (e) => window.__events.push(`${ev} ${e.target.tagName}[${e.target.closest?.("[data-action]")?.getAttribute("data-action") ?? ""}]`), true);
    }
  });
  await page.mouse.move(before.x + 20, before.y + before.height - 8);
  await page.mouse.down();
  await page.waitForTimeout(200);
  const held = await state();
  const moved = (await hit.boundingBox()).y - before.y;
  if (shotDir) await page.screenshot({ path: `${shotDir}/${scheme}-pressed.png`, clip: { x: 0, y: 0, width: 420, height: 520 } });
  await page.mouse.up();
  await page.waitForTimeout(200);
  const events = await page.evaluate(() => window.__events);

  check(`${scheme}: the search box stays open after focus leaves it, because it holds a query`, held.shown);
  check(`${scheme}: pressing a result does not move the list under the pointer`, Math.abs(moved) < 1, `${moved}px`);
  check(`${scheme}: the first press on a result becomes a click on that result`, events.some((e) => e.startsWith("click ") && e.endsWith("[session.open-match]")), events.join(" | "));

  await page.getByRole("searchbox").fill("");
  await page.keyboard.press("Tab");
  await page.mouse.click(700, 500);
  await page.waitForTimeout(250);
  const idle = await state();
  check(`${scheme}: with no query the box still folds away when it loses focus`, !idle.shown);
  await page.close();
}
await browser.close();
if (fails.length) {
  console.error(`\n${fails.length} failed: ${fails.join("; ")}`);
  process.exit(1);
}
console.log("\nall passed");
