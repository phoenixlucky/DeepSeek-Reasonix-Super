// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MockHub } from "../port/mock_hub";
import { NativeSessionImport } from "./NativeSessionImport";
import { HttpError } from "../port/http_error";
import { boot, STORAGE } from "../i18n";

afterEach(() => { cleanup(); localStorage.setItem(STORAGE, "zh"); boot(); });

it("uploads the original file without reading or parsing it in the renderer", async () => {
  const hub = new MockHub();
  const imported = vi.spyOn(hub,"importNativeSession");
  const refresh = vi.fn();
  const archive = {format:"reasonix-native-session",version:2,records:[{role:"system",agentComposition:{digest:"unchanged"}}]};
  const file = new File([JSON.stringify(archive)],"session.json",{type:"application/json"});
  const read = vi.fn().mockRejectedValue(new Error("renderer must not read the archive"));
  Object.defineProperty(file,"text",{value:read});
  render(<NativeSessionImport hub={hub} workspace="/workspace" onImported={refresh}/>);
  await userEvent.upload(screen.getByLabelText("选择会话 JSON 文件"),file);
  expect((await screen.findByRole("status")).textContent).toContain("绑定版本保持不变");
  expect(imported).toHaveBeenCalledWith(file,"/workspace");
  expect(read).not.toHaveBeenCalled();
  expect(refresh).toHaveBeenCalledOnce();
});

it.each(["zh", "en"])("localizes a malformed upload by host code in %s", async (language) => {
  localStorage.setItem(STORAGE, language); boot();
  const hub = new MockHub();
  const refresh = vi.fn();
  vi.spyOn(hub, "importNativeSession").mockRejectedValue(new HttpError(400,
    "SyntaxError: unexpected byte", { code: "session.archive_invalid", error: "raw parser detail" }));
  const file = new File(["{broken"], "broken.json", { type: "application/json" });
  render(<NativeSessionImport hub={hub} workspace="/workspace" onImported={refresh}/>);
  const label = language === "zh" ? "选择会话 JSON 文件" : "Choose a session JSON file";
  await userEvent.upload(screen.getByLabelText(label), file);
  expect((await screen.findByRole("alert")).textContent).toBe(language === "zh"
    ? "会话文件无效，请选择完整的原生会话导出文件。"
    : "Invalid session file. Choose a complete native session export.");
  expect(refresh).not.toHaveBeenCalled();
});
