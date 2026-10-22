// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { EditConn } from "./EditConn";
import { boot, STORAGE } from "../i18n";
import type { ProviderEdit, ProviderEntry } from "../port/port";
import type { Port } from "./Providers";

const base: ProviderEntry = {
  name: "relay", kind: "responses", baseUrl: "https://relay.example/v1",
  models: ["alpha"], default: "alpha", hasKey: true, inUse: false, preset: false,
  balanceFormats: ["deepseek", "moonshot", "openrouter"],
};

function Host({ entry, port }: { entry: ProviderEntry; port: Port }) {
  const [busy, setBusy] = useState("");
  return <EditConn entry={entry} label="Relay" port={port} busy={busy} setBusy={setBusy} onDone={() => {}} onRevert={() => {}} />;
}

function draw(entry: ProviderEntry) {
  localStorage.setItem(STORAGE, "zh");
  boot();
  const editProvider = vi.fn<(edit: ProviderEdit) => Promise<void>>(async () => {});
  render(<Host entry={entry} port={{ protocols: async () => [], editProvider } as unknown as Port} />);
  return { editProvider };
}

afterEach(() => {
  cleanup();
  localStorage.setItem(STORAGE, "zh");
  boot();
});

async function openCompat(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByText("思考参数与推理档位"));
}

it("shows no format choice while there is no address", async () => {
  const user = userEvent.setup();
  draw(base);
  await openCompat(user);
  expect(screen.getByRole("textbox", { name: "余额地址" })).toBeTruthy();
  expect(screen.queryByRole("combobox", { name: "余额格式" })).toBeNull();
});

it("sends the address and the chosen format together, only once they changed", async () => {
  const user = userEvent.setup();
  const { editProvider } = draw(base);
  await openCompat(user);
  await user.type(screen.getByRole("textbox", { name: "余额地址" }), "https://relay.example/user/balance");
  await user.selectOptions(screen.getByRole("combobox", { name: "余额格式" }), "moonshot");
  await user.click(screen.getByRole("button", { name: "保存" }));
  expect(editProvider.mock.calls[0][0]).toMatchObject({ balanceUrl: "https://relay.example/user/balance", balanceFormat: "moonshot" });
});

it("says so when a saved relay address has no format, and not when its host implies one", async () => {
  const user = userEvent.setup();
  draw({ ...base, balanceUrl: "https://relay.example/user/balance", impliedBalanceFormat: "" });
  await openCompat(user);
  expect(screen.getByText(/余额格式未配置/)).toBeTruthy();
  cleanup();
  draw({ ...base, balanceUrl: "https://api.deepseek.com/user/balance", impliedBalanceFormat: "deepseek" });
  await openCompat(userEvent.setup());
  expect(screen.queryByText(/余额格式未配置/)).toBeNull();
  expect(screen.getByRole("option", { name: "按地址推断（DeepSeek 格式）" })).toBeTruthy();
});

it("sends nothing about the wallet when it was not touched", async () => {
  const user = userEvent.setup();
  const { editProvider } = draw({ ...base, balanceUrl: "https://relay.example/user/balance", balanceFormat: "deepseek" });
  await openCompat(user);
  await user.click(screen.getByText("思考参数与推理档位"));
  expect(editProvider).not.toHaveBeenCalled();
});
