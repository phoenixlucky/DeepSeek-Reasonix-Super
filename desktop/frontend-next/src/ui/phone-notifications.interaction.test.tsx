// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { PhoneNotifications } from "./PhoneNotifications";
import { HttpError } from "../port/http_error";
import type { AgentPort, NotifyPrefs } from "../port/port";
import type { HubPort } from "../port/hub";
import type { PhoneView } from "../port/share";

afterEach(cleanup);

const off: NotifyPrefs = {
  enabled: false, turnDone: true, approval: true, ask: true, feedbackReply: true, diskLow: true, sound: false,
  phone: { enabled: false, approval: false, input: false, error: false },
};

const active: PhoneView = { id: "rc_a", ordinal: 1, name: "iPhone Safari", state: "active", uaClass: "ios-safari", createdAt: "2026-10-12T08:00:00Z", lastSeenAt: "2026-10-18T09:30:00Z", keyThumbprint: "t1" };
const waiting: PhoneView = { id: "rc_b", name: "Android Chrome", state: "pending", uaClass: "android-chrome", createdAt: "2026-10-18T09:40:00Z", lastSeenAt: "2026-10-18T09:40:00Z", expiresAt: "2026-10-18T09:55:00Z", keyThumbprint: "Zq8m", replaceCandidate: "rc_a" };

function draw(over: { prefs?: NotifyPrefs | null; phones?: PhoneView[] | null; hub?: Partial<HubPort> } = {}) {
  const prefs = over.prefs === undefined ? off : over.prefs;
  const set = vi.fn(async (next: NotifyPrefs) => next);
  const port = { notifyPrefs: vi.fn(async () => prefs), setNotifyPrefs: set } as unknown as AgentPort;
  const phones = over.phones === undefined ? [active, waiting] : over.phones;
  const hub = {
    sharePhones: vi.fn(async () => (phones ? { phones } : null)),
    resolvePhone: vi.fn(async (id: string, action: string) => ({ phones: (phones ?? []).filter((p) => p.id !== id || action !== "reject").map((p) => (p.id === id ? { ...p, state: "active" as const } : p)) })),
    revokePhone: vi.fn(async (id: string) => ({ phones: (phones ?? []).filter((p) => p.id !== id) })),
    ...over.hub,
  } as unknown as HubPort;
  render(<PhoneNotifications hub={hub} port={port} onError={() => {}} />);
  return { set, hub };
}

describe("phone notifications on the desktop", () => {
  it("draws nothing where this kernel has no window to notify from", async () => {
    draw({ prefs: null });
    await waitFor(() => expect(document.querySelector("#set-phone-notify")).toBeNull());
  });

  it("starts with everything off and turns the master switch and each occasion on separately", async () => {
    const { set } = draw();
    const master = await screen.findByRole("switch", { name: "提醒我的手机" });
    expect(master.getAttribute("aria-checked")).toBe("false");
    for (const name of ["有操作等待批准时", "模型提问时", "一轮失败时"]) {
      const sw = screen.getByRole("switch", { name });
      expect(sw.getAttribute("aria-checked")).toBe("false");
      expect((sw as HTMLButtonElement).disabled).toBe(true);
    }
    await userEvent.click(master);
    await waitFor(() => expect(set).toHaveBeenCalledWith({ ...off, phone: { enabled: true, approval: false, input: false, error: false } }));
    await userEvent.click(await screen.findByRole("switch", { name: "一轮失败时" }));
    await waitFor(() => expect(set).toHaveBeenLastCalledWith({ ...off, phone: { enabled: true, approval: false, input: false, error: true } }));
  });

  it("says what is sent and what is not, so nobody has to guess", async () => {
    draw();
    expect(await screen.findByText(/只有“哪一类”和数量，不含会话名称、命令或任何对话内容/)).toBeTruthy();
  });

  it("lists the enrolled phones, a waiting one with the ways to answer it", async () => {
    const { hub } = draw();
    const list = await screen.findByRole("list", { name: "已登记的手机" });
    expect(within(list).getByText(/iPhone Safari/)).toBeTruthy();
    const row = within(list).getByText(/Android Chrome/).closest("li") as HTMLElement;
    expect(within(row).getByText(/等你批准 · 标识 Zq8m/)).toBeTruthy();
    await userEvent.click(within(row).getByRole("button", { name: "替换旧设备" }));
    expect(hub.resolvePhone).toHaveBeenCalledExactlyOnceWith("rc_b", "replace", "rc_a");
    cleanup();
    const again = draw();
    const next = (await screen.findByText(/Android Chrome/)).closest("li") as HTMLElement;
    await userEvent.click(within(next).getByRole("button", { name: "添加" }));
    expect(again.hub.resolvePhone).toHaveBeenCalledExactlyOnceWith("rc_b", "add");
  });

  it("rejects a waiting phone without a second click, and asks once before removing a phone that is in", async () => {
    const { hub } = draw();
    await userEvent.click(await screen.findByRole("button", { name: "拒绝" }));
    expect(hub.resolvePhone).toHaveBeenCalledExactlyOnceWith("rc_b", "reject");
    const row = screen.getByText(/iPhone Safari/).closest("li") as HTMLElement;
    await userEvent.click(within(row).getByRole("button", { name: "移除" }));
    expect(hub.revokePhone).not.toHaveBeenCalled();
    await userEvent.click(within(row).getByRole("button", { name: "确认移除" }));
    expect(hub.revokePhone).toHaveBeenCalledExactlyOnceWith("rc_a");
  });

  it("says why the list could not be read, by the kernel's code, and keeps the switches", async () => {
    draw({ hub: { sharePhones: vi.fn(async () => { throw new HttpError(409, "x", { code: "phones.not_registered", error: "no" }); }) } });
    expect((await screen.findByRole("alert")).textContent).toContain("还没有登记到账号");
    expect(screen.getByRole("switch", { name: "提醒我的手机" })).toBeTruthy();
  });

  it("offers no list where there is nothing to ask", async () => {
    draw({ phones: null });
    await screen.findByRole("switch", { name: "提醒我的手机" });
    expect(screen.queryByRole("list", { name: "已登记的手机" })).toBeNull();
  });

  it("says plainly when no phone has enrolled", async () => {
    draw({ phones: [] });
    expect(await screen.findByText(/还没有手机登记/)).toBeTruthy();
  });
});
