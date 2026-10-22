import { describe, expect, it } from "vitest";
import { AccountError, type Account, type KeyStore } from "./phone_device";
import { phoneReminders } from "./phone_reminders";
import type { PushBrowser } from "./phone_push";
import type { DeviceKey } from "./phone_proof";

const HOST = "ab".repeat(32);
const CATEGORIES = { approval: false, input: false, error: false };
const STATUS = { categories: CATEGORIES, lastFailureAt: null, lastFailureStatus: null, failureCount: 0 };

function memoryKeys(): KeyStore & { held: Map<string, DeviceKey> } {
  const held = new Map<string, DeviceKey>();
  return { held, load: async (h) => held.get(h) ?? null, save: async (h, k) => void held.set(h, k), remove: async (h) => void held.delete(h) };
}

function setup(state: "active" | "pending" | "revoked" = "active") {
  const calls: string[] = [];
  let subscribed = false;
  const account: Account = {
    async call<T>(method: string, path: string, body?: unknown): Promise<T> {
      calls.push(`${method} ${path}`);
      if (path === "/me") return { user: { id: 7 } } as T;
      if (path === "/me/remote-controllers/challenge") return { nonce: "ef".repeat(32) } as T;
      if (path === "/me/remote-controllers/enroll") {
        const id = (body as { controllerId: string }).controllerId;
        if (state === "pending") throw new AccountError(403, "controller_unconfirmed", { controller: { id, state, name: "" } });
        if (state === "revoked") throw new AccountError(403, "controller_revoked", { controller: { id, state, name: "" } });
        return { controller: { id, state, name: "iPhone Safari" } } as T;
      }
      if (path === "/me/remote-push/key") return { publicKey: "BAQ" } as T;
      if (method === "PUT") { subscribed = true; return { subscription: STATUS } as T; }
      if (method === "PATCH") return { subscription: { ...STATUS, categories: (body as { categories: typeof CATEGORIES }).categories } } as T;
      if (method === "DELETE") { subscribed = false; return undefined as T; }
      if (subscribed) return { subscription: STATUS } as T;
      throw new AccountError(404, "controller_not_found");
    },
  };
  const browser: PushBrowser = {
    supported: () => true, permission: () => "granted", requestPermission: async () => "granted",
    subscribe: async () => ({ endpoint: "https://e/1", p256dh: "k", auth: "a" }), unsubscribe: async () => {},
  };
  const keys = memoryKeys();
  return { calls, keys, reminders: phoneReminders(HOST, { account, keys, browser, uaClass: "ios-safari" }) };
}

describe("this phone's reminders", () => {
  it("reads as off without making a key or touching the account's device list", async () => {
    const s = setup();
    expect(await s.reminders.load()).toEqual({ kind: "off" });
    expect(s.keys.held.size).toBe(0);
    expect(s.calls).toEqual([]);
  });

  it("switching on enrolls, subscribes with every occasion still off, and then reads as on", async () => {
    const s = setup();
    const on = await s.reminders.enable();
    expect(on).toEqual({ kind: "on", status: STATUS });
    expect(s.keys.held.size).toBe(1);
    expect(await s.reminders.load()).toMatchObject({ kind: "on" });
  });

  it("asks for permission before any request leaves, and a refusal sends nothing", async () => {
    const s = setup();
    const order: string[] = [];
    const reminders = phoneReminders(HOST, {
      account: { call: async (m: string, p: string) => { order.push(`${m} ${p}`); throw new Error("must not be called"); } } as Account,
      keys: s.keys, uaClass: "other",
      browser: { supported: () => true, permission: () => "default", requestPermission: async () => { order.push("prompt"); return "denied"; }, subscribe: async () => ({ endpoint: "", p256dh: "", auth: "" }), unsubscribe: async () => {} },
    });
    await expect(reminders.enable()).rejects.toMatchObject({ reason: "denied" });
    expect(order).toEqual(["prompt"]);
    expect(s.keys.held.size).toBe(0);
  });

  it("a phone the desktop has not approved yet waits, and nothing is subscribed", async () => {
    const s = setup("pending");
    expect(await s.reminders.enable()).toEqual({ kind: "waiting" });
    expect(s.calls.some((c) => c.startsWith("PUT"))).toBe(false);
  });

  it("a phone the desktop removed says so, and can start over with a new key", async () => {
    const s = setup("revoked");
    expect(await s.reminders.enable()).toEqual({ kind: "removed" });
    const old = s.keys.held.get(HOST);
    expect(await s.reminders.startOver()).toEqual({ kind: "off" });
    expect(s.keys.held.has(HOST)).toBe(false);
    await s.reminders.enable();
    expect(s.keys.held.get(HOST)).not.toBe(old);
  });

  it("chooses occasions and switches off", async () => {
    const s = setup();
    await s.reminders.enable();
    const chosen = await s.reminders.choose({ approval: true, input: false, error: true });
    expect(chosen).toMatchObject({ kind: "on", status: { categories: { approval: true, error: true } } });
    expect(await s.reminders.disable()).toEqual({ kind: "off" });
    expect(await s.reminders.load()).toEqual({ kind: "off" });
  });

  it("choosing on a phone that never enrolled changes nothing", async () => {
    const s = setup();
    expect(await s.reminders.choose({ approval: true, input: true, error: true })).toEqual({ kind: "off" });
    expect(s.calls).toEqual([]);
  });
});
