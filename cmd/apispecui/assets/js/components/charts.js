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
  const focusId = (hover && hover.node.id) || (selected && selected.node.id) || null;
  const neighbors = new Set();
  if (focusId) {
    trace.edges.forEach((e) => {
      if (e.source === focusId) neighbors.add(e.target);
      if (e.target === focusId) neighbors.add(e.source);
    });
  }
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
      // Relative to the focused node: outgoing = it calls the target (accent),
      // incoming = it is called by the source (warn). Distinct colours make
      // the direction of every relationship readable at a glance.
      const out = focusId === e.source;
      const inc = focusId === e.target;
      const hi = out || inc;
      const dim = focusId && !hi;
      const col = out ? "var(--accent)" : inc ? "var(--warn)" : "var(--border-strong)";
      const marker = out ? "url(#tr-arrow-out)" : inc ? "url(#tr-arrow-in)" : "url(#tr-arrow)";
      return html`<path d=${`M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`} fill="none"
        stroke=${col} stroke-width=${hi ? 2.2 : 1.3}
        opacity=${dim ? 0.12 : 1} marker-end=${marker} />`;
    });

  const nodeColor = (k) => (k === "handler" ? "var(--accent)" : k === "leaf" ? "var(--faint)" : "var(--accent-2)");
  const nodeFill = (k) => (k === "handler" ? "var(--accent-weak)" : "var(--panel-2)");
  const nodes = trace.nodes.map((n) => {
    const p = pos[n.id];
    const isSel = selected && selected.node.id === n.id;
    const isFocus = focusId === n.id;
    return html`<g transform=${`translate(${p.x},${p.y})`} style="cursor:pointer" opacity=${nodeDim(n.id) ? 0.28 : 1}
      onMouseMove=${(e) => setHover({ node: n, x: e.clientX, y: e.clientY })}
      onMouseLeave=${() => setHover(null)}
      onClick=${(e) => setSelected({ node: n, x: e.clientX, y: e.clientY })}>
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
        <defs>${arrow("tr-arrow", "var(--border-strong)")}${arrow("tr-arrow-out", "var(--accent)")}${arrow("tr-arrow-in", "var(--warn)")}</defs>
        ${edgePaths}${nodes}
      </svg>
    </div>
    <p class="muted" style="font-size:var(--fs-xs);margin:6px 0 0">
      Focus a node to highlight its edges: <span style="color:var(--accent)">▸ calls</span> · <span style="color:var(--warn)">◂ called by</span> · hover to preview, click to pin (the pinned card is hoverable &amp; copyable).
    </p>
    ${selected ? html`<${TraceTip} key=${selected.node.id} node=${selected.node} x=${selected.x} y=${selected.y} pinned onClose=${() => setSelected(null)} />` : ""}
    ${hover && (!selected || selected.node.id !== hover.node.id) ? html`<${TraceTip} node=${hover.node} x=${hover.x} y=${hover.y} />` : ""}
  </div>`;
}

const ORIGIN = {
  project: ["project", "var(--accent-2)"],
  library: ["library", "var(--accent)"],
  standard: ["standard", "var(--muted)"],
};

// TraceTip is the node card. As a hover preview it follows the cursor and is
// click-through; when `pinned` (after a click) it freezes at the click point,
// becomes mouse-interactive (selectable + copy buttons) and stays until
// unpinned — so the user can move onto it, unlike the floating preview.
function TraceTip({ node, x, y, pinned, onClose }) {
  const [copied, setCopied] = useState("");
  const [drag, setDrag] = useState(null); // {left,top} once the user has moved it
  const [src, setSrc] = useState(null); // null | "loading" | {error} | {code,startLine,line,file}
  const dragRef = useRef(null);
  const o = ORIGIN[node.origin] || ["", "var(--muted)"];
  const vw = typeof window !== "undefined" ? window.innerWidth : 1200;
  const vh = typeof window !== "undefined" ? window.innerHeight : 800;
  const hasSrc = !!(src && src.code);
  // The card widens and grows taller once source is shown.
  const W = pinned && hasSrc ? 560 : 300;
  const H = pinned ? (hasSrc ? 440 : 210) : 150;
  // keep on-screen: prefer cursor+offset, flip when near the right/bottom edge
  const baseLeft = Math.max(8, Math.min(x + 14, vw - W - 8));
  const baseTop = y + 14 + H > vh ? Math.max(8, y - H - 8) : y + 14;
  const left = drag ? drag.left : baseLeft;
  const top = drag ? drag.top : baseTop;

  // Fetch the source window around this node's file:line. Best-effort —
  // failures (no pos, file outside the module, etc.) surface as a small note.
  const loadSrc = async () => {
    if (!node.pos || src === "loading") return;
    setSrc("loading");
    try {
      const r = await fetch(`/api/insight/source?pos=${encodeURIComponent(node.pos)}&before=3&after=28`);
      if (!r.ok) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || `HTTP ${r.status}`);
      }
      setSrc(await r.json());
    } catch (e) {
      setSrc({ error: e.message || "could not load source" });
    }
  };

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

  // Drag the pinned card by its header. Window-level listeners track the
  // pointer until release; clamped so the card can't be dragged off-screen.
  const startDrag = (e) => {
    e.preventDefault();
    dragRef.current = { sx: e.clientX, sy: e.clientY, l: left, t: top };
    const move = (ev) => {
      const b = dragRef.current;
      if (!b) return;
      setDrag({
        left: Math.max(0, Math.min(b.l + (ev.clientX - b.sx), vw - 40)),
        top: Math.max(0, Math.min(b.t + (ev.clientY - b.sy), vh - 24)),
      });
    };
    const up = () => {
      dragRef.current = null;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  return html`<div class=${"trace-tip" + (pinned ? " pinned" : "") + (hasSrc ? " has-src" : "")} style=${`left:${left}px;top:${top}px`}>
    ${pinned
      ? html`<div class="tt-drag" onPointerDown=${startDrag}>
          <span class="tt-grip">⠿</span><span>drag to move</span>
          <span class="spacer"></span>
          <button class="tt-x" title="Unpin" onPointerDown=${(e) => e.stopPropagation()} onClick=${onClose}>✕</button>
        </div>`
      : ""}
    <div class="tt-title">${node.symbol || node.label}</div>
    ${node.pkg ? html`<div class="tt-pkg">${node.pkg}</div>` : ""}
    <div class="tt-badges">
      <span class="tt-badge" style=${`color:${o[1]};border-color:${o[1]}`}>${o[0] || node.origin || "?"}</span>
      <span class="tt-badge">${node.kind}</span>
      <span class="tt-badge">depth ${node.depth}</span>
    </div>
    <div class="tt-meta">
      <span style="color:var(--accent)">▸ ${node.calls} calls</span> · <span style="color:var(--warn)">◂ called by ${node.calledBy}</span>
    </div>
    ${node.pos ? html`<div class="tt-meta tt-pos">${node.pos}</div>` : ""}
    ${pinned
      ? html`<div class="tt-actions">
          <button class="btn ghost sm" onClick=${() => copy(node.id, "id")}>${copied === "id" ? "✓" : "Copy ID"}</button>
          <button class="btn ghost sm" onClick=${() => copy(allText, "all")}>${copied === "all" ? "✓ Copied" : "Copy all"}</button>
          ${node.pos
            ? html`<button
                class="btn ghost sm"
                title="Show the source around this function/call"
                onClick=${() => (hasSrc || src === "loading" ? setSrc(null) : loadSrc())}
              >
                ${src === "loading" ? "loading…" : hasSrc ? "◇ Hide source" : "◇ View source"}
              </button>`
            : ""}
        </div>`
      : html`<div class="tt-hint">click to pin &amp; copy</div>`}
    ${hasSrc ? html`<${SourceView} src=${src} />` : ""}
    ${pinned && src && src.error ? html`<div class="tt-meta" style="color:var(--danger);margin-top:6px">${src.error}</div>` : ""}
  </div>`;
}

// SourceView renders a fetched code window with line numbers; the line that
// matches the node's position (the function/call site) is highlighted.
function SourceView({ src }) {
  const lines = (src.code || "").split("\n");
  const shortFile = (src.file || "").split("/").slice(-2).join("/");
  return html`<div class="tt-src">
    <div class="tt-src-code">
      ${lines.map((ln, i) => {
        const n = src.startLine + i;
        return html`<div class=${"tt-src-row" + (n === src.line ? " hit" : "")}>
          <span class="tt-src-n">${n}</span><span class="tt-src-c">${ln === "" ? " " : ln}</span>
        </div>`;
      })}
    </div>
    <div class="tt-src-foot">${shortFile}${src.line ? ":" + src.line : ""}</div>
  </div>`;
}
