import { describe, expect, it } from "vitest";
import { base64url, deviceIdFor, enrollMessage, fromBase64url, generateDeviceKey, pushMessage, sign, thumbprintOf } from "./phone_proof";

// Fixed values from the accounts service (auth/controllerProof.vectors.ts). The
// service verifies these bytes, so the phone must reproduce them exactly.
const v = {
  jwk: { kty: "EC", crv: "P-256", x: "FelvUaklSvIURZzOclLu5BBoKwDzjYHqf1O4quuqWCA", y: "bhFRUDuvHP0ARL9G26uoySCYF_YUSmmVfgJPkqJkaFQ" } as const,
  thumbprint: "6z6kTwo2bbB7TA7xu8RgrZusCXrW5vWsXINpYlQLYOQ",
  deviceId: "rc_jYC8UQU0VMlnqQuN8s_bqulPqAchjxNYSm2fRkQSiGc",
  host: "ab".repeat(32),
  nonce: "cd".repeat(32),
  userId: 7,
  enrollHex: "726561736f6e69782d636f6e74726f6c6c65722d76310006656e726f6c6c004063646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364000137004061626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162002b367a366b54776f32626242375441377875385267725a7573435872573576577358494e70596c514c594f51",
  signature: "CGP4TsOSCxKrRY6WAN7NnA1-a05DNGduZokdcn69E5D5DTlIxVzZm6dFJNlaDW-3Y-wy2VYCZoCoMNH7NZjB2w",
  pushHex: "726561736f6e69782d636f6e74726f6c6c65722d7631000470757368004063646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364636463646364000137004061626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162616261626162002e72635f6a59433855515530564d6c6e7151754e38735f6271756c50714163686a784e59536d3266526b5153694763002b5f7539764c6f694e5a754d7043364e7a5f2d795f334e49465039765f36506e37564c5278346d7447464a41",
};
const hex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");

describe("the phone's proof bytes match the accounts service", () => {
  it("names the key and the device the way the service does", async () => {
    expect(await thumbprintOf(v.jwk)).toBe(v.thumbprint);
    expect(await deviceIdFor(v.host, v.thumbprint)).toBe(v.deviceId);
  });

  it("encodes the enrollment message byte for byte, and the service's signature verifies over it", async () => {
    const message = enrollMessage({ nonce: v.nonce, userId: v.userId, host: v.host, thumbprint: v.thumbprint });
    expect(hex(message)).toBe(v.enrollHex);
    const key = await crypto.subtle.importKey("jwk", v.jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
    expect(await crypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, key, fromBase64url(v.signature), message)).toBe(true);
  });

  it("encodes the push message byte for byte", async () => {
    const message = await pushMessage({
      nonce: v.nonce, userId: v.userId, host: v.host, deviceId: v.deviceId,
      endpoint: "https://fcm.googleapis.com/fcm/send/abc", p256dh: "BPp-key", auth: "auth-secret",
    });
    expect(hex(message)).toBe(v.pushHex);
  });

  it("binds the endpoint and both keys: changing any one changes the message", async () => {
    const base = { nonce: v.nonce, userId: v.userId, host: v.host, deviceId: v.deviceId, endpoint: "https://e/1", p256dh: "k", auth: "a" };
    const same = hex(await pushMessage(base));
    for (const change of [{ endpoint: "https://e/2" }, { p256dh: "k2" }, { auth: "a2" }]) {
      expect(hex(await pushMessage({ ...base, ...change }))).not.toBe(same);
    }
  });
});

describe("a generated device key", () => {
  it("signs in the 64-byte form the service accepts, and the private half cannot be exported", async () => {
    const key = await generateDeviceKey();
    expect(fromBase64url(await sign(key.privateKey, new Uint8Array([1, 2, 3]))).length).toBe(64);
    await expect(crypto.subtle.exportKey("jwk", key.privateKey)).rejects.toThrow();
    expect(base64url(fromBase64url(key.publicKey.x))).toBe(key.publicKey.x);
  });
});
