// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SayCard } from "./SayCard";
import { FOLD_CHOICES, FOLD_DEFAULTS, setFoldModes } from "../../state/prefs";
import { firstSentence } from "../thoughtPeek";

afterEach(() => {
  cleanup();
  setFoldModes(FOLD_DEFAULTS);
});

const item = { t: "say" as const, id: "s", text: "answer", reasoning: "先检查配置文件。然后再看日志。", done: true };
const peek = (el: HTMLElement) => el.querySelector(".peek");

describe("firstSentence", () => {
  it("takes the first sentence of Chinese and Latin text", () => {
    expect(firstSentence("先检查配置文件。然后再看日志。", 80)).toBe("先检查配置文件。");
    expect(firstSentence("Check the config first. Then the logs.", 80)).toBe("Check the config first.");
  });
  it("cuts a long first sentence at the limit with an ellipsis", () => {
    const out = firstSentence("a".repeat(200), 20);
    expect([...out]).toHaveLength(21);
    expect(out.endsWith("…")).toBe(true);
  });
  it("ignores leading blank lines and whitespace", () => {
    expect(firstSentence("\n\n  hello there.  More", 80)).toBe("hello there.");
  });
  it("is empty for blank reasoning", () => {
    expect(firstSentence("  \n ", 80)).toBe("");
  });
});

describe("the thinking peek", () => {
  it("is a choice for thinking only, and not the default", () => {
    expect(FOLD_CHOICES.thinking).toContain("peek");
    expect(FOLD_CHOICES.activity).not.toContain("peek");
    expect(FOLD_DEFAULTS.thinking).not.toBe("peek");
  });

  it("shows nothing beside a folded block under the default", () => {
    expect(peek(render(<SayCard item={item} />).container)).toBeNull();
  });

  it("shows the first sentence beside a folded block and hides it once opened", async () => {
    setFoldModes({ thinking: "peek" });
    const { container } = render(<SayCard item={item} />);
    expect(peek(container)?.textContent).toBe("先检查配置文件。");
    expect((container.querySelector("details") as HTMLDetailsElement).open).toBe(false);
    await userEvent.click(container.querySelector("summary") as HTMLElement);
    expect(peek(container)).toBeNull();
  });

  it("follows the setting for a block already on screen", () => {
    const { container } = render(<SayCard item={item} />);
    expect(peek(container)).toBeNull();
    act(() => setFoldModes({ thinking: "peek" }));
    expect(peek(container)).not.toBeNull();
  });

  it("shows no peek while the thought is still streaming", () => {
    setFoldModes({ thinking: "peek" });
    expect(peek(render(<SayCard item={{ ...item, done: false, text: "" }} />).container)).toBeNull();
  });
});
