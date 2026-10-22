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

const entry: ProviderEntry = {
  name: "relay", kind: "responses", baseUrl: "https://relay.example/v1",
  models: ["alpha"], default: "alpha", hasKey: true, inUse: false, preset: false,
  canWebSearch: true, webSearch: false, canSetThinking: true, sendsThinking: true,
  canSetContinuation: true, continuation: "",
};

function Host({ port, onRevert }: { port: Port; onRevert: () => void }) {
  const [busy, setBusy] = useState("");
  const [revision, setRevision] = useState(0);
  return <EditConn key={revision} entry={entry} label="Relay" port={port} busy={busy} setBusy={setBusy}
    onDone={() => setRevision((n) => n + 1)} onRevert={() => { onRevert(); setRevision((n) => n + 1); }} />;
}

function draw() {
  localStorage.setItem(STORAGE, "zh");
  boot();
  const editProvider = vi.fn<(edit: ProviderEdit) => Promise<void>>(async () => {});
  const onRevert = vi.fn();
  const port = { protocols: async () => [], editProvider } as unknown as Port;
  render(<Host port={port} onRevert={onRevert} />);
  return { editProvider, onRevert };
}

const search = (on: boolean) => screen.getByRole("group", { name: "Relay 的联网搜索" }).querySelector<HTMLButtonElement>(`[data-value="web-search:${on}"]`)!;

afterEach(() => {
  cleanup();
  localStorage.setItem(STORAGE, "zh");
  boot();
});

it("holds the switches in the draft until Save, then sends them with the rest", async () => {
  const user = userEvent.setup();
  const { editProvider } = draw();
  const save = screen.getByRole("button", { name: "保存" }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);

  await user.click(search(true));
  await user.click(screen.getByRole("button", { name: "不发送" }));
  await user.click(screen.getByRole("button", { name: "每轮完整发送" }));
  expect(editProvider).not.toHaveBeenCalled();
  expect(save.disabled).toBe(false);
  expect(screen.getByText("有未保存的更改")).toBeTruthy();

  await user.click(save);
  expect(editProvider).toHaveBeenCalledTimes(1);
  expect(editProvider.mock.calls[0][0]).toMatchObject({ name: "relay", webSearch: true, reasoningProtocol: "none", continuation: "stateless" });
});

it("counts a switch put back as no change", async () => {
  const user = userEvent.setup();
  draw();
  await user.click(screen.getByRole("button", { name: "不发送" }));
  expect((screen.getByRole("button", { name: "保存" }) as HTMLButtonElement).disabled).toBe(false);
  await user.click(screen.getByRole("group", { name: "Relay 的思考参数" }).querySelector<HTMLButtonElement>('[data-value="thinking:true"]')!);
  expect((screen.getByRole("button", { name: "保存" }) as HTMLButtonElement).disabled).toBe(true);
});

it("leaves the declarations out of a save that did not change them", async () => {
  const user = userEvent.setup();
  const { editProvider } = draw();
  await user.click(screen.getByRole("button", { name: "不发送" }));
  await user.click(screen.getByRole("button", { name: "保存" }));
  const sent = editProvider.mock.calls[0][0];
  expect(sent).not.toHaveProperty("webSearch");
  expect(sent).not.toHaveProperty("continuation");
});

it("Revert puts every switch back to what is stored", async () => {
  const user = userEvent.setup();
  const { onRevert } = draw();
  await user.click(screen.getByRole("button", { name: "不发送" }));
  await user.click(screen.getByRole("button", { name: "引用上一轮" }));
  await user.click(screen.getByRole("button", { name: "还原" }));
  expect(onRevert).toHaveBeenCalled();
  expect((screen.getByRole("button", { name: "保存" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByRole("button", { name: "引用上一轮" }).getAttribute("aria-pressed")).toBe("false");
  expect(screen.getByRole("button", { name: "不发送" }).getAttribute("aria-pressed")).toBe("false");
});
