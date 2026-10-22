// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Shell } from "./Shell";
import { MockPort } from "../port/mock";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const WSL = "C:\\Windows\\System32\\wsl.exe";
const openPinned = async () => {
  const port = new MockPort();
  const save = vi.spyOn(port, "saveShell");
  render(<Shell port={port} />);
  await userEvent.click(await screen.findByText("指定一个可执行文件"));
  return { port, save };
};
const args = () => screen.getByRole<HTMLTextAreaElement>("textbox", { name: /启动参数/ });
const path = () => screen.getByRole<HTMLInputElement>("textbox", { name: "可执行文件路径" });

it("sends launcher arguments one per line, untouched, and shows the command that was verified", async () => {
  const { save } = await openPinned();
  fireEvent.change(path(), { target: { value: WSL } });
  fireEvent.change(args(), { target: { value: "-d\nmy distro\n--\n/bin/bash\n" } });
  await userEvent.click(screen.getByRole("button", { name: "保存该路径" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("bash", WSL, ["-d", "my distro", "--", "/bin/bash"]));
  expect((await screen.findByText(/启动命令/)).nextElementSibling?.textContent).toBe(`${WSL} -d my distro -- /bin/bash -c …`);
  expect(args().value).toBe("-d\nmy distro\n--\n/bin/bash");
});

it("saves a plain path exactly as before when no arguments are typed", async () => {
  const { save } = await openPinned();
  fireEvent.change(path(), { target: { value: "C:\\Program Files\\Git\\usr\\bin\\bash.exe" } });
  await userEvent.click(screen.getByRole("button", { name: "保存该路径" }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0]).toEqual(["bash", "C:\\Program Files\\Git\\usr\\bin\\bash.exe"]);
});

it("keeps what was typed and says why when the launcher does not run", async () => {
  const { port } = await openPinned();
  vi.spyOn(port, "saveShell").mockRejectedValueOnce(new Error("这个 shell 用不了：wsl.exe: did not run a command"));
  fireEvent.change(path(), { target: { value: WSL } });
  fireEvent.change(args(), { target: { value: "-x" } });
  await userEvent.click(screen.getByRole("button", { name: "保存该路径" }));
  expect((await screen.findByRole("alert")).textContent).toContain("did not run a command");
  expect(args().value).toBe("-x");
  expect(screen.queryByText("启动命令")).toBeNull();
});

it("clearing the pinned program clears its arguments with it", async () => {
  const { port, save } = await openPinned();
  fireEvent.change(path(), { target: { value: WSL } });
  fireEvent.change(args(), { target: { value: "-d\nx\n--\n/bin/sh" } });
  await userEvent.click(screen.getByRole("button", { name: "保存该路径" }));
  await screen.findByText(/启动命令/);
  await userEvent.click(screen.getByRole("button", { name: "取消指定" }));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith("bash", ""));
  await waitFor(() => expect(screen.queryByText("启动命令")).toBeNull());
  expect((await port.shell()).args).toBeUndefined();
  expect(args().value).toBe("");
});
