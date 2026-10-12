import type { ExtensionViewNode } from "../../port/wire";

interface Props {
  node: ExtensionViewNode;
  value: unknown;
  disabled: boolean;
  change: (value: unknown) => void;
}

export function SurfaceField({ node, value, disabled, change }: Props) {
  const f = node.field!;
  return <div className="ask-pane" data-on="">
    <div className="ask-q">{node.label}</div>
    {f.type === "choice" || f.type === "multichoice" ? <div className="opts" aria-label={node.label}>
      {f.options?.map(o => {
        const on = f.type === "choice" ? value === o.value : Array.isArray(value) && value.includes(o.value);
        return <button type="button" key={o.value} className="opt" data-multi={f.type === "multichoice" ? "" : undefined} data-on={on ? "" : undefined} aria-pressed={on} disabled={disabled} onClick={() => {
          if (f.type === "choice") change(on ? undefined : o.value);
          else { const chosen = Array.isArray(value) ? value : []; change(on ? chosen.filter(v => v !== o.value) : [...chosen, o.value]); }
        }}><span className="mark" /><span className="txt"><span className="lb">{o.label}</span>{o.description && <span className="ds">{o.description}</span>}{o.preview && <span className="ds">{o.preview}</span>}</span></button>;
      })}
    </div> : f.type === "bool" ? <button type="button" className="opt" aria-label={node.label} aria-pressed={value === true} data-on={value === true ? "" : undefined} disabled={disabled} onClick={() => change(value !== true)}><span className="mark" /><span className="txt"><span className="lb">{node.label}</span></span></button>
    : <div className="other-wrap" data-on=""><input aria-label={node.label} type={f.type === "number" ? "number" : "text"} min={f.min} max={f.max} step="any" value={value === undefined ? "" : String(value)} readOnly={disabled} onChange={e => change(e.target.value)} /></div>}
  </div>;
}
