// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import "./testkit";
import { Picker } from "./Menu";
import { chosenGroups, groupHeadings, modelMenu, narrowToGroups } from "./modelmenu";
import { ModelGroupChips } from "./ModelGroupChips";
import { boot } from "../i18n";
import { MODEL_FILTER_KEY, useModelFilter } from "../state/modelfilter";
import { writeModelFilter } from "../state/prefs";
import type { ModelEntry } from "../port/port";

afterEach(() => {
  cleanup();
  localStorage.clear();
  boot();
});

const fleet = (hosts = ["api.deepseek.com", "api.moonshot.cn", "relay.example.com"]): ModelEntry[] =>
  hosts.flatMap((host) => {
    const name = host.split(".")[0] === "api" ? host.split(".")[1] : host.split(".")[0];
    return [0, 1].map((i) => ({ ref: `${name}/m${i}`, provider: name, vendor: host, model: `${name}-model-${i}`, kind: "openai" }));
  });

function Menu({ models, current }: { models: ModelEntry[]; current?: string }) {
  const selected = useModelFilter();
  const all = modelMenu(models, [], current);
  const chosen = chosenGroups(all, selected);
  return (
    <Picker label="model" place="bottom" current={current} items={narrowToGroups(all, chosen)} searchAlways menuClassName="studio-model-menu"
      head={<ModelGroupChips headings={groupHeadings(all)} chosen={chosen} onChange={writeModelFilter} />} onPick={vi.fn()} />
  );
}

const open = () => fireEvent.click(screen.getByRole("button", { name: "model" }));
const chip = (name: string) => screen.getByRole("button", { name });
const keyOf = (name: string) => chip(name).getAttribute("data-target") ?? "";
const heads = () => [...document.querySelectorAll(".studio-model-menu .mi.head .lb")].map((n) => n.textContent);
const rows = () => document.querySelectorAll(".studio-model-menu button.mi:not(.plain)").length;

describe("narrowing the model menu to chosen providers", () => {
  it("offers one chip per provider, and shows every provider until one is chosen", () => {
    render(<Menu models={fleet()} />);
    open();
    expect(["deepseek", "moonshot", "relay"].every((name) => chip(name).getAttribute("aria-pressed") === "false")).toBe(true);
    expect(heads()).toEqual(["deepseek", "moonshot", "relay"]);
  });

  it("narrows to the chosen providers, combines with the search, and clears", () => {
    render(<Menu models={fleet()} />);
    open();
    fireEvent.click(chip("moonshot"));
    fireEvent.click(chip("relay"));
    expect(heads()).toEqual(["moonshot", "relay"]);
    expect(rows()).toBe(4);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "relay-model-1" } });
    expect(rows()).toBe(1);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: /清除|Clear/ }));
    expect(heads()).toEqual(["deepseek", "moonshot", "relay"]);
  });

  it("remembers the choice for the next window, keyed by the account", () => {
    const first = render(<Menu models={fleet()} />);
    open();
    fireEvent.click(chip("moonshot"));
    expect(JSON.parse(localStorage.getItem(MODEL_FILTER_KEY) ?? "[]")).toEqual([keyOf("moonshot")]);
    first.unmount();
    render(<Menu models={fleet()} />);
    open();
    expect(heads()).toEqual(["moonshot"]);
  });

  it("keeps the current model on screen whatever is chosen, and never empties the list for a provider that is gone", () => {
    render(<Menu models={fleet()} current="deepseek/m0" />);
    open();
    const relay = keyOf("relay");
    act(() => { writeModelFilter([relay]); });
    expect(document.querySelector('.studio-model-menu button.mi[data-value="deepseek/m0"]')).not.toBeNull();
    expect(heads()).toEqual(["relay"]);
    act(() => { writeModelFilter(["__account:removed-long-ago"]); });
    expect(heads()).toEqual(["deepseek", "moonshot", "relay"]);
  });

  it("draws no chips for a single provider", () => {
    render(<Menu models={fleet(["api.deepseek.com"])} />);
    open();
    expect(document.querySelector(".mchips")).toBeNull();
  });
});
