// A docked browser column starts at the top of the pane, and gives the rows back when it
// is undocked. Where things start is a layout question, so it is asked of a real browser.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const SHOTS = process.env.DOCK_SHOTS;
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const geometry = (page) =>
  page.evaluate(() => {
    const box = (sel) => {
      const el = document.querySelector(sel);
      const r = el?.getBoundingClientRect();
      return r && r.width > 0 ? { left: r.left, right: r.right, top: r.top, bottom: r.bottom } : null;
    };
    const card = document.querySelector('.pane > .scroll[data-pane="browser"] > .workbench');
    const divider = card ? parseFloat(getComputedStyle(card).borderLeftWidth) : 0;
    return { pane: box(".pane"), side: box('.pane > .scroll[data-pane="browser"]'), body: box(".pane > .pbody"), strip: box(".workbench-tabs"), divider };
  });

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const [label, width] of [["wide", 1440], ["narrow", 560]]) {
    const page = await browser.newPage({ viewport: { width, height: 800 }, colorScheme: scheme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    const tag = `${scheme}/${label}`;
    let docked = null;
    for (const round of ["first open", "after a reload"]) {
      await page.goto(`${BASE}?pref=en&turns=3`, { waitUntil: "networkidle" });
      await page.waitForSelector(".app");
      await page.locator('[data-action="pane.view"][data-value="browser"]').first().click();
      await page.waitForTimeout(700);
      const open = await geometry(page);
      docked = open;
      check(`${tag} ${round}: the browser column is drawn`, !!open.side);
      if (open.side) {
        if (label === "wide") {
          check(`${tag} ${round}: it starts at the top of the pane`, Math.abs(open.side.top - open.pane.top) <= 1, `pane ${open.pane.top} column ${open.side.top}`);
          check(`${tag} ${round}: it runs to the bottom of the pane`, Math.abs(open.side.bottom - open.pane.bottom) <= 1, `${open.side.bottom} against ${open.pane.bottom}`);
          check(`${tag} ${round}: the workbench card's own edge separates it from the conversation`, open.divider >= 1, String(open.divider));
        } else {
          check(`${tag} ${round}: the page takes the pane's full width when it stacks`, open.side.left <= open.pane.left + 1 && open.side.right >= open.pane.right - 1, JSON.stringify([open.pane, open.side]));
        }
      }
      if (SHOTS && round === "first open") await page.screenshot({ path: `${SHOTS}/${process.env.DOCK_TAG ?? "shot"}-${tag.replace("/", "-")}.png` });
    }
    await page.locator('[data-action="pane.view"][data-value="browser"][aria-pressed="true"]').first().click();
    await page.waitForTimeout(700);
    const shut = await geometry(page);
    check(`${tag} undocked: the column is gone`, !shut.side || shut.side.left >= shut.pane.right - 1, JSON.stringify([shut.side, shut.pane]));
    await page.close();
  }
}
// Every room between phone and wide, not two samples: the conversation shrinks with the window, and it must
// stay clear of the browser column, which stacks over it once the window is narrow.
{
  const page = await browser.newPage({ viewport: { width: 1440, height: 800 } });
  const bad = [];
  for (let width = 560; width <= 1440; width += 20) {
    await page.setViewportSize({ width, height: 800 });
    await page.goto(`${BASE}?pref=en&turns=3`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    await page.locator('[data-action="pane.view"][data-value="browser"]').first().click();
    await page.waitForTimeout(400);
    const g = await page.evaluate(() => {
      const side = document.querySelector('.pane > .scroll[data-pane="browser"]')?.getBoundingClientRect();
      const body = document.querySelector(".pane > .pbody")?.getBoundingClientRect();
      const pane = document.querySelector(".pane").getBoundingClientRect();
      const stacked = !!side && side.left <= pane.left + 40;
      return { stacked, clear: stacked || !side || !body || body.right <= side.left + 1 };
    });
    if (!g.clear || (width <= 640 && !g.stacked) || (width >= 1000 && g.stacked)) bad.push(`${width}:${g.clear ? "" : "overlap "}stacked=${g.stacked}`);
  }
  check("from 560 to 1440 the conversation stays clear of the browser column, which stacks only in a narrow window", bad.length === 0, bad.join(" "));
  await page.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
