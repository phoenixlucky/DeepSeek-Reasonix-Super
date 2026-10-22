import { deviceIdFor, enrollMessage, generateDeviceKey, sign, thumbprintOf, type DeviceKey } from "./phone_proof";

// The key never leaves the browser that made it and is not extractable, so a
// copied cookie or a script reading storage cannot sign for this phone. It is
// kept per computer: a controller id names one key on one host.
export interface KeyStore {
  load(host: string): Promise<DeviceKey | null>;
  save(host: string, key: DeviceKey): Promise<void>;
  remove(host: string): Promise<void>;
}

const DB = "reasonix-phone-device";
const STORE = "keys";

function request<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function open(): Promise<IDBDatabase> {
  const opening = indexedDB.open(DB, 1);
  opening.onupgradeneeded = () => opening.result.createObjectStore(STORE);
  return request(opening);
}

export const indexedDbKeyStore: KeyStore = {
  async load(host) {
    const db = await open();
    try {
      const found = await request(db.transaction(STORE).objectStore(STORE).get(host));
      return found && typeof found === "object" && "privateKey" in found ? (found as DeviceKey) : null;
    } finally {
      db.close();
    }
  },
  async save(host, key) {
    const db = await open();
    try {
      await request(db.transaction(STORE, "readwrite").objectStore(STORE).put(key, host));
    } finally {
      db.close();
    }
  },
  async remove(host) {
    const db = await open();
    try {
      await request(db.transaction(STORE, "readwrite").objectStore(STORE).delete(host));
    } finally {
      db.close();
    }
  },
};

// An account-service failure keeps its code, so a caller tells "waiting for
// approval" from "revoked" from "try again later" without reading a sentence.
export class AccountError extends Error {
  constructor(readonly status: number, readonly code: string, readonly detail: unknown = null) {
    super(`${status} ${code}`);
  }
}

export interface Account {
  call<T>(method: string, path: string, body?: unknown): Promise<T>;
}

export function accountClient(base: string, send: typeof fetch): Account {
  return {
    async call<T>(method: string, path: string, body?: unknown): Promise<T> {
      const response = await send(`${base.replace(/\/$/, "")}${path}`, {
        method,
        credentials: "include",
        headers: body === undefined ? undefined : { "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (response.status === 204) return undefined as T;
      const parsed = await response.json().catch(() => null) as { error?: { code?: string } } | null;
      if (!response.ok) throw new AccountError(response.status, parsed?.error?.code ?? "unknown", parsed);
      return parsed as T;
    },
  };
}

export async function currentUserId(account: Account): Promise<number> {
  return (await account.call<{ user: { id: number } }>("GET", "/me")).user.id;
}

export type DeviceState = "pending" | "active" | "revoked";

export interface Device {
  id: string;
  state: DeviceState;
  name: string;
}

interface DeviceView extends Device {
  ordinal: number | null;
}

export interface Identity {
  account: Account;
  keys: KeyStore;
  uaClass: string;
}

export function uaClassOf(agent: string, mobileHint = false): string {
  if (/iPhone|iPad|iPod/.test(agent)) return /Safari/.test(agent) && !/FxiOS|CriOS|EdgiOS/.test(agent) ? "ios-safari" : "ios-webview";
  if (/Android/.test(agent)) return /; wv\)/.test(agent) ? "android-webview" : "android-chrome";
  if (mobileHint) return "other";
  if (/Edg\//.test(agent)) return "desktop-edge";
  if (/Firefox\//.test(agent)) return "desktop-firefox";
  if (/Chrome\//.test(agent)) return "desktop-chrome";
  if (/Safari\//.test(agent)) return "desktop-safari";
  return "other";
}

// Makes this browser's key on first use and asks the account to record it for
// the computer. A second phone on the same computer is held for the desktop to
// approve: the account answers 403 controller_unconfirmed and the phone shows
// that, instead of treating the refusal as a failure.
export async function enrollDevice(id: Identity, host: string): Promise<Device> {
  let key = await id.keys.load(host);
  if (!key) {
    key = await generateDeviceKey();
    await id.keys.save(host, key);
  }
  const thumbprint = await thumbprintOf(key.publicKey);
  const deviceId = await deviceIdFor(host, thumbprint);
  const userId = await currentUserId(id.account);
  const { nonce } = await id.account.call<{ nonce: string }>("POST", "/me/remote-controllers/challenge", { host, purpose: "enroll" });
  const signature = await sign(key.privateKey, enrollMessage({ nonce, userId, host, thumbprint }));
  try {
    const { controller } = await id.account.call<{ controller: DeviceView }>("POST", "/me/remote-controllers/enroll", {
      host, nonce, publicKey: key.publicKey, signature, uaClass: id.uaClass, controllerId: deviceId,
    });
    return { id: controller.id, state: controller.state, name: controller.name };
  } catch (error) {
    if (error instanceof AccountError && (error.code === "controller_unconfirmed" || error.code === "controller_revoked")) {
      const { controller } = (error.detail as { controller?: DeviceView } | null) ?? {};
      return { id: controller?.id ?? deviceId, state: error.code === "controller_revoked" ? "revoked" : "pending", name: controller?.name ?? "" };
    }
    throw error;
  }
}
