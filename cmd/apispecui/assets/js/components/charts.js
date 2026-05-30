// charts.js — dependency-free SVG visualizations for the Insight view:
// a radial Gauge, a Donut with legend, an Info tooltip, and a layered
// node-edge TraceDiagram for the route-scoped resolution trace.
import { html, useState, useRef } from "/assets/js/preact.js";

export const PALETTE = [
  "var(--accent)",
  "var(--accent-2)",
  "var(--info)",
  "var(--warn)",
  "var(--danger)",
  "#8b949e",
  "#d2a8ff",
  "#56d4dd",
];

const trunc = (s, n) => (s.length > n ? s.slice(0, n - 1) + "…" : s);

// Info — a small ⓘ that reveals an explanatory tooltip on hover/focus.
// The tooltip is position:fixed (placed from the icon's bounding rect) so
// it is never clipped by a scrolling/overflow container such as the
// Configure left panel.
export function Info({ text }) {
  const [tip, setTip] = useState(null);
  const ref = useRef(null);
  const show = () => {
    const r = ref.current && ref.current.getBoundingClientRect();
    if (r) setTip({ x: r.left + r.width / 2, top: r.top, bottom: r.bottom, below: r.top < 150 });
  };
  return html`<span
    class="info"
    ref=${ref}
    tabindex="0"
    aria-label=${text}
    onMouseEnter=${show}
    onFocus=${show}
    onMouseLeave=${() => setTip(null)}
    onBlur=${() => setTip(null)}
  >
    <span class="info-i">ⓘ</span>
    ${tip ? html`<${InfoTip} text=${text} tip=${tip} />` : ""}
  </span>`;
}

function InfoTip({ text, tip }) {
  const W = 260;
  const vw = typeof window !== "undefined" ? window.innerWidth : 1200;
  const left = Math.max(8, Math.min(tip.x - W / 2, vw - W - 8));
  const style = tip.below
    ? `left:${left}px;top:${tip.bottom + 8}px`
    : `left:${left}px;top:${tip.top - 8}px;transform:translateY(-100%)`;
  return html`<span class="info-tip" style=${style}>${text}</span>`;
}

// Gauge — a radial progress dial (0..100).
export function Gauge({ value, size = 132, label, color }) {
  const v = Math.max(0, Math.min(100, value || 0));
  const stroke = 12;
  const r = (size - stroke) / 2;
  const c = size / 2;
  const circ = 2 * Math.PI * r;
  const len = (v / 100) * circ;
  const col = color || (v >= 90 ? "var(--accent-2)" : v >= 70 ? "var(--warn)" : "var(--danger)");
  return html`<svg width=${size} height=${size} viewBox=${`0 0 ${size} ${size}`} style="display:block">
    <circle cx=${c} cy=${c} r=${r} fill="none" stroke="var(--panel-3)" stroke-width=${stroke} />
    <circle cx=${c} cy=${c} r=${r} fill="none" stroke=${col} stroke-width=${stroke} stroke-linecap="round"
      stroke-dasharray=${`${len} ${circ - len}`} transform=${`rotate(-90 ${c} ${c})`} />
    <text x=${c} y=${c - 1} text-anchor="middle" font-size="26" font-weight="700" fill="var(--text)">${v}%</text>
    ${label ? html`<text x=${c} y=${c + 18} text-anchor="middle" font-size="10" fill="var(--muted)">${label}</text>` : ""}
  </svg>`;
}

// Donut — proportional ring with a legend. data: [{name,count,color?}].
export function Donut({ data, size = 132, thickness = 20, centerLabel, centerSub }) {
  const total = data.reduce((s, d) => s + d.count, 0) || 1;
  const r = (size - thickness) / 2;
  const c = size / 2;
  const circ = 2 * Math.PI * r;
  let offset = 0;
  const segs = data.map((d, i) => {
    const len = (d.count / total) * circ;
    const seg = html`<circle cx=${c} cy=${c} r=${r} fill="none"
      stroke=${d.color || PALETTE[i % PALETTE.length]} stroke-width=${thickness}
      stroke-dasharray=${`${len} ${circ - len}`} stroke-dashoffset=${-offset}
      transform=${`rotate(-90 ${c} ${c})`}><title>${d.name}: ${d.count}</title></circle>`;
    offset += len;
    return seg;
  });
  return html`<div class="row" style="gap:var(--sp-4);align-items:center;flex-wrap:wrap">
    <svg width=${size} height=${size} viewBox=${`0 0 ${size} ${size}`} style="display:block;flex:0 0 auto">
      <circle cx=${c} cy=${c} r=${r} fill="none" stroke="var(--panel-3)" stroke-width=${thickness} />
      ${segs}
      ${centerLabel != null ? html`<text x=${c} y=${c - 1} text-anchor="middle" font-size="22" font-weight="700" fill="var(--text)">${centerLabel}</text>` : ""}
      ${centerSub ? html`<text x=${c} y=${c + 16} text-anchor="middle" font-size="10" fill="var(--muted)">${centerSub}</text>` : ""}
    </svg>
    <div class="stack" style="gap:5px;min-width:120px">
      ${data.map(
        (d, i) => html`<div class="row" style="gap:6px;font-size:var(--fs-sm)">
          <span style=${`width:10px;height:10px;border-radius:2px;flex:0 0 auto;background:${d.color || PALETTE[i % PALETTE.length]}`}></span>
          <span class="muted" style="overflow:hidden;text-overflow:ellipsis">${d.name}</span>
          <span style="margin-left:auto;font-variant-numeric:tabular-nums">${d.count}</span>
        </div>`,
      )}
    </div>
  </div>`;
}

// TraceDiagram — a left-to-right layered call graph. nodes:[{id,label,
// depth,kind}], edges:[{source,target}]. Each depth is a column; edges
// are drawn as curved connectors. Scrollable for larger subtrees.
export function TraceDiagram({ trace }) {
  const [zoom, setZoom] = useState(1);
  const [hover, setHover] = useState(null); // {node,x,y}
  const [selected, setSelected] = useState(null); // pinned node (copyable)
  if (!trace || !trace.nodes || trace.nodes.length === 0) return "";
  const z = (f) => setZoom((Z) => Math.max(0.4, Math.min(2.5, +(Z * f).toFixed(2))));
  const NW = 158,
    NH = 30,
    GX = 64,
    GY = 14;

  const layers = {};
  trace.nodes.forEach((n) => (layers[n.depth] = layers[n.depth] || []).push(n));
  const depths = Object.keys(layers).map(Number).sort((a, b) => a - b);

  let maxRows = 0;
  depths.forEach((d) => (maxRows = Math.max(maxRows, layers[d].length)));
  const height = Math.max(NH, maxRows * (NH + GY) - GY);
  const width = depths.length * NW + (depths.length - 1) * GX;

  const pos = {};
  depths.forEach((d, di) => {
    const cnt = layers[d].length;
    const colH = cnt * (NH + GY) - GY;
    const off = (height - colH) / 2;
    layers[d].forEach((n, i) => {
      pos[n.id] = { x: di * (NW + GX), y: off + i * (NH + GY) };
    });
  });

  // Focus node: hovered (cursor) or pinned. When set, only its edges and
  // direct neighbours stay bright — everything else dims, so the
  // relationships of the focused node are unmistakable.
  const focusId = (hover && hover.node.id) || (selected && selected.id) || null;
  const neighbors = new Set();
  if (focusId) {
    trace.edges.forEach((e) => {
      if (e.source === focusId) neighbors.add(e.target);
      if (e.target === focusId) neighbors.add(e.source);
    });
  }
  const edgeActive = (e) => focusId && (e.source === focusId || e.target === focusId);
  const nodeDim = (id) => focusId && id !== focusId && !neighbors.has(id);

  const edgePaths = trace.edges
    .filter((e) => pos[e.source] && pos[e.target])
    .map((e) => {
      const s = pos[e.source],
        t = pos[e.target];
      const x1 = s.x + NW,
        y1 = s.y + NH / 2,
        x2 = t.x - 2,
        y2 = t.y + NH / 2;
      const mx = (x1 + x2) / 2;
      const hi = edgeActive(e);
      const dim = focusId && !hi;
      return html`<path d=${`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} fill="none"
        stroke=${hi ? "var(--accent)" : "var(--border-strong)"} stroke-width=${hi ? 2.2 : 1.3}
        opacity=${dim ? 0.12 : 1} marker-end=${hi ? "url(#tr-arrow-hi)" : "url(#tr-arrow)"} />`;
    });

  const nodeColor = (k) => (k === "handler" ? "var(--accent)" : k === "leaf" ? "var(--faint)" : "var(--accent-2)");
  const nodeFill = (k) => (k === "handler" ? "var(--accent-weak)" : "var(--panel-2)");
  const nodes = trace.nodes.map((n) => {
    const p = pos[n.id];
    const isSel = selected && selected.id === n.id;
    const isFocus = focusId === n.id;
    return html`<g transform=${`translate(${p.x},${p.y})`} style="cursor:pointer" opacity=${nodeDim(n.id) ? 0.28 : 1}
      onMouseMove=${(e) => setHover({ node: n, x: e.clientX, y: e.clientY })}
      onMouseLeave=${() => setHover(null)}
      onClick=${() => setSelected(n)}>
      <rect width=${NW} height=${NH} rx="6" fill=${nodeFill(n.kind)}
        stroke=${isSel ? "var(--text)" : isFocus ? "var(--accent)" : nodeColor(n.kind)}
        stroke-width=${isSel || isFocus ? 2.5 : n.kind === "handler" ? 2 : 1} />
      <circle cx="11" cy=${NH / 2} r="3" fill=${nodeColor(n.kind)} />
      <text x="22" y=${NH / 2 + 4} font-size="11" fill="var(--text)" style="font-family:var(--font-mono)">${trunc(n.label, 19)}</text>
    </g>`;
  });

  const arrow = (id, color) =>
    html`<marker id=${id} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
      <path d="M0,0 L10,5 L0,10 z" fill=${color} />
    </marker>`;

  return html`<div style="position:relative">
    <div class="trace-zoom">
      <button title="Zoom out" onClick=${() => z(0.8)}>−</button>
      <span>${Math.round(zoom * 100)}%</span>
      <button title="Zoom in" onClick=${() => z(1.25)}>+</button>
      <button title="Reset" onClick=${() => setZoom(1)}>⟲</button>
    </div>
    <div style="overflow:auto;max-height:60vh;border:1px solid var(--border);border-radius:var(--r-2);background:var(--bg)">
      <svg width=${(width + 16) * zoom} height=${(height + 16) * zoom} viewBox=${`-8 -8 ${width + 16} ${height + 16}`} style="display:block">
        <defs>${arrow("tr-arrow", "var(--border-strong)")}${arrow("tr-arrow-hi", "var(--accent)")}</defs>
        ${edgePaths}${nodes}
      </svg>
    </div>
    <p class="muted" style="font-size:var(--fs-xs);margin:6px 0 0">Arrows show call direction · hover a node to highlight its calls · click to pin details.</p>
    ${selected ? html`<${NodeDetails} node=${selected} onClose=${() => setSelected(null)} />` : ""}
    ${hover && (!selected || selected.id !== hover.node.id) ? html`<${TraceTip} node=${hover.node} x=${hover.x} y=${hover.y} />` : ""}
  </div>`;
}

const ORIGIN = {
  project: ["project", "var(--accent-2)"],
  library: ["library", "var(--accent)"],
  standard: ["standard", "var(--muted)"],
};

function TraceTip({ node, x, y }) {
  const o = ORIGIN[node.origin] || ["", "var(--muted)"];
  const vw = typeof window !== "undefined" ? window.innerWidth : 1200;
  const vh = typeof window !== "undefined" ? window.innerHeight : 800;
  const W = 300,
    H = 150; // approximate tooltip box
  // keep on-screen: prefer cursor+offset, flip when near the right/bottom edge
  const left = Math.max(8, Math.min(x + 14, vw - W - 8));
  const top = y + 14 + H > vh ? Math.max(8, y - H - 8) : y + 14;
  return html`<div class="trace-tip" style=${`left:${left}px;top:${top}px`}>
    <div class="tt-title">${node.symbol || node.label}</div>
    ${node.pkg ? html`<div class="tt-pkg">${node.pkg}</div>` : ""}
    <div class="tt-badges">
      <span class="tt-badge" style=${`color:${o[1]};border-color:${o[1]}`}>${o[0] || node.origin || "?"}</span>
      <span class="tt-badge">${node.kind}</span>
      <span class="tt-badge">depth ${node.depth}</span>
    </div>
    <div class="tt-meta">calls ${node.calls} · called by ${node.calledBy}</div>
    ${node.pos ? html`<div class="tt-meta tt-pos">${node.pos}</div>` : ""}
    <div class="tt-hint">click to pin &amp; copy</div>
  </div>`;
}

// NodeDetails is the pinned, interactive (selectable, copyable) panel
// shown below the diagram — always on-screen, unlike the hover tooltip.
function NodeDetails({ node, onClose }) {
  const [copied, setCopied] = useState("");
  const allText = [
    "symbol: " + (node.symbol || node.label),
    "id: " + node.id,
    node.pkg ? "package: " + node.pkg : "",
    `origin: ${node.origin} · kind: ${node.kind} · depth: ${node.depth}`,
    `calls: ${node.calls} out · ${node.calledBy} in`,
    node.pos ? "position: " + node.pos : "",
  ]
    .filter(Boolean)
    .join("\n");
  const copy = async (val, tag) => {
    try {
      await navigator.clipboard.writeText(val);
      setCopied(tag);
      setTimeout(() => setCopied(""), 1200);
    } catch {
      /* ignore */
    }
  };
  const row = (k, v, mono) =>
    v ? html`<div class="nd-k">${k}</div><div class=${"nd-v" + (mono ? " mono" : "")}>${v}</div>` : "";
  return html`<div class="node-details">
    <div class="row">
      <strong>Pinned node</strong>
      <span class="spacer"></span>
      <button class="btn ghost sm" onClick=${() => copy(node.id, "id")}>${copied === "id" ? "✓" : "Copy ID"}</button>
      <button class="btn ghost sm" onClick=${() => copy(allText, "all")}>${copied === "all" ? "✓ Copied" : "Copy all"}</button>
      <button class="btn ghost sm" title="Unpin" onClick=${onClose}>✕</button>
    </div>
    <div class="nd-grid">
      ${row("symbol", node.symbol || node.label, true)}
      ${row("id", node.id, true)}
      ${row("package", node.pkg, true)}
      ${row("origin", `${node.origin} · ${node.kind} · depth ${node.depth}`)}
      ${row("calls", `${node.calls} out · ${node.calledBy} in`)}
      ${row("position", node.pos, true)}
    </div>
  </div>`;
}
