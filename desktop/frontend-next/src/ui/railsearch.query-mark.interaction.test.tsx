// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import "./testkit";
import { RailSearch } from "./railsearch";

afterEach(cleanup);

it("marks the search box as holding a query for as long as it holds one, focus or not", () => {
  const { container } = render(<RailSearch><p>list</p></RailSearch>);
  const box = container.querySelector(".wsfind")!;
  const field = screen.getByRole("searchbox");
  expect(box.hasAttribute("data-query")).toBe(false);

  fireEvent.change(field, { target: { value: "zebra" } });
  fireEvent.blur(field);
  expect(box.hasAttribute("data-query")).toBe(true);

  fireEvent.keyDown(field, { key: "Escape" });
  expect(box.hasAttribute("data-query")).toBe(false);
});
