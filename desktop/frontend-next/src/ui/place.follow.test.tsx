// @vitest-environment jsdom
import { useRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useFollow } from "./place";

afterEach(cleanup);

it("follows only its anchor/container transitions and releases listeners on close", () => {
  const place = vi.fn();
  function Popup({ open }: { open: boolean }) {
    const anchor = useRef<HTMLButtonElement>(null);
    const container = useRef<HTMLDivElement>(null);
    useFollow(open, place, undefined, { anchor, container });
    return <><section data-testid="parent"><button ref={anchor}>anchor</button></section>
      <div ref={container}>popup</div><div>unrelated</div></>;
  }
  const view = render(<Popup open />);
  place.mockClear();
  fireEvent.transitionEnd(screen.getByText("unrelated"));
  expect(place).not.toHaveBeenCalled();
  fireEvent.transitionEnd(screen.getByText("anchor"));
  fireEvent.transitionEnd(screen.getByText("popup"));
  fireEvent.transitionEnd(screen.getByTestId("parent"));
  expect(place).toHaveBeenCalledTimes(3);
  fireEvent.resize(window);
  expect(place).toHaveBeenCalledTimes(4);
  view.rerender(<Popup open={false} />);
  fireEvent.transitionEnd(screen.getByText("anchor"));
  fireEvent.resize(window);
  expect(place).toHaveBeenCalledTimes(4);
});
