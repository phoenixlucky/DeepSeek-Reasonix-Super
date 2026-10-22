// A server that answered 401 offers to sign in beside the retry; a server whose
// tools were withheld offers to accept and follow beside the plain approval; a
// followed server says so and offers to stop. None of it may spill at a phone's
// width or crowd the row's other controls.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};
const shotDir = process.env.SHOTS;

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [1280, 420]) {
    const tag = `${scheme} ${width}px`;
    const page = await browser.newPage({ viewport: { width, height: 1100 }, locale: "zh-CN", colorScheme: scheme, reducedMotion: "reduce" });
    page.on("pageerror", (e) => fails.push("页面异常: " + e.message));
    await page.goto(`${BASE}?pref=zh&turns=2&mcpactions=1`, { waitUntil: "networkidle" });
    await page.waitForSelector(".app");
    await page.evaluate(() => document.querySelector('[data-action="chrome.settings"]')?.click());
    await page.waitForSelector(".prefs-nav button");
    await page.evaluate(() => [...document.querySelectorAll(".prefs-nav button")].find((b) => b.textContent.includes("扩展"))?.click());
    await page.waitForSelector(".mcp-server");
    await page.evaluate(() => document.querySelectorAll("details.mcp-server").forEach((d) => { d.open = true; }));
    const r = await page.evaluate(() => {
      const row = (name) => [...document.querySelectorAll(".mcp-server")].find((d) => (d.querySelector("summary") ?? d).textContent.includes(name));
      const label = (el) => [...el.querySelectorAll("button")].map((b) => b.textContent.trim());
      const spill = (el) => { const r = el.getBoundingClientRect(); return [...el.querySelectorAll("button")].filter((b) => b.getBoundingClientRect().right > r.right + 1 || b.getBoundingClientRect().left < r.left - 1).length; };
      const figma = row("figma"), notes = row("notes"), wiki = row("wiki");
      const overlap = (el) => { const bs = [...el.querySelectorAll("button")].map((b) => b.getBoundingClientRect()).filter((b) => b.width > 0); return bs.some((a, i) => bs.some((c, j) => i < j && a.left < c.right - 1 && c.left < a.right - 1 && a.top < c.bottom - 1 && c.top < a.bottom - 1)); };
      return {
        figma: figma ? label(figma) : null, notes: notes ? label(notes) : null, wiki: wiki ? label(wiki) : null,
        followNote: wiki?.textContent.includes("这个服务器的改动会自动接受") ?? false,
        spill: [figma, notes, wiki].filter(Boolean).map(spill),
        overlap: [figma, notes, wiki].filter(Boolean).map(overlap),
      };
    });
    check(`${tag}: a 401 row offers sign-in beside the retry`, r.figma?.includes("登录") && r.figma.includes("重连"), r.figma?.join("|"));
    check(`${tag}: a row with withheld tools offers both approvals`, r.notes?.includes("批准并重新连接") && r.notes.includes("接受并以后都自动接受这个服务器的改动"), r.notes?.join("|"));
    check(`${tag}: a followed row says so and offers to stop`, r.followNote && r.wiki?.includes("取消跟随"), r.wiki?.join("|"));
    check(`${tag}: no button spills out of its row`, r.spill.every((n) => n === 0), r.spill.join(","));
    check(`${tag}: no two buttons overlap`, r.overlap.every((o) => !o));
    if (shotDir && width === 1280) {
      await page.addStyleTag({ content: ".cfgbad { display: none !important; }" });
      for (const name of ["figma", "notes", "wiki"]) {
        const row = page.locator(".mcp-server", { hasText: name }).first();
        await row.screenshot({ path: `${shotDir}/${scheme}-${name}.png` });
      }
    }
    await page.close();
  }
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
