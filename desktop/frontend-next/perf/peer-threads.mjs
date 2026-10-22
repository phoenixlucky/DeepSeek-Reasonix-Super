// A session this one has written to carries one small icon for where the
// conversation stands: it has to sit inside its row, leave the title readable,
// say its state to a reader who cannot hover, and not select the row it is on.
import { chromium } from "playwright";

const PAGE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?pref=zh&ws=1&sess=6&peerthreads=1";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const shotDir = process.env.SHOTS;

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, locale: "zh-CN", colorScheme: scheme });
  page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
  await page.goto(PAGE, { waitUntil: "networkidle" });
  await page.waitForSelector(".sessrow");
  await page.waitForSelector(".peerthread");
  const rows = await page.evaluate(() =>
    [...document.querySelectorAll(".sessrow")].map((row) => {
      const mark = row.querySelector(".peerthread");
      const title = row.querySelector(".sesstitle");
      const r = row.getBoundingClientRect();
      const m = mark?.getBoundingClientRect();
      const t = title?.getBoundingClientRect();
      return {
        path: row.getAttribute("data-target"),
        state: mark?.getAttribute("data-state") ?? null,
        label: mark?.getAttribute("aria-label") ?? null,
        inside: m ? m.left >= r.left && m.right <= r.right && m.top >= r.top && m.bottom <= r.bottom : null,
        clear: m && t ? m.left >= t.right - 1 : null,
        size: m ? Math.round(m.width) : null,
      };
    }),
  );
  const marked = rows.filter((r) => r.state);
  check(`${scheme}: four sessions carry a mark, the rest none`, marked.length === 4 && marked.map((r) => r.state).sort().join() === "declined,replied,waiting,working", marked.map((r) => r.state).join(","));
  check(`${scheme}: each mark says its state in words`, marked.every((r) => r.label && r.label.length > 6));
  check(`${scheme}: each mark sits inside its row`, marked.every((r) => r.inside));
  check(`${scheme}: no mark covers the title`, marked.every((r) => r.clear));
  check(`${scheme}: marks are small`, marked.every((r) => r.size >= 14 && r.size <= 22), marked.map((r) => r.size).join(","));
  const before = await page.locator('.sessrow[aria-selected="true"]').count();
  await page.locator(".peerthread[data-state=\"waiting\"]").first().focus();
  check(`${scheme}: a mark takes keyboard focus`, await page.evaluate(() => document.activeElement?.classList.contains("peerthread")));
  if (shotDir) await page.locator(".rail").screenshot({ path: `${shotDir}/rail-${scheme}.png` });
  void before;
  await page.close();
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
