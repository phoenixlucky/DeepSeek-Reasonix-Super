// @vitest-environment jsdom
import "./testkit";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { UserCard } from "./cards/UserCard";
import { clock } from "../i18n/format";

afterEach(cleanup);

const at = new Date(2026, 9, 17, 18, 33).getTime();
const mine = (extra: object = {}) => ({ t: "user" as const, id: "u1", text: "需要", ...extra });

describe("the time under the person's own message", () => {
  it("is drawn in the message's action row as a time element", () => {
    const { container } = render(<UserCard item={mine({ at })} />);
    const time = container.querySelector(".user-acts time");
    expect(time?.textContent).toBe(clock(at));
    expect(time?.getAttribute("datetime")).toBe(new Date(at).toISOString());
  });

  it("is not drawn when the message has no time, and the copy button stays", () => {
    const { container } = render(<UserCard item={mine()} />);
    expect(container.querySelector(".user-acts time")).toBeNull();
    expect(container.querySelector(".user-acts button")).not.toBeNull();
  });
});
