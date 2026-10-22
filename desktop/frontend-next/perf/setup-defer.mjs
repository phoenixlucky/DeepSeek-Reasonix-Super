// First-run setup can be put off: the workbench opens with a way back to
// connecting that sits above the input box without covering it, in both schemes,
// and the way back lands on setup again.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
import { join } from "node:path";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?onboarding=1";
const SHOTS = process.env.SETUP_DEFER_SHOTS;
if (SHOTS) mkdirSync(SHOTS, { recursive: true });
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const ctx = await browser.newContext({ locale: "zh-CN", viewport: { width: 1280, height: 800 }, colorScheme: scheme });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => fails.push("page error: " + e.message));
  await page.goto(PAGE, { waitUntil: "domcontentloaded" });
  await page.waitForSelector(".onb-card");
  if (SHOTS) await page.screenshot({ path: join(SHOTS, `${scheme}-setup.png`) });
  await page.click('[data-action="onboarding.skip"]');
  await page.waitForSelector('[data-action="onboarding.reopen"]');
  const box = await page.evaluate(() => {
    const note = document.querySelector('.posturenote[data-kind="connect"]').getBoundingClientRect();
    const compose = document.querySelector('textarea[aria-label="任务输入"]').getBoundingClientRect();
    return { noteBottom: note.bottom, composeTop: compose.top, left: note.left, right: note.right, w: innerWidth, h: note.height };
  });
  check(`${scheme}: note sits above the input box`, box.noteBottom <= box.composeTop + 1, JSON.stringify(box));
  check(`${scheme}: note stays inside the window`, box.left >= 0 && box.right <= box.w);
  check(`${scheme}: no setup stage behind the workbench`, (await page.locator(".onb-stage").count()) === 0);
  if (SHOTS) await page.screenshot({ path: join(SHOTS, `${scheme}-deferred.png`) });
  await page.click('[data-action="onboarding.reopen"]');
  await page.waitForSelector(".onb-card");
  check(`${scheme}: the note leads back to setup`, true);
  await ctx.close();
}
await browser.close();
if (fails.length) {
  console.error(`\n${fails.length} failed`);
  process.exit(1);
}
