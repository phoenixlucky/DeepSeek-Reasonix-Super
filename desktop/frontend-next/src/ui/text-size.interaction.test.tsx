// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import { Appearance } from "./Appearance";
import { apply } from "./look";
import { MockPort } from "../port/mock";
import { boot, STORAGE } from "../i18n";
import type { AgentPort, Appearance as Look } from "../port/port";

beforeEach(() => {
  localStorage.setItem(STORAGE, "zh");
  boot();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  apply(null);
});

function draw(look: Look = {}) {
  const onLook = vi.fn();
  render(<Appearance port={new MockPort() as unknown as AgentPort} theme="light" onTheme={() => {}} contrast="" onContrast={() => {}}
    weight="" onWeight={() => {}} look={look} onLook={onLook} reloadThemes={() => {}} />);
  return onLook;
}

const group = () => screen.getByRole("group", { name: "界面文字大小" });
const pressed = () => [...group().querySelectorAll("button")].filter((b) => b.getAttribute("aria-pressed") === "true").map((b) => b.textContent);

describe("the interface text size", () => {
  it("is its own row, apart from the interface zoom and the body size", () => {
    draw();
    expect(group()).toBeTruthy();
    expect(screen.getByRole("group", { name: "界面大小" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "正文字号" })).toBeTruthy();
  });

  it("reads standard until a size is chosen, and shows the one chosen", () => {
    draw();
    expect(pressed()).toEqual(["标准"]);
    cleanup();
    draw({ textSize: 1.15 });
    expect(pressed()).toEqual(["大"]);
  });

  it("saves a choice as its scale and touches nothing else", async () => {
    const onLook = draw({ zoom: 1.2, readSize: 15 });
    await userEvent.click(within(group()).getByRole("button", { name: "更大" }));
    expect(onLook).toHaveBeenCalledExactlyOnceWith({ zoom: 1.2, readSize: 15, textSize: 1.3 });
  });

  it("saves standard as unset, so the config keeps no number for the default", async () => {
    const onLook = draw({ textSize: 1.3 });
    await userEvent.click(within(group()).getByRole("button", { name: "标准" }));
    expect(onLook).toHaveBeenCalledExactlyOnceWith({ textSize: 0 });
  });
});

describe("applying the look", () => {
  it("moves the text scale on the root, and leaves zoom and body size alone", () => {
    apply({ textSize: 1.15, zoom: 1.1, readSize: 15 });
    const root = document.documentElement.style;
    expect(root.getPropertyValue("--fs-scale")).toBe("1.15");
    expect(root.getPropertyValue("--zoom")).toBe("1.1");
    expect(root.getPropertyValue("--read")).toBe("15px");
  });

  it("takes the scale off again for the default and for no look at all", () => {
    apply({ textSize: 1.15 });
    apply({});
    expect(document.documentElement.style.getPropertyValue("--fs-scale")).toBe("");
    apply({ textSize: 1.3 });
    apply(null);
    expect(document.documentElement.style.getPropertyValue("--fs-scale")).toBe("");
    apply({ textSize: 1 });
    expect(document.documentElement.style.getPropertyValue("--fs-scale")).toBe("");
  });
});
