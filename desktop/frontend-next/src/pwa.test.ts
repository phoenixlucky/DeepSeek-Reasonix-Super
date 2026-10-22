import { describe, expect, it, vi } from "vitest";

const fs = await vi.importActual<{ readFileSync: (path: string, encoding: string) => string; existsSync: (path: string) => boolean }>("node:fs");
const cp = await vi.importActual<{ spawnSync: (cmd: string, args: string[]) => { status: number | null } }>("node:child_process");
const read = (p: string) => fs.readFileSync(p, "utf8");
const manifest = JSON.parse(read("public/manifest.webmanifest"));
const ignored = (p: string) => cp.spawnSync("git", ["check-ignore", "-q", p]).status === 0;

describe("the phone page can be added to the home screen", () => {
  it("links the manifest and the touch icon from the page", () => {
    const html = read("index.html");
    expect(html).toMatch(/<link rel="manifest" href="[^"]*manifest\.webmanifest"/);
    expect(html).toMatch(/<link rel="apple-touch-icon" href="[^"]*apple-touch-icon\.png"/);
    expect(html).toMatch(/<meta name="theme-color" content="#07080c"/);
  });

  it("opens full screen at the page it was installed from", () => {
    expect(manifest.display).toBe("standalone");
    expect(manifest.start_url).toBe("./");
    expect(manifest.scope).toBe("./");
    expect(manifest.name).toBeTruthy();
  });

  it("ships the icon sizes an install needs", () => {
    const sizes = manifest.icons.map((i: { sizes: string }) => i.sizes);
    expect(sizes).toContain("192x192");
    expect(sizes).toContain("512x512");
    expect(manifest.icons.some((i: { purpose: string }) => i.purpose === "maskable")).toBe(true);
  });

  it("has every icon on disk and none of them ignored by git", () => {
    const files = [...manifest.icons.map((i: { src: string }) => `public/${i.src}`), "public/icons/apple-touch-icon.png", "public/icons/icon.svg"];
    for (const f of files) {
      expect(fs.existsSync(f), `${f} is missing`).toBe(true);
      expect(ignored(f), `${f} is ignored by git, so it would never reach a build`).toBe(false);
    }
  });

  it("registers no service worker when the page loads", () => {
    expect(read("index.html")).not.toMatch(/serviceWorker/);
  });
});

// The worker is run against a stand-in scope, so what it registers and what it
// shows are read from what it does, not from how its source is spelled.
function runWorker() {
  const listeners = new Map<string, (event: unknown) => void>();
  const shown: { title: string; options: Record<string, unknown> }[] = [];
  const touched: string[] = [];
  const scope = new Proxy({
    addEventListener: (name: string, fn: (event: unknown) => void) => listeners.set(name, fn),
    navigator: { language: "en-US" },
    registration: { scope: "https://phone.test/remote/", showNotification: async (title: string, options: Record<string, unknown>) => { shown.push({ title, options }); } },
    clients: { matchAll: async () => [], openWindow: async () => null },
  } as Record<string, unknown>, {
    get(target, key: string) {
      if (key in target) return target[key];
      touched.push(key);
      return undefined;
    },
  });
  new Function("self", read("public/push-sw.js"))(scope);
  return { listeners, shown, touched, scope };
}

describe("the push service worker", () => {
  it("handles only push and notificationclick, with no fetch handler and no cache", () => {
    const { listeners, touched } = runWorker();
    expect([...listeners.keys()].sort()).toEqual(["notificationclick", "push"]);
    expect(touched.filter((key) => ["caches", "fetch", "importScripts", "skipWaiting"].includes(key))).toEqual([]);
  });

  it("is served fresh, so a fixed worker reaches the phone", () => {
    expect(read("public/_headers")).toMatch(/\/push-sw\.js\s+Cache-Control: no-store/);
  });

  it("shows fixed wording for the type and count, never text from the payload", async () => {
    const { listeners, shown } = runWorker();
    const push = listeners.get("push")!;
    const waits: Promise<unknown>[] = [];
    const send = (data: unknown) => push({ data: { json: () => data }, waitUntil: (p: Promise<unknown>) => waits.push(p) });
    send({ v: 1, type: "approval", count: 3, title: "rm -rf /", body: "secret prompt" });
    send({ v: 1, type: "nonsense", count: 1 });
    send({ v: 2, type: "error" });
    await Promise.all(waits);
    expect(shown.map((n) => n.title)).toEqual(["An action needs your approval (3)", "Reasonix", "Reasonix"]);
    expect(JSON.stringify(shown)).not.toMatch(/rm -rf|secret/);
    expect(shown[0].options.tag).toBe("approval");
  });

  it("still shows something when the payload cannot be read, as the browser requires", async () => {
    const { listeners, shown } = runWorker();
    const waits: Promise<unknown>[] = [];
    listeners.get("push")!({ data: { json: () => { throw new Error("bad"); } }, waitUntil: (p: Promise<unknown>) => waits.push(p) });
    await Promise.all(waits);
    expect(shown).toHaveLength(1);
  });

  it("opens the page it belongs to when a notification is pressed", async () => {
    const { listeners, scope } = runWorker();
    const open = vi.fn(async () => null);
    (scope.clients as { openWindow: unknown }).openWindow = open;
    const waits: Promise<unknown>[] = [];
    listeners.get("notificationclick")!({ notification: { close: () => {} }, waitUntil: (p: Promise<unknown>) => waits.push(p) });
    await Promise.all(waits);
    expect(open).toHaveBeenCalledWith("https://phone.test/remote/");
  });
});
