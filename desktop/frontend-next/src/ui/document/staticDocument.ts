import DOMPurify from "dompurify";

export function staticDocument(source: string): string {
  const clean = DOMPurify.sanitize(source, {
    WHOLE_DOCUMENT: true,
    ADD_TAGS: ["style"],
    FORBID_TAGS: ["script", "iframe", "object", "embed", "form", "input", "button", "textarea", "select", "link", "base", "meta"],
    FORBID_ATTR: ["srcdoc", "srcset", "action", "formaction", "href", "xlink:href", "ping", "target"],
  });
  const doc = new DOMParser().parseFromString(clean, "text/html");
  for (const el of doc.querySelectorAll("[src]")) {
    const src = el.getAttribute("src") ?? "";
    if (el.tagName !== "IMG" || !/^data:image\/(png|jpeg|gif|webp|avif|bmp|x-icon);base64,/i.test(src)) el.removeAttribute("src");
  }
  const policy = doc.createElement("meta");
  policy.httpEquiv = "Content-Security-Policy";
  policy.content = "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'";
  doc.head.prepend(policy);
  return "<!doctype html>" + doc.documentElement.outerHTML;
}
