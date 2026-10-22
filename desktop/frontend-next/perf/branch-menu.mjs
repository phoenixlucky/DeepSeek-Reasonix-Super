// Workspace selection must stay reachable before and after a task, with no
// legacy hover card stacked over the open palette.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&turns=2";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE });
try {
  for (const turns of [0, 2]) {
    const page = await browser.newPage({ locale: "zh-CN", viewport: { width: 1280, height: 800 }, reducedMotion: "reduce" });
    const url = new URL(BASE);
    url.searchParams.set("turns", String(turns));
    await page.goto(url.href, { waitUntil: "networkidle" });
    const chip = page.getByRole("button", { name: "配置工作目录", exact: true });
    await chip.hover();
    await chip.click();
    const palette = page.getByRole("dialog", { name: "配置工作目录", exact: true });
    await palette.waitFor();
    check(`${turns} turns: the workspace palette opened`, await chip.getAttribute("aria-expanded") === "true");
    const stacked = await page.locator(".studio-branch-card").evaluateAll((cards) => cards.some((card) => getComputedStyle(card).visibility === "visible" && Number(getComputedStyle(card).opacity) > 0.5));
    check(`${turns} turns: no legacy hover card overlaps the palette`, !stacked);
    await palette.getByRole("option", { name: "main", exact: true }).waitFor();
    check(`${turns} turns: the current branch is selected`, await palette.getByRole("option", { name: "main", exact: true }).getAttribute("aria-selected") === "true");
    const search = palette.getByRole("searchbox", { name: "搜索分支", exact: true });
    await search.fill("studio");
    check(`${turns} turns: search filters the branch choices`, await palette.getByRole("option").count() === 1 && await palette.getByRole("option", { name: "studio", exact: true }).isVisible());
    await search.press("Escape");
    await palette.waitFor({ state: "detached" });
    check(`${turns} turns: closing returns focus to the workspace entry`, await chip.evaluate((element) => element === document.activeElement));
    await page.close();
  }
} finally {
  await browser.close();
}
if (fails.length) {
  console.error(`\n${fails.length} failed: ${fails.join("; ")}`);
  process.exit(1);
}
console.log("\nall passed");
