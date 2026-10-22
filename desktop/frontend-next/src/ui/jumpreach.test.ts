import { describe, expect, it } from "vitest";
import { EDGE, landing, reach } from "./jumpreach";

describe("which jump buttons the transcript offers", () => {
  it("offers none when nothing scrolls", () => {
    expect(reach(0, 600, 600)).toEqual({ up: false, down: false });
    expect(reach(0, 600, 640)).toEqual({ up: false, down: false });
  });

  it("hides an end's button within the margin of that end", () => {
    expect(reach(0, 600, 2000)).toEqual({ up: false, down: true });
    expect(reach(EDGE - 1, 600, 2000)).toEqual({ up: false, down: true });
    expect(reach(EDGE, 600, 2000)).toEqual({ up: true, down: true });
    expect(reach(1400, 600, 2000)).toEqual({ up: true, down: false });
    expect(reach(1400 - EDGE + 1, 600, 2000)).toEqual({ up: true, down: false });
    expect(reach(1400 - EDGE, 600, 2000)).toEqual({ up: true, down: true });
  });
});

describe("where a jump lands", () => {
  const height = 5000;
  const client = 600;

  it("goes up to the nearest expanded block above the view, not to the top", () => {
    expect(landing("up", [300, 1200, 2600, 4000], 2000, client, height)).toBe(1200 - 8);
  });

  it("goes down to the nearest expanded block below the view, not to the end", () => {
    expect(landing("down", [300, 1200, 2600, 4000], 2000, client, height)).toBe(2600 - 8);
  });

  it("ignores a block that is already in view", () => {
    expect(landing("down", [2100, 3500], 2000, client, height)).toBe(3500 - 8);
    expect(landing("up", [2100, 800], 2000, client, height)).toBe(800 - 8);
  });

  it("falls back to the very start or the very end when nothing is expanded that way", () => {
    expect(landing("up", [2600], 2000, client, height)).toBe(0);
    expect(landing("down", [300], 2000, client, height)).toBe("end");
    expect(landing("down", [], 2000, client, height)).toBe("end");
  });

  it("treats a target that would leave the bottom margin as the end itself", () => {
    expect(landing("down", [height - client - 10], 2000, client, height)).toBe("end");
  });

  it("never lands above the start", () => {
    expect(landing("up", [3], 2000, client, height)).toBe(0);
  });
});
