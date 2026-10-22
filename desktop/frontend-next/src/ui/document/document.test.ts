// @vitest-environment jsdom
import { expect, it } from "vitest";
import { staticDocument } from "./staticDocument";
import { parseDelimited } from "./parseDelimited";

it("keeps CSV values literal including multiline quoted cells and leading zeros", () => {
  expect(parseDelimited('id,value\n001,"hello\nworld"\n002,"=SUM(A1:A2)"', ",")).toEqual([
    ["id", "value"], ["001", "hello\nworld"], ["002", "=SUM(A1:A2)"],
  ]);
  expect(parseDelimited('a\t"b\tc"\n1\t2', "\t")).toEqual([["a", "b\tc"], ["1", "2"]]);
  expect(() => parseDelimited('"unterminated', ",")).toThrow();
  expect(() => parseDelimited("a,b,c", ",", 2)).toThrow();
});

it("static HTML cannot execute or load remote resources", () => {
  const html = staticDocument('<html><head><meta http-equiv="refresh" content="0;url=https://bad.test"><style>p{color:red}</style></head><body><script>alert(1)</script><p onclick="alert(1)">hello</p><iframe src="https://bad.test"></iframe><form action="https://bad.test"><input></form><img src="https://bad.test/x"><a href="https://bad.test">link</a></body></html>');
  const doc = new DOMParser().parseFromString(html, "text/html");
  expect(doc.querySelector("script,iframe,form,input,[onclick],[href]")).toBeNull();
  expect(doc.querySelector('meta[http-equiv="refresh"]')).toBeNull();
  expect(doc.querySelector("img")?.getAttribute("src")).toBeNull();
  expect(doc.querySelector("p")?.textContent).toBe("hello");
  expect(doc.querySelector("style")?.textContent).toContain("color:red");
  expect(doc.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute("content")).toContain("default-src 'none'");
});
