// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { SessionRef } from "./SessionRef";
import type { AgentPort } from "../port/port";
import type { SessionRefSettings } from "../port/boundary";
import { HttpError } from "../port/port";

afterEach(cleanup);

const base: SessionRefSettings = { tokens: 8000, defaultTokens: 8000, minTokens: 500, maxTokens: 200000, path: "/u/config.toml" };

function portWith(save: (n: number) => Promise<SessionRefSettings>, initial = base) {
  return { sessionRefSettings: vi.fn(async () => initial), saveSessionRefSettings: vi.fn(save) } as unknown as AgentPort & { saveSessionRefSettings: ReturnType<typeof vi.fn> };
}

describe("the referenced-conversation size setting", () => {
  it("shows the current limit and has nothing to restore at the default", async () => {
    const port = portWith(async (n) => ({ ...base, tokens: n }));
    render(<SessionRef port={port} />);
    const field = (await screen.findByRole("textbox", { name: "引用的最大长度" })) as HTMLInputElement;
    expect(field.value).toBe("8000");
    expect((screen.getByRole("button", { name: "恢复默认" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("commits a new limit once, on Enter, and then offers the way back", async () => {
    const port = portWith(async (n) => ({ ...base, tokens: n || base.defaultTokens }));
    render(<SessionRef port={port} />);
    const field = await screen.findByRole("textbox", { name: "引用的最大长度" });
    await userEvent.clear(field);
    await userEvent.type(field, "12000{Enter}");
    expect(port.saveSessionRefSettings).toHaveBeenCalledTimes(1);
    expect(port.saveSessionRefSettings).toHaveBeenCalledWith(12000);
    const restore = screen.getByRole("button", { name: "恢复默认" }) as HTMLButtonElement;
    await waitFor(() => expect(restore.disabled).toBe(false));
    await userEvent.click(restore);
    expect(port.saveSessionRefSettings).toHaveBeenLastCalledWith(0);
    await waitFor(() => expect((field as HTMLInputElement).value).toBe("8000"));
  });

  it("refuses a limit out of range without writing", async () => {
    const port = portWith(async (n) => ({ ...base, tokens: n }));
    render(<SessionRef port={port} />);
    const field = await screen.findByRole("textbox", { name: "引用的最大长度" });
    await userEvent.clear(field);
    await userEvent.type(field, "50{Enter}");
    expect(port.saveSessionRefSettings).not.toHaveBeenCalled();
    expect(screen.getByText("请输入 500 到 200000 之间的整数。")).toBeTruthy();
  });

  it("says why when the kernel refuses a save, and keeps the stored limit", async () => {
    const port = portWith(async () => { throw new HttpError(400, "bad", { code: "session_ref.out_of_range", error: "tokens must be between 500 and 200000" }); });
    render(<SessionRef port={port} />);
    const field = (await screen.findByRole("textbox", { name: "引用的最大长度" })) as HTMLInputElement;
    await userEvent.clear(field);
    await userEvent.type(field, "9000{Enter}");
    expect(await screen.findByText(/长度上限超出允许范围/)).toBeTruthy();
    expect(port.saveSessionRefSettings).toHaveBeenCalledWith(9000);
  });

  it("says so when the setting cannot be read", async () => {
    const port = { sessionRefSettings: vi.fn(async () => { throw new Error("down"); }), saveSessionRefSettings: vi.fn() } as unknown as AgentPort;
    render(<SessionRef port={port} />);
    expect(await screen.findByText("无法读取引用设置。")).toBeTruthy();
  });
});
