import { currentUserId, enrollDevice, type Account, type Device, type KeyStore } from "./phone_device";
import { disablePush, enablePush, EnableRefused, NO_CATEGORIES, pushStatus, setCategories, type PushBrowser, type PushCategories, type PushStatus } from "./phone_push";
import { deviceIdFor, sign, thumbprintOf } from "./phone_proof";

// Where this phone stands, as the person would put it. "waiting" is the desktop
// not having approved it yet; "removed" is the desktop having taken it back.
export type Reminders =
  | { kind: "off" }
  | { kind: "waiting" }
  | { kind: "removed" }
  | { kind: "on"; status: PushStatus };

export interface RemindersDeps {
  account: Account;
  keys: KeyStore;
  browser: PushBrowser;
  uaClass: string;
}

export interface PhoneReminders {
  supported(): boolean;
  load(): Promise<Reminders>;
  enable(): Promise<Reminders>;
  choose(categories: PushCategories): Promise<Reminders>;
  disable(): Promise<Reminders>;
  startOver(): Promise<Reminders>;
}

// One computer's reminders on this phone. Nothing is created until enable():
// reading the state never makes a key and never adds a row to the desktop's list.
export function phoneReminders(host: string, deps: RemindersDeps): PhoneReminders {
  const { account, keys, browser } = deps;
  const known = async () => {
    const key = await keys.load(host);
    return key ? { key, id: await deviceIdFor(host, await thumbprintOf(key.publicKey)) } : null;
  };
  const load = async (): Promise<Reminders> => {
    const own = await known();
    const status = own ? await pushStatus(account, own.id) : null;
    return status ? { kind: "on", status } : { kind: "off" };
  };
  return {
    supported: () => browser.supported(),
    load,
    // The permission prompt needs the press that started this, so it is asked
    // before the first network round trip rather than after.
    async enable() {
      if (!browser.supported()) throw new EnableRefused("unsupported");
      if (browser.permission() !== "granted" && (await browser.requestPermission()) !== "granted") throw new EnableRefused("denied");
      const device: Device = await enrollDevice({ account, keys, uaClass: deps.uaClass }, host);
      if (device.state === "pending") return { kind: "waiting" };
      if (device.state === "revoked") return { kind: "removed" };
      const key = (await keys.load(host))!;
      const status = await enablePush(account, browser, (m) => sign(key.privateKey, m), host, device, await currentUserId(account), NO_CATEGORIES);
      return { kind: "on", status };
    },
    async choose(categories) {
      const own = await known();
      if (!own) return { kind: "off" };
      return { kind: "on", status: await setCategories(account, own.id, categories) };
    },
    async disable() {
      const own = await known();
      if (own) await disablePush(account, browser, own.id);
      return { kind: "off" };
    },
    async startOver() {
      await keys.remove(host);
      return { kind: "off" };
    },
  };
}
