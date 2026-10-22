// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import type { ProviderEntry } from "../port/port";
import { Providers, type Port } from "./Providers";

beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => void values.set(key, value),
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const relay: ProviderEntry = {
  name: "relay", kind: "openai", baseUrl: "https://relay.example/v1", models: ["chat"], default: "chat",
  visionModels: [], hasKey: true, inUse: false, preset: false,
};

// The kernel remembers a plain re-check and hands it back with the list; the
// row's dot is where a failed test stays visible once the result card is gone.
it("leaves a failed test on the row's dot, named by its typed cause, after the list is read again", async () => {
  let remembered: ProviderEntry["lastCheck"];
  const providers = vi.fn(async () => [{ ...relay, ...(remembered ? { lastCheck: remembered } : {}) }]);
  const checkProvider = vi.fn(async () => {
    remembered = { at: 1, ok: false, code: "provider.probe.unauthorized" };
    return { ok: false, code: "provider.probe.unauthorized", httpStatus: 401 };
  });
  const port = { providers, protocols: vi.fn(async () => []), checkProvider } as unknown as Port;
  render(<Providers port={port} onChanged={() => {}} onFailed={() => {}} protocol={{}} onProtocol={() => {}} activeKindFor={(a) => a.kinds[0]} />);

  const dot = async () => (await screen.findAllByText("relay.example")).at(0)!.closest("button")!.querySelector(".pstate")!;
  expect((await dot()).getAttribute("data-state")).toBeNull();
  const reads = providers.mock.calls.length;

  await userEvent.click(within(screen.getByRole("region")).getByRole("button", { name: "测试连接" }));
  await waitFor(() => expect(providers.mock.calls.length).toBeGreaterThan(reads));
  await waitFor(async () => expect((await dot()).getAttribute("data-state")).toBe("fail"));
  expect((await dot()).getAttribute("title")).toContain("上次测试失败");
});
