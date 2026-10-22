// The corner notices sit above the composer of the pane in front and may not
// cover its controls (the model picker, send) at any width: covering them was
// the complaint that asked for the notices. Geometry needs a real browser.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&turns=4";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const browser = await chromium.launch();
try {
  for (const theme of ["light", "dark"]) {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, colorScheme: theme, reducedMotion: "reduce" });
    page.setDefaultTimeout(8000);
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.goto(PAGE, { waitUntil: "networkidle" });
    await page.waitForSelector(".compose");
    await page.evaluate(() => document.fonts.ready);
    for (const { width, height } of [{ width: 1280, height: 800 }, { width: 900, height: 700 }, { width: 640, height: 900 }, { width: 390, height: 800 }]) {
      await page.setViewportSize({ width, height });
      await page.fill('textarea[role="combobox"]', "继续检查");
      await page.evaluate(() => {
        for (const text of ["无法切换到模型 deepseek-v4-pro：该连接没有可用的密钥", "分支切换失败：工作区有未提交的改动", "无法打开链接：系统没有默认浏览器"]) window.__toast(text);
      });
      await page.waitForSelector(".toast");
      await page.waitForTimeout(250);
      const g = await page.evaluate(() => {
        const rect = (el) => el?.getBoundingClientRect() ?? null;
        const compose = rect(document.querySelector(".compose"));
        const stack = rect(document.querySelector(".toasts"));
        const controls = [...document.querySelectorAll('.compose [data-action="session.send"], .compose [data-action="model.select"], .compose .model-picker')].map((el) => ({ name: el.getAttribute("data-action") || el.className, r: rect(el) }));
        const hit = (el) => {
          const b = rect(el);
          const at = document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2);
          return !!at && (at === el || el.contains(at));
        };
        return { compose, stack, controls: controls.map((c) => ({ name: c.name, r: c.r, hit: hit([...document.querySelectorAll('.compose [data-action="session.send"], .compose [data-action="model.select"], .compose .model-picker')][controls.indexOf(c)]) })) };
      });
      const where = `${theme} ${width}×${height}`;
      check(`${where}: the notices end above the composer`, g.stack.bottom <= g.compose.top + 1, `stack.bottom=${Math.round(g.stack.bottom)} compose.top=${Math.round(g.compose.top)}`);
      check(`${where}: the notices stay inside the window`, g.stack.left >= 0 && g.stack.right <= width && g.stack.top >= 0);
      check(`${where}: the composer's controls are found`, g.controls.length >= 2, g.controls.map((c) => c.name).join(","));
      for (const c of g.controls) check(`${where}: ${c.name} takes the pointer`, c.hit);
      await page.evaluate(() => document.querySelectorAll(".toast button[data-action='toast.dismiss']").forEach((b) => b.click()));
      await page.waitForSelector(".toast", { state: "detached" });
    }
    await page.close();
  }
} finally {
  await browser.close();
}
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall passed");
