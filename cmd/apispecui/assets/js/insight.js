// insight.js — Insight mode: whole-API Overview dashboard + per-endpoint
// view. Uses diverse SVG visualizations (gauge, donuts, layered call
// graph) and ⓘ info tooltips on every metric.
import { html, useState, useEffect } from "/assets/js/preact.js";
import { useStore, setState } from "/assets/js/store.js";
import { getJSON } from "/assets/js/api.js";
import { Donut, Gauge, Info, TraceDiagram } from "/assets/js/components/charts.js";

/* ---- shared bits ---------------------------------------------------- */

function normalizeReport(d) {
  d = d || {};
  for (const k of ["issues", "endpoints", "byMethod", "byStatus", "byContentType", "byTag", "topTypes"]) {
    if (!Array.isArray(d[k])) d[k] = [];
  }
  d.health = d.health || { score: 0, cleanRoutes: 0, totalRoutes: 0 };
  d.callGraph = d.callGraph || { packages: 0, functions: 0, edges: 0 };
  d.security = d.security || { schemesDefined: 0, schemes: [], protected: 0, public: 0, unsecured: 0, bySchemeUsage: [] };
  return d;
}

function Bars({ data, color }) {
  const max = Math.max(1, ...data.map((d) => d.count));
  return html`<div class="bars">
    ${data.map(
      (d) => html`<div class="bar-row" title=${`${d.name}: ${d.count}`}>
        <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${d.name}</span>
        <div class="bar-track"><div class="bar-fill" style=${`width:${(d.count / max) * 100}%${color ? ";background:" + color : ""}`}></div></div>
        <span class="muted" style="text-align:right">${d.count}</span>
      </div>`,
    )}
  </div>`;
}

const shortName = (n) => n.split("_").pop() || n;
const statusColor = (s) =>
  ({ "2": "var(--accent-2)", "3": "var(--info)", "4": "var(--warn)", "5": "var(--danger)" })[s[0]] || "var(--muted)";
const gradeColor = (g) =>
  ({ A: "var(--grade-a)", B: "var(--grade-b)", C: "var(--grade-c)", D: "var(--grade-d)" })[g] || "var(--muted)";
const KIND_LABEL = {
  "dangling-ref": "dangling $ref",
  "unresolved-type": "unresolved type",
  "no-responses": "no responses",
  "missing-body": "missing body",
  "wrapper-specialised": "wrapper specialised",
};

const INFO = {
  health:
    "Share of routes whose request/response schemas fully resolve — no dangling $refs, unresolved/placeholder types, or synthesized path params.",
  components: "Named schemas emitted under components/schemas in the generated spec.",
  routes: "Distinct path templates (e.g. /users/{id}).",
  operations: "Method + path combinations (one path can have GET, POST, …).",
  methods: "HTTP methods across all operations.",
  status: "Response status codes declared across the API.",
  ctype: "Request/response content types declared across the API.",
  tags: "OpenAPI tags grouping the routes.",
  toptypes: "Schemas referenced most often across request/response bodies.",
  security:
    "How authentication is applied across the API: how many operations require auth, are explicitly public, or have no security at all — plus the declared schemes and any middleware apispec couldn't map to a scheme.",
  secops:
    "Each operation by its effective security: protected (a requirement applies — its own or inherited from the document), public (an explicit security: [] opt-out), or no auth (no requirement at all).",
  secschemes: "Security schemes declared under components.securitySchemes, and how many operations require each.",
  callgraph: "Coarse size of the analysed call graph backing the spec.",
  fanout:
    "How many distinct functions each function directly calls, within this endpoint's call subtree (Go builtins like len/append and standard-library calls like fmt/net/http are excluded, so only your code + frameworks count). Average = total direct calls ÷ functions that make at least one call — leaf functions (0 calls) are left out of the divisor, so it reflects the branching factor among branching functions rather than being diluted toward 0 by leaves. Max = the single most-branching function. Example: handler calls 3, A calls 2, B calls 0 → avg (3+2)/2 = 2.5, max 3. Higher = more branching.",
  paths:
    "How many distinct routes a call can take from the handler down to a leaf function. Branches multiply: if the handler calls A and B, and each calls C and D, that's 4 paths. Expand “Show paths” below to see exactly where the number comes from. '+' = traversal limit hit.",
  depth: "Longest call-chain depth from the handler. '+' = traversal limit reached.",
  reachable: "Distinct functions reachable from the handler.",
  ptrval: "Arguments passed by pointer vs by value across the subtree (memory-safety vs copy cost).",
  chain: "Fluent method-chain depth at the handler (e.g. r.Group().Use()).",
  grade:
    "Heuristic complexity grade (A best … D worst) blending call-path fan-out, depth, mutations and unresolved types. A readability indicator — NOT a correctness or performance guarantee (built on AST, not SSA). The bars below are NOT a percent of a total: each gauges its metric against a fixed reference ceiling (a 'notably high' value, e.g. fan-out 8, depth 12, paths 200), so a full bar means 'at or above that ceiling'. Hover any bar to see its ceiling. The pointer:value bar is the exception — a true proportion.",
};

/* ---- root ----------------------------------------------------------- */

export function InsightMode() {
  const s = useStore();
  const [view, setView] = useState("overview");
  const [epFilter, setEpFilter] = useState("");
  const [rep, setRep] = useState(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!s.hasSpec) return;
    setLoading(true);
    setErr("");
    getJSON("/api/insight/overview")
      .then((d) => setRep(normalizeReport(d)))
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false));
  }, [s.hasSpec, s.lastGenTick]);

  if (!s.hasSpec) {
    return html`<div class="content pad"><div class="empty"><div class="empty-inner">
      <h2>◷ API Analysis & Insight</h2>
      <p>Generate a spec first — then this view analyzes how every endpoint resolved.</p>
    </div></div></div>`;
  }
  if (loading && !rep) return html`<div class="content pad muted">Analyzing…</div>`;
  if (err) return html`<div class="content pad" style="color:var(--danger)">${err}</div>`;
  if (!rep) return html`<div class="content pad muted">No report.</div>`;

  return html`
    <div class="content pad">
      <div style="width:100%">
        <div class="row" style="margin-bottom:var(--sp-3)">
          <h2>API insight</h2>
          <div class="seg" style="margin-left:var(--sp-2)">
            <button class=${view === "overview" ? "active" : ""} onClick=${() => setView("overview")}>Overview</button>
            <button class=${view === "endpoint" ? "active" : ""} onClick=${() => setView("endpoint")}>Endpoint</button>
          </div>
          <span class="spacer"></span>
          <span class="muted">${rep.summary || ""}</span>
        </div>
        ${view === "overview"
          ? html`<${Overview} rep=${rep} onTag=${(t) => { setEpFilter(t); setView("endpoint"); }} />`
          : html`<${EndpointView} rep=${rep} initialFilter=${epFilter} />`}
      </div>
    </div>
  `;
}

const Title = (text, info) =>
  html`<div class="row" style="gap:6px"><h3>${text}</h3>${info ? html`<${Info} text=${info} />` : ""}</div>`;

/* ---- overview ------------------------------------------------------- */

function Overview({ rep, onTag }) {
  const [exportOpen, setExportOpen] = useState(false);
  const warns = rep.issues.filter((i) => i.severity === "warn");
  const infos = rep.issues.filter((i) => i.severity !== "warn");
  const maxType = Math.max(1, ...rep.topTypes.map((t) => t.count));

  return html`
    <div class="row" style="margin-bottom:var(--sp-3)">
      <span class="spacer"></span>
      ${warns.length
        ? html`<button class="btn export" onClick=${() => setExportOpen(true)}>⤴ Export to AI</button>`
        : html`<button class="btn export" disabled title="No issues to fix — nothing to export">⤴ Export to AI</button>`}
    </div>

    <div class="grid-cards" style="grid-template-columns:repeat(auto-fit,minmax(200px,1fr));margin-bottom:var(--sp-3)">
      <div class="card" style="display:flex;align-items:center;gap:var(--sp-3)">
        <${Gauge} value=${rep.health.score} label="resolved" />
        <div>
          <div class="row" style="gap:6px"><strong>Resolution health</strong><${Info} text=${INFO.health} /></div>
          <div class="muted" style="font-size:var(--fs-sm);margin-top:4px">${rep.health.cleanRoutes}/${rep.health.totalRoutes} routes clean</div>
        </div>
      </div>
      <div class="stat"><div class="num">${rep.routes}</div><div class="lbl">routes <${Info} text=${INFO.routes} /></div></div>
      <div class="stat"><div class="num">${rep.operations}</div><div class="lbl">operations <${Info} text=${INFO.operations} /></div></div>
      <div class="stat"><div class="num">${rep.components}</div><div class="lbl">components <${Info} text=${INFO.components} /></div></div>
    </div>

    <div class="grid-cards" style="grid-template-columns:repeat(auto-fit,minmax(300px,1fr));margin-bottom:var(--sp-3)">
      ${rep.byMethod.length ? html`<div class="card">${Title("Methods", INFO.methods)}<${Bars} data=${rep.byMethod} /></div>` : ""}
      ${rep.byStatus.length ? html`<div class="card">${Title("Status codes", INFO.status)}<${Donut} data=${rep.byStatus.map((d) => ({ ...d, color: statusColor(d.name) }))} centerLabel=${rep.byStatus.reduce((a, b) => a + b.count, 0)} centerSub="responses" /></div>` : ""}
      ${rep.byContentType.length ? html`<div class="card">${Title("Content types", INFO.ctype)}<${Donut} data=${rep.byContentType} /></div>` : ""}
      ${rep.byTag.length
        ? html`<div class="card">
            ${Title("Tags", INFO.tags)}
            <p class="muted" style="font-size:var(--fs-xs);margin:0 0 var(--sp-2)">Click a tag to see its endpoints.</p>
            <div class="chips">
              ${rep.byTag.map(
                (t) => html`<button class="chip" onClick=${() => onTag(t.name)} title=${`${t.count} route(s) — click to filter`}>
                  ${t.name}<span class="chip-count">${t.count}</span>
                </button>`,
              )}
            </div>
          </div>`
        : ""}
    </div>

    <${SecurityCard} rep=${rep} />

    <div class="card" style="margin-bottom:var(--sp-3)">
      <div class="row">
        <h3>Needs attention</h3><span class="spacer"></span>
        ${warns.length ? html`<span class="badge err"><span class="dot"></span>${warns.length} to fix</span>` : html`<span class="badge ok"><span class="dot"></span>all clear</span>`}
        ${infos.length ? html`<span class="badge"><span class="dot"></span>${infos.length} info</span>` : ""}
      </div>
      ${warns.length === 0 && infos.length === 0
        ? html`<p class="muted">No issues detected — every reference resolves.</p>`
        : html`<div class="stack" style="margin-top:var(--sp-2)">
            ${[...warns, ...infos].slice(0, 40).map(
              (i) => html`<div class="row" style="gap:var(--sp-2);align-items:flex-start">
                <span class=${"badge " + (i.severity === "warn" ? "err" : "")} style="flex:0 0 auto">${KIND_LABEL[i.kind] || i.kind}</span>
                <span class="mono" style="flex:0 0 auto;font-size:var(--fs-sm)">${i.method} ${i.path}</span>
                <span class="muted" style="font-size:var(--fs-sm)">${i.detail}${i.ref ? ` (${shortName(i.ref)})` : ""}</span>
              </div>`,
            )}
          </div>`}
    </div>

    <div class="grid-cards" style="grid-template-columns:1fr;margin-bottom: var(--sp-3);">
      <div class="card">${Title("Most-referenced types", INFO.toptypes)}
        ${rep.topTypes.length
          ? html`<div class="ranklist">
              ${rep.topTypes.map(
                (t, i) => html`<div class=${"rank-row" + (i < 3 ? " top" : "")}>
                  <span class="rank-n">${i + 1}</span>
                  <div class="rank-body">
                    <div class="rank-top">
                      <span class="rank-name" title=${t.name.replace(/_/g, ".")}>${shortName(t.name)}</span>
                      <span class="rank-count">${t.count}×</span>
                    </div>
                    <div class="rank-bar"><span style=${`width:${(t.count / maxType) * 100}%`}></span></div>
                  </div>
                </div>`,
              )}
            </div>`
          : html`<span class="muted">none</span>`}
      </div>
    </div>

    <${CallGraphCard} cg=${rep.callGraph} />
    <${ExportModal} open=${exportOpen} onClose=${() => setExportOpen(false)} scope="all" />
  `;
}

// SecurityCard visualises how auth is applied across the API (protected /
// public / no-auth operations, declared schemes, per-scheme usage) and surfaces
// any middleware apispec detected but couldn't map to a scheme.
function SecurityCard({ rep }) {
  const s = useStore();
  const sec = rep.security || {};
  const unresolved = (s.unresolvedSecurity || []).length;
  const total = (sec.protected || 0) + (sec.public || 0) + (sec.unsecured || 0);
  const donut = [
    { name: "protected", count: sec.protected || 0, color: "var(--accent-2)" },
    { name: "public", count: sec.public || 0, color: "var(--info)" },
    { name: "no auth", count: sec.unsecured || 0, color: "var(--warn)" },
  ].filter((d) => d.count > 0);
  const usage = (sec.bySchemeUsage || []).map((d) => ({ name: d.name, count: d.count }));
  const noAuth = !(sec.schemesDefined || sec.protected || unresolved);

  return html`
    <div class="card" style="margin-bottom:var(--sp-3)">
      ${Title("Security", INFO.security)}
      ${noAuth
        ? html`<p class="muted">No authentication detected — no security schemes, and every operation is open. If routes are auth-guarded by a custom middleware, map it under Configure ▸ Security mappings.</p>`
        : html`<div class="grid-cards" style="grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:var(--sp-4)">
            <div>
              <div class="shape-h">Operations <${Info} text=${INFO.secops} /></div>
              ${donut.length
                ? html`<${Donut} data=${donut} size=${120} thickness=${18} centerLabel=${total} centerSub="ops" />`
                : html`<span class="muted">none</span>`}
              <div class="row wrap" style="gap:var(--sp-3);margin-top:var(--sp-2);font-size:var(--fs-sm)">
                <span><span class="dot" style="background:var(--accent-2)"></span> ${sec.protected || 0} protected</span>
                <span><span class="dot" style="background:var(--info)"></span> ${sec.public || 0} public</span>
                <span><span class="dot" style="background:var(--warn)"></span> ${sec.unsecured || 0} no auth</span>
              </div>
            </div>
            <div>
              <div class="shape-h">Schemes <${Info} text=${INFO.secschemes} /></div>
              ${(sec.schemes || []).length
                ? html`<div class="chips">${sec.schemes.map((n) => html`<span class="chip">${n}</span>`)}</div>`
                : html`<span class="muted">none defined</span>`}
              ${usage.length ? html`<div style="margin-top:var(--sp-2)"><${Bars} data=${usage} color="var(--accent-2)" /></div>` : ""}
              ${sec.globalSecurity
                ? html`<p class="muted" style="font-size:var(--fs-xs);margin-top:var(--sp-2)">A document-level security requirement applies by default.</p>`
                : ""}
            </div>
          </div>`}
      ${unresolved
        ? html`<div class="row" style="margin-top:var(--sp-3);gap:var(--sp-2);align-items:center;flex-wrap:wrap">
            <span class="badge err"><span class="dot"></span>${unresolved} unresolved</span>
            <span class="muted" style="font-size:var(--fs-sm)">middleware detected on routes but not mapped to a scheme</span>
            <span class="spacer"></span>
            <button class="btn sm" onClick=${() => setState({ mode: "configure" })}>Map them →</button>
          </div>`
        : ""}
    </div>
  `;
}

const KIND_COLOR = { project: "var(--accent-2)", library: "var(--accent)", standard: "var(--muted)" };

function CallGraphCard({ cg }) {
  cg = cg || {};
  const comp = (cg.edgeKinds || []).map((k) => ({ name: k.name, count: k.count, color: KIND_COLOR[k.name] }));
  const hot = cg.hotFunctions || [];
  const busy = cg.busyPackages || [];
  const maxHot = Math.max(1, ...hot.map((h) => h.count));

  return html`
    <div class="card">
      ${Title("Call graph", INFO.callgraph)}
      <div class="row wrap" style="gap:var(--sp-5);margin:var(--sp-1) 0 var(--sp-3)">
        <div><span style="font-size:var(--fs-xl);font-weight:700">${cg.packages || 0}</span> <span class="muted">packages</span></div>
        <div><span style="font-size:var(--fs-xl);font-weight:700">${cg.functions || 0}</span> <span class="muted">functions</span></div>
        <div><span style="font-size:var(--fs-xl);font-weight:700">${cg.edges || 0}</span> <span class="muted">call edges</span></div>
      </div>
      <div class="grid-cards" style="grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:var(--sp-4)">
        <div>
          <div class="shape-h">Edge composition <${Info} text="Where calls go: your project code, third-party libraries/frameworks, or the Go standard library + builtins." /></div>
          ${comp.length
            ? html`<${Donut} data=${comp} size=${120} thickness=${18} centerLabel=${cg.edges || 0} centerSub="edges" />`
            : html`<span class="muted">no edges</span>`}
        </div>
        <div>
          <div class="shape-h">Hot functions <${Info} text="Your project's most-called functions (fan-in) — the shared hubs. High fan-in means many call sites depend on it." /></div>
          ${hot.length
            ? html`<div class="ranklist">
                ${hot.map(
                  (h, i) => html`<div class=${"rank-row" + (i < 3 ? " top" : "")}>
                    <span class="rank-n">${i + 1}</span>
                    <div class="rank-body">
                      <div class="rank-top"><span class="rank-name" title=${h.name}>${h.name}</span><span class="rank-count">${h.count}×</span></div>
                      <div class="rank-bar"><span style=${`width:${(h.count / maxHot) * 100}%;background:var(--accent-2)`}></span></div>
                    </div>
                  </div>`,
                )}
              </div>`
            : html`<span class="muted">no project calls traced</span>`}
        </div>
        <div>
          <div class="shape-h">Busiest packages <${Info} text="Your project packages with the most functions/methods — where the code concentrates." /></div>
          ${busy.length ? html`<${Bars} data=${busy} color="var(--info)" />` : html`<span class="muted">none</span>`}
        </div>
      </div>
    </div>
  `;
}

/* ---- endpoint ------------------------------------------------------- */

function EndpointView({ rep, initialFilter }) {
  const [sel, setSel] = useState(rep.endpoints[0] ? rep.endpoints[0].method + " " + rep.endpoints[0].path : "");
  const [ep, setEp] = useState(null);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState(initialFilter || "");
  const [exportOpen, setExportOpen] = useState(false);
  const [traceSrc, setTraceSrc] = useState("tracker"); // "tracker" | "callgraph"

  // Sync the filter when the user clicks a tag from the Overview.
  useEffect(() => {
    if (initialFilter) setFilter(initialFilter);
  }, [initialFilter]);

  const parse = (v) => {
    const i = v.indexOf(" ");
    return { method: v.slice(0, i), path: v.slice(i + 1) };
  };

  useEffect(() => {
    if (!sel) return;
    const { method, path } = parse(sel);
    setLoading(true);
    getJSON(`/api/insight/endpoint?method=${encodeURIComponent(method)}&path=${encodeURIComponent(path)}&trace=${traceSrc}`)
      .then(setEp)
      .finally(() => setLoading(false));
  }, [sel, traceSrc]);

  const list = rep.endpoints.filter((e) => {
    if (!filter) return true;
    const hay = (e.method + " " + e.path + " " + (e.tags || []).join(" ")).toLowerCase();
    return hay.includes(filter.toLowerCase());
  });

  return html`
    <div class="split-2">
      <div class="card" style="padding:0;max-height:72vh;display:flex;flex-direction:column">
        <div class="pad" style="border-bottom:1px solid var(--border)">
          <input class="input" placeholder="filter routes…" value=${filter} onInput=${(e) => setFilter(e.target.value)} />
        </div>
        <div style="overflow:auto">
          ${list.map((e) => {
            const v = e.method + " " + e.path;
            return html`<div class=${"row-item" + (v === sel ? " has-gomod" : "")} style=${v === sel ? "background:var(--panel-2)" : ""} onClick=${() => setSel(v)}>
              <span class="badge" style="flex:0 0 auto">${e.method}</span>
              <span class="mono" style="font-size:var(--fs-sm);overflow:hidden;text-overflow:ellipsis">${e.path}</span>
            </div>`;
          })}
        </div>
      </div>
      <div>
        <div class="row" style="margin-bottom:var(--sp-2);align-items:center;gap:var(--sp-2);flex-wrap:wrap">
          <span class="muted" style="font-size:var(--fs-sm)">Trace from</span>
          <div class="seg">
            <button class=${traceSrc === "tracker" ? "active" : ""} title="Call graph ∪ interface/generic resolution — the superset apispec uses to build the spec" onClick=${() => setTraceSrc("tracker")}>Tracker tree</button>
            <button class=${traceSrc === "callgraph" ? "active" : ""} title="Raw call graph only — syntactic calls, no resolution" onClick=${() => setTraceSrc("callgraph")}>Call graph</button>
          </div>
        </div>
        ${loading && !ep ? html`<div class="muted">loading…</div>` : ""}
        ${ep && ep.found ? html`<${EndpointDetail} ep=${ep} onExport=${() => setExportOpen(true)} />` : ep ? html`<div class="muted">No operation found.</div>` : ""}
      </div>
    </div>
    ${ep && ep.found ? html`<${ExportModal} open=${exportOpen} onClose=${() => setExportOpen(false)} scope="endpoint" method=${ep.method} path=${ep.path} trace=${traceSrc} />` : ""}
  `;
}

// Metric — a labelled value with an optional meter bar. The bar is a visual
// gauge, not a percent of a total: pass `cap` (the reference ceiling that fills
// the bar) and the hover tooltip explains it, or pass `barTip` to override the
// wording (e.g. for a true proportion like pointer:value).
const Metric = ({ label, info, value, frac, color, cap, barTip }) => {
  const tip =
    barTip ||
    (cap != null
      ? `${Math.round(Math.min(1, frac) * 100)}% of the reference ceiling (${cap}). The bar fills as the value approaches that ceiling — it's a gauge, not a percent of a total.`
      : `${Math.round(Math.min(1, frac) * 100)}%`);
  return html`
    <div class="metric">
      <span class="m-label">${label}${info ? html`<${Info} text=${info} />` : ""}</span>
      <span class="m-value">${value}</span>
      ${frac != null
        ? html`<div class="m-meter" title=${tip}><span style=${`width:${Math.min(100, frac * 100)}%${color ? ";background:" + color : ""}`}></span></div>`
        : ""}
    </div>
  `;
};

// PathsBreakdown — an expandable list of the actual handler→leaf call
// paths, so the call-paths count is inspectable ("where does 12 come
// from?"). Shows the enumerated sample and notes any remainder.
function PathsBreakdown({ trace, count, truncated }) {
  const [open, setOpen] = useState(false);
  const paths = (trace && trace.paths) || [];
  if (!paths.length || count <= 1) return "";
  return html`<div style="margin-top:var(--sp-2)">
    <button class="btn ghost sm" onClick=${() => setOpen((o) => !o)}>
      ${open ? "▾" : "▸"} ${open ? "Hide" : "Show"} the ${count}${truncated ? "+" : ""} call-path${count === 1 ? "" : "s"}
    </button>
    ${open
      ? html`<div class="paths-list">
          ${paths.map(
            (p, i) => html`<div class="path-row">
              <span class="path-n">${i + 1}</span>
              <span class="path-seq">${p.join(" → ")}</span>
            </div>`,
          )}
          ${count > paths.length
            ? html`<div class="muted" style="font-size:var(--fs-xs);padding:4px 8px">…and ${count - paths.length}${truncated ? "+" : ""} more</div>`
            : ""}
        </div>`
      : ""}
  </div>`;
}

function EndpointDetail({ ep, onExport }) {
  const m = ep.metrics;
  const warns = (ep.issues || []).filter((i) => i.severity === "warn");
  const maxOf = (val, trunc) => (trunc ? val + "+" : "" + val);
  const ptrTotal = (m.pointerArgs || 0) + (m.valueArgs || 0);

  return html`
    <div class="card">
      <div class="row">
        <span class="badge">${ep.method}</span>
        <span class="mono">${ep.path}</span>
        <span class="spacer"></span>
        <button
          class="btn export sm"
          disabled=${!warns.length}
          title=${warns.length ? "Export this endpoint's issue(s), trace & source for an AI assistant" : "No issues on this endpoint — nothing to export"}
          onClick=${onExport}
        >
          ⤴ to AI
        </button>
      </div>
      <div class="muted" style="font-size:var(--fs-sm);margin-top:4px">${ep.handler}${ep.handlerPos ? " · " + ep.handlerPos : ""}</div>
    </div>

    <div class="card">
      <h3>Shape</h3>

      <div class="shape-block">
        <div class="shape-h">Request</div>
        ${ep.request
          ? html`<div class="shape-row">
              <span class="badge">${ep.request.contentType}</span>
              <span class="mono" style="font-size:var(--fs-sm)">${ep.request.schema || "—"}</span>
              ${ep.request.required ? html`<span class="badge warn">required</span>` : html`<span class="muted" style="font-size:var(--fs-xs)">optional</span>`}
            </div>`
          : html`<span class="muted">none</span>`}
      </div>

      <div class="shape-block">
        <div class="shape-h">Responses</div>
        <table class="shape-tbl"><tbody>
          ${(ep.responses || []).map(
            (r) => html`<tr>
              <td class="shrink"><span class="badge" style=${"border-color:" + statusColor(r.status) + ";color:" + statusColor(r.status)}>${r.status}</span></td>
              <td class="shrink"><span class="kv-k">${r.contentType || "—"}</span></td>
              <td class="mono">${r.schema || "—"}</td>
            </tr>`,
          )}
        </tbody></table>
      </div>

      ${(ep.params || []).length
        ? html`<div class="shape-block">
            <div class="shape-h">Parameters</div>
            <table class="shape-tbl"><tbody>
              ${ep.params.map(
                (p) => html`<tr>
                  <td class="shrink mono">${p.name}</td>
                  <td class="shrink"><span class="badge">${p.in}</span></td>
                  <td class="mono">${p.type || "—"}</td>
                  <td class="shrink">${p.required ? html`<span class="badge warn">required</span>` : html`<span class="muted" style="font-size:var(--fs-xs)">optional</span>`}</td>
                </tr>`,
              )}
            </tbody></table>
          </div>`
        : ""}
    </div>

    ${warns.length ? html`<div class="card"><h3>Issues</h3>${warns.map((i) => html`<div class="row"><span class="badge err">${KIND_LABEL[i.kind] || i.kind}</span><span class="muted" style="font-size:var(--fs-sm)">${i.detail}${i.ref ? ` (${shortName(i.ref)})` : ""}</span></div>`)}</div>` : ""}

    ${ep.handlerFound
      ? html`
          <div class="card">
            <div class="row" style="align-items:center;gap:var(--sp-3)">
              <div class="grade-ring" style=${`border-color:${gradeColor(m.grade)};color:${gradeColor(m.grade)}`}>${m.grade || "—"}</div>
              <div>
                <div class="row" style="gap:6px"><strong>Complexity grade</strong><${Info} text=${INFO.grade} /></div>
                <div class="muted" style="font-size:var(--fs-sm)">${m.gradeLowerBound ? "lower bound (traversal limited)" : "heuristic readability indicator"}</div>
              </div>
            </div>
            <div style="margin-top:var(--sp-3)">
              ${Metric({ label: "call fan-out", info: INFO.fanout, value: `${m.fanoutAvg.toFixed(1)} avg / ${m.fanoutMax} max`, frac: Math.min(1, m.fanoutMax / 8), cap: "8 max direct calls" })}
              ${Metric({ label: "call-paths", info: INFO.paths, value: maxOf(m.callPaths, m.callPathsTruncated), frac: Math.min(1, m.callPaths / 200), color: "var(--info)", cap: "200 paths" })}
              ${Metric({ label: "max depth", info: INFO.depth, value: maxOf(m.maxDepth, m.depthTruncated), frac: Math.min(1, m.maxDepth / 12), color: "var(--warn)", cap: "12 levels deep" })}
              ${Metric({ label: "reachable fns", info: INFO.reachable, value: maxOf(m.reachable, m.depthTruncated), frac: Math.min(1, m.reachable / 100), cap: "100 functions" })}
              ${Metric({ label: "pointer : value args", info: INFO.ptrval, value: `${m.pointerArgs} : ${m.valueArgs}`, frac: ptrTotal ? m.pointerArgs / ptrTotal : 0, color: "var(--accent-2)", barTip: `True proportion: ${m.pointerArgs} of ${ptrTotal || 0} args (${Math.round((ptrTotal ? m.pointerArgs / ptrTotal : 0) * 100)}%) are passed by pointer. Unlike the other bars, this one IS a real percentage.` })}
              ${Metric({ label: "chain depth", info: INFO.chain, value: "" + m.chainDepth, frac: Math.min(1, m.chainDepth / 6), cap: "6 chained calls" })}
            </div>
            <p class="muted" style="font-size:var(--fs-xs);margin:var(--sp-2) 0 0">
              Bars gauge each metric against a reference ceiling (a notably-high value) — a full bar means at or above it, not a percent of a total. Hover a bar for its ceiling.
            </p>
            <${PathsBreakdown} trace=${ep.trace} count=${m.callPaths} truncated=${m.callPathsTruncated} />
          </div>
          <div class="card">
            <div class="row" style="gap:6px">
              <h3>Resolution trace</h3>
              <${Info} text="The call subtree from this handler — a left-to-right layered graph. ● handler, ○ callee, ◌ leaf. Source is selectable above: the tracker tree resolves interface/generic calls (what apispec used to build the spec); the call graph is raw syntactic calls." />
              <span class="spacer"></span>
              ${ep.traceSource
                ? html`<span class="sec-count" title=${ep.traceSource === "tracker" ? "Interface-resolved tracker tree" : "Raw call graph (syntactic calls only)"}>${ep.traceSource === "tracker" ? "tracker tree" : "call graph"}</span>`
                : ""}
            </div>
            <p class="muted" style="font-size:var(--fs-sm);margin:0 0 var(--sp-2)">${ep.trace.nodes.length} nodes · ${ep.trace.edges.length} edges${ep.trace.truncated ? " (truncated)" : ""}</p>
            <${TraceDiagram} trace=${ep.trace} />
          </div>
        `
      : html`<div class="card"><h3>Trace & metrics</h3><p class="muted">The handler couldn't be located in the call graph for this route, so the scoped trace and metrics are unavailable. Request/response/params above are still accurate.</p></div>`}
  `;
}

/* ---- export modal --------------------------------------------------- */

function ExportModal({ open, onClose, scope, method, path, trace }) {
  const [md, setMd] = useState("");
  const [redact, setRedact] = useState(false);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const url = () => {
    let u = "/api/insight/export?scope=" + (scope || "all");
    if (scope === "endpoint") {
      u += `&method=${encodeURIComponent(method)}&path=${encodeURIComponent(path)}`;
      if (trace) u += `&trace=${trace}`;
    }
    if (redact) u += "&redact=1";
    return u;
  };

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    fetch(url())
      .then((r) => r.text())
      .then(setMd)
      .finally(() => setLoading(false));
  }, [open, redact, trace]);

  if (!open) return null;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(md);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };
  const download = () => {
    const blob = new Blob([md], { type: "text/markdown" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = scope === "endpoint" ? "apispec-endpoint.md" : "apispec-issues.md";
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return html`
    <div class="modal-backdrop open" onClick=${(e) => e.target === e.currentTarget && onClose()}>
      <div class="modal" role="dialog" aria-label="Export to AI">
        <div class="modal-head">
          <h3>⤴ Export to AI ${scope === "endpoint" ? "(endpoint)" : "(all issues)"}</h3>
          <span class="spacer"></span>
          <label class="row muted" style="font-size:var(--fs-sm);cursor:pointer">
            <input type="checkbox" checked=${redact} onChange=${(e) => setRedact(e.target.checked)} /> redact identifiers
          </label>
        </div>
        <div class="modal-body pad">
          <p class="muted" style="font-size:var(--fs-sm);margin:0 0 var(--sp-2)">Paste into your AI assistant — it includes the issue(s)${scope === "endpoint" ? ", the resolution trace and handler source" : ""} and your config so it can suggest a code or config fix.</p>
          <textarea class="input" readonly style="min-height:340px;width:100%">${loading ? "loading…" : md}</textarea>
        </div>
        <div class="modal-foot">
          <span class="spacer"></span>
          <button class="btn ghost" onClick=${onClose}>Close</button>
          <button class="btn secondary" onClick=${download}>⤓ Download .md</button>
          <button class="btn" onClick=${copy}>${copied ? "✓ Copied" : "Copy"}</button>
        </div>
      </div>
    </div>
  `;
}
