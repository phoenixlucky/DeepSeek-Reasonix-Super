// The composer keeps the 1.x feel: a ring of light sweeps its border once when
// it is hovered or focused, nothing sweeps while a turn runs or when motion is
// reduced, and the run state is announced rather than printed inside the box.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&turns=2";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const shotDir = process.env.SHOTS;
const sweep = (page) => page.evaluate(() => getComputedStyle(document.querySelector(".compose"), "::before").animationName);

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const running of [false, true]) {
    const label = `${scheme} ${running ? "running" : "idle"}`;
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, locale: "zh-CN", colorScheme: scheme });
    page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
    await page.goto(BASE + (running ? "&running=1" : ""), { waitUntil: "networkidle" });
    await page.waitForSelector(".compose");
    if (running) {
      await page.waitForFunction(() => typeof window.__feed === "function");
      await page.waitForTimeout(400);
      await page.evaluate(() => window.__feed({ kind: "turn_started" }));
    }
    await page.waitForSelector(running ? ".studio-runstate[data-running]" : ".studio-runstate[data-idle]");
    await page.mouse.move(5, 5);
    await page.waitForTimeout(150);

    const state = await page.evaluate(() => {
      const status = document.querySelector(".compose .studio-runstate");
      const word = status?.querySelector(".studio-runlabel > span.sr-only");
      const r = word?.getBoundingClientRect();
      const clock = status?.querySelector(".studio-runclock");
      return {
        role: status?.getAttribute("role"),
        live: status?.getAttribute("aria-live"),
        said: word?.textContent ?? "",
        wordBox: r ? [Math.round(r.width), Math.round(r.height)] : null,
        clockShown: !!clock && clock.getBoundingClientRect().width > 0,
        mark: !!status?.querySelector(".rmark"),
      };
    });
    check(`${label}: the state is announced`, state.role === "status" && state.live === "polite" && state.said.length > 0, state.said);
    check(`${label}: the word is not printed in the box`, state.wordBox && state.wordBox[0] <= 2 && state.wordBox[1] <= 2, JSON.stringify(state.wordBox));
    check(`${label}: the mark stays`, state.mark);
    if (running) check(`${label}: the clock stays`, state.clockShown);

    check(`${label}: nothing sweeps at rest`, (await sweep(page)) === "none");
    await page.hover(".compose textarea");
    await page.waitForTimeout(300);
    const hovered = await sweep(page);
    check(`${label}: ${running ? "no sweep on hover while running" : "a sweep on hover"}`, running ? hovered === "none" : hovered === "compose-sweep", hovered);
    if (shotDir && !running) await page.locator(".compose").screenshot({ path: `${shotDir}/${scheme}-hover.png` });
    await page.mouse.move(5, 5);
    await page.waitForTimeout(100);
    await page.focus(".compose textarea");
    await page.waitForTimeout(300);
    const focused = await sweep(page);
    check(`${label}: ${running ? "no sweep on focus while running" : "a sweep on focus"}`, running ? focused === "none" : focused === "compose-sweep", focused);
    if (shotDir) await page.locator(".compose").screenshot({ path: `${shotDir}/${scheme}-${running ? "running" : "idle"}.png` });
    await page.close();
  }
  const calm = await browser.newPage({ viewport: { width: 1280, height: 800 }, locale: "zh-CN", colorScheme: scheme, reducedMotion: "reduce" });
  await calm.goto(BASE, { waitUntil: "networkidle" });
  await calm.waitForSelector(".compose");
  await calm.hover(".compose textarea");
  await calm.focus(".compose textarea");
  await calm.waitForTimeout(300);
  check(`${scheme}: no sweep when motion is reduced`, (await sweep(calm)) === "none");
  await calm.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
