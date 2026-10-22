// A picture sent with a message is a 240x160 thumbnail whatever its shape. A
// tall phone screenshot used to keep its own aspect ratio at the thumbnail's
// width and spill far below its box, over the message text.
import { chromium } from "playwright";

const BASE = process.env.PERF_URL ?? "http://localhost:4399/perf.html?ws=1&sess=1&turns=1&pref=zh";
const fails = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "  ok" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
  if (!ok) fails.push(name);
};

const shape = { tall: [390, 700], wide: [1600, 400], square: [500, 500], small: [60, 40] };
const svg = (w, h) => `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}"><rect width="${w}" height="${h}" fill="#e8eef5"/></svg>`;
const text = "@.reasonix/attachments/clipboard-tall.png\n@.reasonix/attachments/clipboard-wide.png\n管理员长按成员头像弹出的踢出弹窗，成员名字下方修改为岗位\n移除成员界面成员也要显示岗位";

const browser = await chromium.launch();
for (const scheme of ["light", "dark"]) {
  for (const width of [1400, 520]) {
    const page = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: scheme });
    page.on("pageerror", (e) => fails.push("page error: " + e.message));
    await page.route(/fixture/, (r) => {
      const name = Object.keys(shape).find((k) => r.request().url().includes(k)) ?? "square";
      return r.fulfill({ contentType: "image/svg+xml", body: svg(...shape[name]) });
    });
    await page.goto(BASE, { waitUntil: "networkidle" });
    await page.waitForFunction(() => window.__panes?.() > 0);
    await page.evaluate((text) => window.__feed({
      kind: "turn_started", authoredTurn: 9, msgIndex: 90, text,
      attachments: ["tall", "wide", "square", "small"].map((n) => ({ path: `.reasonix/attachments/${n}.png` })),
    }), text);
    await page.locator(".call[data-k='me'] .user-image img").last().waitFor();
    await page.waitForTimeout(400);
    const rows = await page.evaluate(() => {
      const me = [...document.querySelectorAll(".call[data-k='me']")].at(-1);
      const rect = (e) => { const b = e.getBoundingClientRect(); return { l: b.left, t: b.top, r: b.right, b: b.bottom, w: b.width, h: b.height }; };
      const words = [...me.querySelectorAll(".out, .txt, .bubble, p, div")].filter((e) => !e.closest(".user-images") && e.textContent.includes("管理员"));
      const bubble = words.at(-1);
      return { imgs: [...me.querySelectorAll(".user-image img")].map((i) => ({ box: rect(i.closest(".user-image")), img: rect(i) })), text: bubble ? rect(bubble) : null };
    });
    const tag = `${scheme} ${width}px`;
    check(`${tag}: every picture stays inside a 240x160 thumbnail`, rows.imgs.length === 4 && rows.imgs.every(({ img }) => img.w <= 241 && img.h <= 161), JSON.stringify(rows.imgs.map((x) => [Math.round(x.img.w), Math.round(x.img.h)])));
    check(`${tag}: no picture leaves its own box`, rows.imgs.every(({ box, img }) => img.t >= box.t - 1 && img.b <= box.b + 1 && img.l >= box.l - 1 && img.r <= box.r + 1));
    check(`${tag}: the message text is not covered by a picture`, rows.text !== null && rows.imgs.every(({ img }) => img.b <= rows.text.t + 1 || img.t >= rows.text.b - 1 || img.r <= rows.text.l + 1 || img.l >= rows.text.r - 1));
    await page.close();
  }
}
await browser.close();
if (fails.length) {
  console.log(`\n${fails.length} failed`);
  process.exit(1);
}
console.log("\nall ok");
