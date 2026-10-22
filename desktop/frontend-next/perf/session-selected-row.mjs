// The current session's row must read as current at a glance: its fill is clearly stronger than a hovered row's
// and a plain row's, a neutral ring (not a state colour) outlines it, and hovering it does not weaken either.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

const root = new URL("../src/", import.meta.url);
const names = [...(await readFile(new URL("main.tsx", root), "utf8")).matchAll(/import "\.\/styles\/([\w.-]+\.css)"/g)].map((m) => m[1]);
const sheets = await Promise.all(names.map((n) => readFile(new URL(`styles/${n}`, root), "utf8")));
const MIN_SELECTED_OVER_REST = 1.2;
const MIN_SELECTED_OVER_HOVER = 1.12;
const rows = [["第一个会话", "done"], ["第二个会话", "done", "sel"], ["第三个会话", "done"], ["有未读的会话", "", "", "unread"], ["正在运行的会话", "running"]];
const fixture = `<div class="rail" style="width:260px;padding:8px"><div class="kids">${rows.map(([t, run, sel, unread]) =>
  `<div class="sessrow sessrow-context" tabindex="0" ${run ? `data-run="${run}"` : ""} ${sel ? 'aria-selected="true"' : ""} ${unread ? "data-unread" : ""}><span class="sesstitle"><span>${t}</span></span><time class="sessmeta sessage">1 小时</time></div>`).join("")}</div></div>`;
const luminance = ([r, g, b]) => [r, g, b].map((c) => { c /= 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; }).reduce((s, v, i) => s + v * [0.2126, 0.7152, 0.0722][i], 0);
const contrast = (a, b) => { const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x); return (hi + 0.05) / (lo + 0.05); };

const browser = await chromium.launch();
try {
  for (const theme of ["light", "dark"]) {
    const page = await browser.newPage({ viewport: { width: 400, height: 400 } });
    await page.setContent(fixture);
    await page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), theme);
    for (const content of sheets) await page.addStyleTag({ content });
    await page.addStyleTag({ content: "*{transition:none!important;animation:none!important}" });
    const sample = async (i, hover) => {
      const row = page.locator(".sessrow").nth(i);
      await (hover ? row.hover() : page.mouse.move(390, 390));
      const box = await row.boundingBox();
      const png = await page.screenshot({ clip: { x: box.x + box.width - 60, y: box.y + 2, width: 4, height: 4 } });
      return page.evaluate(async (b64) => {
        const img = new Image();
        img.src = `data:image/png;base64,${b64}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = c.height = 4;
        const ctx = c.getContext("2d");
        ctx.drawImage(img, 0, 0);
        return [...ctx.getImageData(1, 1, 1, 1).data].slice(0, 3);
      }, png.toString("base64"));
    };
    const rest = await sample(2, false), selected = await sample(1, false), hovered = await sample(2, true), selectedHovered = await sample(1, true);
    assert.ok(contrast(selected, rest) >= MIN_SELECTED_OVER_REST, `${theme}: selected over a plain row is ${contrast(selected, rest).toFixed(3)}`);
    assert.ok(contrast(selected, hovered) >= MIN_SELECTED_OVER_HOVER, `${theme}: selected over a hovered row is ${contrast(selected, hovered).toFixed(3)}`);
    assert.deepEqual(selectedHovered, selected, `${theme}: hovering the selected row changes its fill`);
    const ring = await page.evaluate(() => {
      const [on, off] = [document.querySelectorAll(".sessrow")[1], document.querySelectorAll(".sessrow")[2]];
      return [getComputedStyle(on).outlineStyle, getComputedStyle(off).outlineStyle];
    });
    assert.notEqual(ring[0], "none", `${theme}: the selected row has no ring`);
    assert.equal(ring[1], "none", `${theme}: a plain row has a ring`);
    await page.close();
  }
} finally {
  await browser.close();
}
console.log("the selected session row is distinct from hover and plain rows, in light and dark");
