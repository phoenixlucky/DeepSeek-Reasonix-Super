// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { TraceExport } from "./TraceExport";
import { HttpError } from "../port/http_error";
import type { AgentPort, ObservabilitySettings } from "../port/port";

afterEach(cleanup);

const base = (over: Partial<ObservabilitySettings> = {}): ObservabilitySettings => ({
  enabled: false, endpoint: "", headerName: "", headerEnv: "", serviceName: "", headerEnvSet: false,
  path: "/Users/you/.reasonix/config.toml", applies: "next_session", ...over,
});

function draw(initial: ObservabilitySettings, save?: AgentPort["saveObservabilitySettings"]) {
  const saveObservabilitySettings = vi.fn(save ?? (async (patch) => ({ ...initial, ...patch })));
  const port = { observabilitySettings: vi.fn(async () => initial), saveObservabilitySettings } as unknown as AgentPort;
  render(<TraceExport port={port} />);
  return { saveObservabilitySettings };
}

describe("trace export setting", () => {
  it("starts off, and says that an address is needed as well", async () => {
    draw(base());
    expect(await screen.findByText("现在是关闭的：开启并填写收集器地址后才会发送。")).toBeTruthy();
    expect(screen.getByRole("switch", { name: "导出追踪" }).getAttribute("aria-checked")).toBe("false");
  });

  it("says which of the three states it is in", async () => {
    draw(base({ enabled: true }));
    expect(await screen.findByText("已开启，但还没有收集器地址，所以不会发送任何东西。")).toBeTruthy();
    cleanup();
    draw(base({ enabled: true, endpoint: "http://localhost:4318" }));
    expect(await screen.findByText("已开启：之后新开的会话会把追踪发到下面的地址。")).toBeTruthy();
  });

  it("turns the export on with exactly that one field", async () => {
    const { saveObservabilitySettings } = draw(base({ endpoint: "http://localhost:4318" }));
    await userEvent.click(await screen.findByRole("switch", { name: "导出追踪" }));
    expect(saveObservabilitySettings).toHaveBeenCalledExactlyOnceWith({ enabled: true });
  });

  it("saves a field on blur, only when it changed, and trims it", async () => {
    const { saveObservabilitySettings } = draw(base());
    const field = await screen.findByLabelText("收集器地址");
    await userEvent.click(field);
    await userEvent.click(document.body);
    expect(saveObservabilitySettings).not.toHaveBeenCalled();
    await userEvent.type(field, "  http://localhost:4318 ");
    await userEvent.click(document.body);
    expect(saveObservabilitySettings).toHaveBeenCalledExactlyOnceWith({ endpoint: "http://localhost:4318" });
  });

  it("says whether the credential variable has a value, and never shows one", async () => {
    draw(base({ headerEnv: "OTEL_AUTH", headerEnvSet: true }));
    expect(await screen.findByText("环境变量已设置；它的值不会显示，也不会写进配置文件。")).toBeTruthy();
    cleanup();
    draw(base({ headerEnv: "OTEL_AUTH", headerEnvSet: false }));
    expect(await screen.findByText("环境变量当前没有值：会不带凭据发送。")).toBeTruthy();
  });

  it("shows the kernel's refusal from its code and leaves the field editable", async () => {
    draw(base(), async () => { throw new HttpError(400, "x", { code: "observability.header_env_invalid", error: "bad" }); });
    const field = await screen.findByLabelText("保存凭据的环境变量");
    await userEvent.type(field, "AWS_SECRET_ACCESS_KEY");
    await userEvent.click(document.body);
    expect((await screen.findByRole("alert")).textContent).toContain("OTEL_");
    expect((field as HTMLInputElement).disabled).toBe(false);
  });

  it("goes read-only when the window has not been granted editing", async () => {
    draw(base(), async () => { throw new HttpError(403, "x", { code: "observability.editing_disabled", error: "no" }); });
    await userEvent.click(await screen.findByRole("switch", { name: "导出追踪" }));
    await waitFor(() => expect(screen.getByText("这些设置现在是只读的：这个窗口没有被授予编辑权限。")).toBeTruthy());
    expect((screen.getByLabelText("收集器地址") as HTMLInputElement).disabled).toBe(true);
  });
});
