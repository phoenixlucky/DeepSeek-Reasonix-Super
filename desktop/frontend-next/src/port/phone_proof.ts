// The phone's half of the controller proof: the same bytes the accounts service
// verifies in auth/controllerProof.ts. A signature is WebCrypto's IEEE P1363
// form (r and s, 32 bytes each), which is what the service expects.
const encoder = new TextEncoder();
type Bytes = Uint8Array<ArrayBuffer>;

const PROOF_LABEL = "reasonix-controller-v1";
const ID_LABEL = "reasonix-rc-v1";

export interface DeviceJwk {
  kty: "EC";
  crv: "P-256";
  x: string;
  y: string;
}

export function base64url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function fromBase64url(text: string): Bytes {
  const padded = text.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(text.length / 4) * 4, "=");
  return Uint8Array.from(atob(padded), (char) => char.charCodeAt(0));
}

async function sha256(input: string): Promise<Bytes> {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(input)));
}

export async function thumbprintOf(jwk: DeviceJwk): Promise<string> {
  return base64url(await sha256(`{"crv":"${jwk.crv}","kty":"${jwk.kty}","x":"${jwk.x}","y":"${jwk.y}"}`));
}

export async function deviceIdFor(host: string, thumbprint: string): Promise<string> {
  return `rc_${base64url(await sha256(`${ID_LABEL}${host}${thumbprint}`))}`;
}

function lengthPrefixed(parts: string[]): Bytes {
  const chunks = [encoder.encode(PROOF_LABEL)];
  for (const part of parts) {
    const bytes = encoder.encode(part);
    chunks.push(new Uint8Array([bytes.length >> 8, bytes.length & 0xff]), bytes);
  }
  const out = new Uint8Array(chunks.reduce((total, chunk) => total + chunk.length, 0));
  let at = 0;
  for (const chunk of chunks) {
    out.set(chunk, at);
    at += chunk.length;
  }
  return out;
}

export function enrollMessage(input: { nonce: string; userId: number; host: string; thumbprint: string }): Bytes {
  return lengthPrefixed(["enroll", input.nonce, String(input.userId), input.host, input.thumbprint]);
}

export async function pushMessage(input: {
  nonce: string; userId: number; host: string; deviceId: string; endpoint: string; p256dh: string; auth: string;
}): Promise<Bytes> {
  const fields = [input.endpoint, input.p256dh, input.auth].map((part) => `${part.length}:${part}`).join("|");
  return lengthPrefixed(["push", input.nonce, String(input.userId), input.host, input.deviceId, base64url(await sha256(fields))]);
}

export interface DeviceKey {
  publicKey: DeviceJwk;
  privateKey: CryptoKey;
}

export async function generateDeviceKey(): Promise<DeviceKey> {
  const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign"]);
  const jwk = await crypto.subtle.exportKey("jwk", pair.publicKey);
  return { publicKey: { kty: "EC", crv: "P-256", x: String(jwk.x), y: String(jwk.y) }, privateKey: pair.privateKey };
}

export async function sign(key: CryptoKey, message: Bytes): Promise<string> {
  return base64url(new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key, message)));
}
