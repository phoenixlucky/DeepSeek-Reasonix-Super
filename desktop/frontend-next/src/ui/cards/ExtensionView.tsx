import type { ExtensionViewNode } from "../../port/wire";
import type { ReactNode } from "react";

// The host half of the composed-surface contract: an extension sends a tree of
// primitives, and every one of them is drawn here with the components this app
// already uses. Nothing in a node names a size, a position or a colour, so a
// view cannot drift away from the rest of the window — and the same tree can be
// rendered as text by a terminal frontend that has none of these elements.

interface Props {
  body: ExtensionViewNode[];
  onAction?: (actionId: string) => void;
  renderField?: (node: ExtensionViewNode) => ReactNode;
}

export function ExtensionView({ body, onAction, renderField }: Props) {
  return (
    <div className="xview">
      <Nodes nodes={body} onAction={onAction} renderField={renderField} />
    </div>
  );
}

function Nodes({ nodes, onAction, renderField }: { nodes: ExtensionViewNode[]; onAction?: (id: string) => void; renderField?: Props["renderField"] }) {
  return (
    <>
      {nodes.map((n, i) => (
        <Node key={`${n.kind}:${n.key ?? n.actionId ?? n.value ?? i}`} n={n} onAction={onAction} renderField={renderField} />
      ))}
    </>
  );
}

function Node({ n, onAction, renderField }: { n: ExtensionViewNode; onAction?: (id: string) => void; renderField?: Props["renderField"] }) {
  switch (n.kind) {
    case "text":
      return <span className="xv-t" data-tone={n.tone}>{n.value}</span>;
    // Markdown is the one primitive that carries a document rather than a
    // value. It is deliberately rendered as plain text here: the transcript's
    // renderer assumes trusted content, and a view is not that.
    case "markdown":
      return <p className="xv-md">{n.value}</p>;
    case "row":
      return (
        <div className="xv-row">
          <Nodes nodes={n.children ?? []} onAction={onAction} renderField={renderField} />
        </div>
      );
    case "stack":
      return (
        <div className="xv-stack">
          <Nodes nodes={n.children ?? []} onAction={onAction} renderField={renderField} />
        </div>
      );
    case "kv":
      return (
        <div className="xv-kv">
          <span className="k">{n.key}</span>
          <span className="v">{n.value}</span>
        </div>
      );
    case "meter":
      return (
        <div className="xv-meter" role="progressbar" aria-valuenow={Math.round((n.progress ?? 0) * 100)}>
          <i style={{ width: `${Math.round(Math.max(0, Math.min(1, n.progress ?? 0)) * 100)}%` }} />
          {n.label && <span className="lb">{n.label}</span>}
        </div>
      );
    case "pip":
      return <i className="pip" data-tone={n.tone} />;
    case "button":
      return (
        <button className="act" data-action="extensions.invoke" onClick={() => n.actionId && onAction?.(n.actionId)}>
          {n.label}
        </button>
      );
    case "divider":
      return <hr className="xv-hr" />;
    case "field": case "choice_card":
      return renderField?.(n) ?? null;
    default:
      // A node kind this build does not know is skipped rather than shown as
      // a blank: an older frontend meeting a newer extension should look like
      // it is missing a feature, not like it is broken.
      return null;
  }
}
