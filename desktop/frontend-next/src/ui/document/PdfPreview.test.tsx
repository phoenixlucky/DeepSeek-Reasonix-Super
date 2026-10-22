// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

const pdf = vi.hoisted(() => ({ load: vi.fn(), destroy: vi.fn() }));
vi.mock("pdfjs-dist", () => ({
  GlobalWorkerOptions: {}, TextLayer: vi.fn(), getDocument: pdf.load,
}));
import PdfPreview from "./PdfPreview";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks(); });

it("keeps XFA off and releases the loading task when the file changes or closes", () => {
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  pdf.load.mockImplementation(() => ({ promise: new Promise(() => {}), destroy: pdf.destroy }));
  const data = new ArrayBuffer(16);
  const { rerender, unmount } = render(<PdfPreview data={data} view={{}} />);
  expect(pdf.load).toHaveBeenCalledWith(expect.objectContaining({ enableXfa: false }));
  expect(pdf.load.mock.calls[0][0].data).not.toBe(data);
  rerender(<PdfPreview data={new ArrayBuffer(32)} view={{}} />);
  expect(pdf.destroy).toHaveBeenCalledTimes(1);
  expect(pdf.load).toHaveBeenLastCalledWith(expect.objectContaining({ enableXfa: false }));
  unmount();
  expect(pdf.destroy).toHaveBeenCalledTimes(2);
});
