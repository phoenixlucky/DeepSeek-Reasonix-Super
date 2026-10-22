// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import "./testkit";
import { Composer } from "./Composer";
import { MockPort } from "../port/mock";
import type { Completion, SessionStatus } from "../port/port";

afterEach(() => { cleanup(); localStorage.clear(); vi.restoreAllMocks(); });

function draw() {
  const port = new MockPort();
  const onSubmit = vi.fn(async () => true);
  const view = render(<Composer
    port={port}
    status={{ plan: false, effort: "auto", modelRef: "deepseek/deepseek-v4-pro" } as SessionStatus}
    running={false} focus={0} onSubmit={onSubmit} onChanged={vi.fn()} onError={vi.fn()}
  />);
  const box = screen.getByRole("combobox", { name: "任务输入" }) as HTMLTextAreaElement;
  const type = (value: string, caret = value.length) => fireEvent.change(box, { target: { value, selectionStart: caret } });
  return { ...view, port, onSubmit, box, type };
}

it.each([
  ["@inter", "internal/", "@internal/"],
  ["@REAS", "REASONIX.md", "@REASONIX.md"],
])("confirms the default reference for %s before allowing Enter to send", async (query, label, reference) => {
  const { box, type, onSubmit } = draw();
  type(query);
  const option = await screen.findByRole("option", { name: new RegExp(label) });
  expect(option.getAttribute("aria-selected")).toBe("true");
  expect(box.getAttribute("aria-activedescendant")).toBe(option.id);

  fireEvent.keyDown(box, { key: "Enter" });
  expect(onSubmit).not.toHaveBeenCalled();
  expect(box.value).toBe(reference + " ");
  await waitFor(() => expect(screen.queryByRole("listbox", { name: "补全" })).toBeNull());
  expect(box.selectionStart).toBe(box.value.length);
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith(reference));
});

it.each([
  ["@REAS", "@REASONIX.md"],
  ["查看 @REAS", "查看 @REASONIX.md"],
  ["@internal/control/compl", "@internal/control/complete.go"],
])("sends an exact reference in one Enter when no candidates remain (%s)", async (query, exact) => {
  const { box, type, onSubmit } = draw();
  type(query);
  await screen.findByRole("listbox", { name: "补全" });
  type(exact);
  await waitFor(() => expect(screen.queryByRole("listbox", { name: "补全" })).toBeNull());
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith(exact));
});

it("confirms a genuine alternative while an exact reference is also a path prefix", async () => {
  const { port, box, type, onSubmit } = draw();
  const query = "@notes.md";
  vi.spyOn(port, "complete").mockImplementation(async (line): Promise<Completion> => line === query
    ? { kind: "ref", from: 0, to: query.length,
      items: [{ label: "notes.md.bak", insert: "@notes.md.bak", kind: "file" }] }
    : { kind: "", from: 0, to: 0, items: [] });
  type(query);
  await screen.findByRole("option", { name: /notes.md.bak/ });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(onSubmit).not.toHaveBeenCalled();
  expect(box.value).toBe("@notes.md.bak ");
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@notes.md.bak"));
});

it("uses Tab to descend through directories and Enter to finish a nested file reference", async () => {
  const { box, type, onSubmit } = draw();
  type("@inter");
  await screen.findByRole("option", { name: /internal\// });
  fireEvent.keyDown(box, { key: "Tab" });
  expect(box.value).toBe("@internal/");
  await screen.findByRole("option", { name: /control\// });
  fireEvent.keyDown(box, { key: "Tab" });
  expect(box.value).toBe("@internal/control/");
  await screen.findByRole("option", { name: /complete.go/ });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("@internal/control/complete.go ");
  expect(onSubmit).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@internal/control/complete.go"));
});

it("uses Tab on a file to finish the reference and return to ordinary drafting", async () => {
  const { box, type, onSubmit } = draw();
  type("@REAS");
  await screen.findByRole("option", { name: /REASONIX.md/ });
  fireEvent.keyDown(box, { key: "Tab" });
  expect(box.value).toBe("@REASONIX.md ");
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  type(box.value + "请解释这个文件");
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@REASONIX.md 请解释这个文件"));
});

it("confirms the row selected with arrow keys rather than the default row", async () => {
  const { box, type, onSubmit } = draw();
  type("@internal/control/");
  await screen.findByRole("option", { name: /controller.go/ });
  fireEvent.keyDown(box, { key: "ArrowDown" });
  expect(screen.getByRole("option", { name: /controller.go/ }).getAttribute("aria-selected")).toBe("true");
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("@internal/control/controller.go ");
  expect(onSubmit).not.toHaveBeenCalled();
});

it("finishes a directory reference when its row is clicked", async () => {
  const { box, type, onSubmit } = draw();
  type("@inter");
  const option = await screen.findByRole("option", { name: /internal\// });
  fireEvent.mouseDown(option);
  expect(box.value).toBe("@internal/ ");
  expect(onSubmit).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@internal/"));
});

it.each([" ", "\n", "\t"])("preserves the suffix and existing %j separator when confirming an escaped path", async (separator) => {
  const { port, box, type, onSubmit } = draw();
  const prefix = "查看 ";
  const query = "@my\\ d";
  const before = prefix + query + separator + "再解释";
  const reference = "@my\\ dir/";
  vi.spyOn(port, "complete").mockImplementation(async (line): Promise<Completion> => line === before
    ? { kind: "ref", from: prefix.length, to: prefix.length + query.length,
      items: [{ label: "my dir/", insert: reference, kind: "dir", descend: true }] }
    : { kind: "", from: 0, to: 0, items: [] });
  type(before, prefix.length + query.length);
  await screen.findByRole("option", { name: /my dir\// });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe(prefix + reference + separator + "再解释");
  expect(box.selectionStart).toBe(prefix.length + reference.length + 1);
  expect(onSubmit).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
});

it("restores the configured modifier send shortcut after confirming a reference", async () => {
  localStorage.setItem("rx-send-shortcut", "modifier_enter");
  const { box, type, onSubmit } = draw();
  type("@inter");
  await screen.findByRole("option", { name: /internal\// });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("@internal/ ");
  expect(onSubmit).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(fireEvent.keyDown(box, { key: "Enter" })).toBe(true);
  expect(onSubmit).not.toHaveBeenCalled();
  fireEvent.keyDown(box, { key: "Enter", ctrlKey: true });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@internal/"));
});

it("leaves newline, reverse focus navigation, and IME confirmation out of reference acceptance", async () => {
  const { box, type, onSubmit } = draw();
  type("@inter");
  await screen.findByRole("option", { name: /internal\// });
  expect(fireEvent.keyDown(box, { key: "Enter", shiftKey: true })).toBe(true);
  expect(fireEvent.keyDown(box, { key: "Tab", shiftKey: true })).toBe(true);
  fireEvent.keyDown(box, { key: "Enter", isComposing: true });
  expect(box.value).toBe("@inter");
  expect(screen.getByRole("listbox")).toBeTruthy();
  expect(onSubmit).not.toHaveBeenCalled();
});

it("lets Escape dismiss the list before sending the draft as typed", async () => {
  const { box, type, onSubmit } = draw();
  type("@inter");
  await screen.findByRole("option", { name: /internal\// });
  fireEvent.keyDown(box, { key: "Escape" });
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(box.value).toBe("@inter");
  fireEvent.keyDown(box, { key: "Enter" });
  await waitFor(() => expect(onSubmit).toHaveBeenCalledExactlyOnceWith("@inter"));
});
