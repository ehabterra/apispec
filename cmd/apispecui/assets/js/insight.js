// insight.js — Insight mode: the root (report loading, Overview/Endpoint
// switch) and the per-endpoint view. The Overview lives in
// insight_overview.js; helpers shared by both in insight_common.js.
import { html, useState, useEffect } from "/assets/js/preact.js";
import { useStore } from "/assets/js/store.js";
import { getJSON } from "/assets/js/api.js";
import { Info, TraceDiagram } from "/assets/js/components/charts.js";
import { shortName, statusColor, gradeColor, KIND_LABEL, INFO, ExportModal } from "/assets/js/insight_common.js";
import { Overview } from "/assets/js/insight_overview.js";

/* ---- report --------------------------------------------------------- */

function normalizeReport(d) {
  d = d || {};
  for (const k of ["issues", "endpoints", "byMethod", "byStatus", "statusBodies", "byContentType", "byTag", "topTypes", "taxonomy", "verbDispatch"]) {
    if (!Array.isArray(d[k])) d[k] = [];
  }
  d.analysis = d.analysis || {};
  if (!Array.isArray(d.analysis.frameworks)) d.analysis.frameworks = [];
  d.health = d.health || { score: 0, cleanRoutes: 0, totalRoutes: 0 };
  d.callGraph = d.callGraph || { packages: 0, functions: 0, edges: 0 };
  d.security = d.security || { schemesDefined: 0, schemes: [], protected: 0, public: 0, unsecured: 0, bySchemeUsage: [] };
  d.resolution = d.resolution || { full: 0, partial: 0, broken: 0 };
  d.coverage = d.coverage || {};
  for (const k of ["requestBody", "errorResponses", "protected"]) {
    d.coverage[k] = d.coverage[k] || { have: 0, total: 0 };
  }
  d.interfaces = d.interfaces || { total: 0, singleImpl: 0, ambiguous: 0, unimplemented: 0, ambiguousList: [] };
  if (!Array.isArray(d.interfaces.ambiguousList)) d.interfaces.ambiguousList = [];
  return d;
}

/* ---- root ----------------------------------------------------------- */

export function InsightMode() {
  const s = useStore();
  const [view, setView] = useState("overview");
  // What the Endpoint view opens on when the Overview hands over: a filter
  // (a tag) or one route. A fresh object per hand-over so the same route can be
  // opened twice in a row.
  const [handoff, setHandoff] = useState({ filter: "", sel: "" });
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
          ? html`<${Overview}
              rep=${rep}
              onTag=${(t) => { setHandoff({ filter: t, sel: "" }); setView("endpoint"); }}
              onRoute=${(method, path) => { setHandoff({ filter: "", sel: method + " " + path }); setView("endpoint"); }}
            />`
          : html`<${EndpointView} rep=${rep} initialFilter=${handoff.filter} initialSel=${handoff.sel} />`}
      </div>
    </div>
  `;
}

/* ---- endpoint ------------------------------------------------------- */

// matchesFilter is the route list's filter: method, path and tags, as typed.
const matchesFilter = (e, filter) =>
  !filter || (e.method + " " + e.path + " " + (e.tags || []).join(" ")).toLowerCase().includes(filter.toLowerCase());

function EndpointView({ rep, initialFilter, initialSel }) {
  // Open on the route handed over, else the first route the hand-over's filter
  // (a tag) keeps — never one the filtered list does not show.
  const first = rep.endpoints.find((e) => matchesFilter(e, initialFilter));
  const [sel, setSel] = useState(initialSel || (first ? first.method + " " + first.path : ""));
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

  const list = rep.endpoints.filter((e) => matchesFilter(e, filter));

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
