import { expect, it } from "vitest";
import type { ProviderEntry } from "../port/port";
import { providerDot } from "./providerdot";

const entry = (over: Partial<ProviderEntry>): ProviderEntry => ({
  name: "a", kind: "openai", baseUrl: "https://a.example/v1", models: [], default: "", hasKey: true, inUse: false, preset: false, ...over,
});

it("says nothing for an account nobody tested", () => {
  expect(providerDot([entry({})])).toEqual({ title: "" });
});

it("leads with a failed test and names its typed cause, even on the provider in use", () => {
  const dot = providerDot([entry({ inUse: true, lastCheck: { at: 1, ok: false, code: "provider.probe.unauthorized" } })]);
  expect(dot.state).toBe("fail");
  expect(dot.title).toContain("上次测试失败");
});

it("keeps the standing facts in their old order, and shows a pass only when nothing else is said", () => {
  expect(providerDot([entry({ inUse: true, lastCheck: { at: 1, ok: true } })]).state).toBe("use");
  expect(providerDot([entry({ hasKey: false, lastCheck: { at: 1, ok: true } })]).state).toBe("warn");
  expect(providerDot([entry({ lastCheck: { at: 1, ok: true } })]).state).toBe("pass");
});

it("reads any door of the account: one failing door is the account's news", () => {
  const dot = providerDot([entry({ lastCheck: { at: 1, ok: true } }), entry({ kind: "anthropic", lastCheck: { at: 2, ok: false } })]);
  expect(dot.state).toBe("fail");
});
