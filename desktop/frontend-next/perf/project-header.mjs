import assert from "node:assert/strict";
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const failures = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? " — " + detail : ""}`);
  if (!ok) failures.push(name);
};

const browser = await chromium.launch();
try {
  for (const scheme of ["light", "dark"]) {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: scheme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (error) => failures.push(error.message));
    await page.goto(`${BASE}?pref=zh&turns=3`, { waitUntil: "networkidle" });
    const add = page.locator(".projects-heading .projects-add");
    const more = page.locator(".projects-heading .wsmore");
    await add.waitFor();
    await page.addStyleTag({ content: "*{transition:none!important;animation:none!important}" });
    const paint = (control) => control.evaluate((button) => {
      const style = getComputedStyle(button);
      return { fill: style.backgroundColor, ink: style.color };
    });
    await page.mouse.move(900, 500);
    const rest = await paint(add);
    await add.hover();
    const addHover = await paint(add);
    await more.hover();
    const moreHover = await paint(more);
    check(`${scheme}: both header actions visibly highlight on hover`,
      addHover.fill !== rest.fill && moreHover.fill !== rest.fill);
    check(`${scheme}: project header actions have the same hover fill and ink`,
      addHover.fill === moreHover.fill && addHover.ink === moreHover.ink,
      JSON.stringify({ add: addHover, more: moreHover }));
    await more.click();
    const menu = page.getByRole("menu", { name: "项目列表的更多操作", exact: true });
    await menu.waitFor();
    await page.mouse.move(900, 500);
    const moreOpen = await paint(more);
    check(`${scheme}: the open project menu keeps the header action highlight`,
      moreOpen.fill === addHover.fill && moreOpen.ink === addHover.ink,
      JSON.stringify(moreOpen));
    await page.keyboard.press("Escape");
    check(`${scheme}: Escape closes the project menu and returns focus`,
      await more.getAttribute("aria-expanded") === "false" && await more.evaluate((button) => button === document.activeElement));
    await page.keyboard.press("Shift+Tab");
    const addFocus = await add.evaluate((button) => ({
      focused: button === document.activeElement,
      outline: getComputedStyle(button).outline,
    }));
    await page.keyboard.press("Tab");
    const moreFocus = await more.evaluate((button) => ({
      focused: button === document.activeElement,
      outline: getComputedStyle(button).outline,
    }));
    check(`${scheme}: both header actions retain visible keyboard focus`,
      addFocus.focused && moreFocus.focused
        && !addFocus.outline.includes("none") && !moreFocus.outline.includes("none"),
      JSON.stringify({ add: addFocus, more: moreFocus }));
    await more.click();
    await menu.waitFor();
    await page.locator(".projects-heading-title").click();
    check(`${scheme}: clicking outside dismisses the menu`, await more.getAttribute("aria-expanded") === "false");
    await add.click();
    const dialog = page.getByRole("dialog", { name: "打开项目", exact: true });
    await dialog.waitFor();
    check(`${scheme}: the add action still offers the local folder picker`,
      await dialog.getByRole("button", { name: "选择本机文件夹", exact: true }).isEnabled());
    await page.keyboard.press("Escape");
    await dialog.waitFor({ state: "hidden" });
    await page.mouse.move(900, 500);
    const after = await paint(more);
    check(`${scheme}: dismissing the menu restores the resting background`, after.fill === rest.fill);
    await page.close();
  }
} finally {
  await browser.close();
}
assert.deepEqual(failures, [], "project header interaction regressions");
