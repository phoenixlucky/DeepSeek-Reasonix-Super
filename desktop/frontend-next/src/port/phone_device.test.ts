import { describe, expect, it } from "vitest";
import { AccountError, accountClient, enrollDevice, uaClassOf, type KeyStore } from "./phone_device";
import { deviceIdFor, enrollMessage, fromBase64url, thumbprintOf, type DeviceJwk, type DeviceKey } from "./phone_proof";

const HOST = "ab".repeat(32);

function memoryKeys(): KeyStore & { held: Map<string, DeviceKey> } {
  const held = new Map<string, DeviceKey>();
  return { held, load: async (h) => held.get(h) ?? null, save: async (h, k) => void held.set(h, k), remove: async (h) => void held.delete(h) };
}

// An account that checks the proof the way the real service does: the nonce is
// single use and the signature must verify over the enrollment message.
function fakeAccount(options: { state?: "active" | "pending" | "revoked" } = {}) {
  const calls: string[] = [];
  const nonces = new Set<string>();
  let enrolled: { body: Record<string, unknown>; verified: boolean } | null = null;
  const send = (async (url: string, init?: RequestInit) => {
    const path = url.replace("https://id.test", "");
    calls.push(`${init?.method} ${path}`);
    const body = init?.body ? JSON.parse(String(init.body)) : {};
    const reply = (status: number, data: unknown) => new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });
    if (path === "/me") return reply(200, { user: { id: 7 } });
    if (path === "/me/remote-controllers/challenge") {
      const nonce = String(nonces.size).padStart(64, "0");
      nonces.add(nonce);
      return reply(200, { nonce, expiresAt: "later" });
    }
    if (path === "/me/remote-controllers/enroll") {
      if (!nonces.delete(body.nonce)) return reply(403, { error: { code: "challenge_invalid" } });
      const thumbprint = await thumbprintOf(body.publicKey as DeviceJwk);
      const key = await crypto.subtle.importKey("jwk", body.publicKey, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
      const verified = await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, fromBase64url(body.signature), enrollMessage({ nonce: body.nonce, userId: 7, host: String(body.host), thumbprint }));
      enrolled = { body, verified };
      if (!verified) return reply(403, { error: { code: "invalid_proof" } });
      const controller = { id: body.controllerId, state: options.state ?? "active", name: "iPhone Safari" };
      if (controller.state === "pending") return reply(403, { error: { code: "controller_unconfirmed" }, controller });
      if (controller.state === "revoked") return reply(403, { error: { code: "controller_revoked" }, controller });
      return reply(200, { controller });
    }
    return reply(404, { error: { code: "not_found" } });
  }) as unknown as typeof fetch;
  return { calls, send, enrolled: () => enrolled };
}

describe("enrolling this phone with the account", () => {
  it("makes a key once, proves it, and is recorded under the id the key names", async () => {
    const keys = memoryKeys(), account = fakeAccount();
    const id = { account: accountClient("https://id.test", account.send), keys, uaClass: "ios-safari" };
    const device = await enrollDevice(id, HOST);
    const key = keys.held.get(HOST)!;
    expect(device.state).toBe("active");
    expect(device.id).toBe(await deviceIdFor(HOST, await thumbprintOf(key.publicKey)));
    expect(account.enrolled()?.verified).toBe(true);
    expect(account.enrolled()?.body.uaClass).toBe("ios-safari");

    await enrollDevice(id, HOST);
    expect(keys.held.get(HOST)).toBe(key);
  });

  it("keeps a separate key for each computer", async () => {
    const keys = memoryKeys();
    const id = { account: accountClient("https://id.test", fakeAccount().send), keys, uaClass: "other" };
    await enrollDevice(id, HOST);
    const other = "cd".repeat(32);
    await enrollDevice(id, other);
    expect(keys.held.size).toBe(2);
    expect(keys.held.get(HOST)).not.toBe(keys.held.get(other));
  });

  it("reports a phone waiting for the desktop's approval as pending, not as a failure", async () => {
    const id = { account: accountClient("https://id.test", fakeAccount({ state: "pending" }).send), keys: memoryKeys(), uaClass: "other" };
    const device = await enrollDevice(id, HOST);
    expect(device.state).toBe("pending");
    expect(device.id).toMatch(/^rc_/);
  });

  it("reports a revoked phone as revoked", async () => {
    const id = { account: accountClient("https://id.test", fakeAccount({ state: "revoked" }).send), keys: memoryKeys(), uaClass: "other" };
    expect((await enrollDevice(id, HOST)).state).toBe("revoked");
  });

  it("lets any other refusal through with its code", async () => {
    const send = (async () => new Response(JSON.stringify({ error: { code: "unauthorized" } }), { status: 401 })) as unknown as typeof fetch;
    const id = { account: accountClient("https://id.test", send), keys: memoryKeys(), uaClass: "other" };
    await expect(enrollDevice(id, HOST)).rejects.toMatchObject({ status: 401, code: "unauthorized" });
    await expect(enrollDevice(id, HOST)).rejects.toBeInstanceOf(AccountError);
  });
});

describe("naming this browser for the desktop's device list", () => {
  it.each([
    ["Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1", "ios-safari"],
    ["Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 CriOS/120.0 Mobile/15E148 Safari/604.1", "ios-webview"],
    ["Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36", "android-chrome"],
    ["Mozilla/5.0 (Linux; Android 14; Pixel 8; wv) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36", "android-webview"],
    ["Mozilla/5.0 (Macintosh) Gecko/20100101 Firefox/120.0", "desktop-firefox"],
    ["something else", "other"],
  ])("%s", (agent, expected) => {
    expect(uaClassOf(agent)).toBe(expected);
  });
});
