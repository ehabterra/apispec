// insight_overview.js — Insight ▸ Overview.
//
// Three layers, each answering one question, so the page never asks the reader
// to work out what they are looking at or where the next detail is:
//
//   Brief            — is this spec in good shape? (one line)
//   Needs attention  — what should I do? (one ranked list; each row expands to
//                      the routes it affects, each route opens the Endpoint view)
//   At a glance      — how does each facet look? (tiles with ONE anatomy:
//                      label · value · bar · caption)
//
// Every tile opens the same side drawer, so the detail of any facet is always
// in the same place, one click away, and ←/→ step between facets without going
// back to the grid. The grid stays put behind it.
import { html, useState, useEffect, useRef } from "/assets/js/preact.js";
import { useStore } from "/assets/js/store.js";
import { openConfigGroup } from "/assets/js/actions.js";
import { Donut, Gauge, Info } from "/assets/js/components/charts.js";
import { Bars, shortName, statusColor, KIND_LABEL, INFO, ExportModal } from "/assets/js/insight_common.js";

/* ---- small helpers -------------------------------------------------- */

const pct = (have, total) => (total ? Math.round((have / total) * 100) : 0);
const plural = (n, one, many) => `${n} ${n === 1 ? one : many || one + "s"}`;

const TONE = {
  ok: { color: "var(--accent-2)", word: "good" },
  warn: { color: "var(--warn)", word: "worth a look" },
  crit: { color: "var(--danger)", word: "needs fixing" },
  neutral: { color: "var(--faint)", word: "for information" },
};

const METHOD_COLOR = {
  GET: "var(--accent)",
  POST: "var(--accent-2)",
  PUT: "var(--warn)",
  PATCH: "var(--info)",
  DELETE: "var(--danger)",
};

const EDGE_COLOR = { project: "var(--accent-2)", library: "var(--accent)", standard: "var(--muted)" };

// SegBar is the one bar every tile and drawer uses: segments in proportion,
// each titled with its count, so no chart needs a legend to be read.
function SegBar({ segs, tall }) {
  const shown = segs.filter((s) => s.n > 0);
  const label = shown.map((s) => `${s.n} ${s.title}`).join(", ") || "nothing to show";
  return html`<span class=${"segbar" + (tall ? " tall" : "")} role="img" aria-label=${label}>
    ${shown.length
      ? shown.map((s) => html`<span style=${`flex:${s.n};background:${s.color}`} title=${`${s.n} ${s.title}`}></span>`)
      : html`<span class="segbar-empty"></span>`}
  </span>`;
}

function Legend({ segs, all }) {
  return html`<div class="legend">
    ${segs
      .filter((s) => all || s.n > 0)
      .map((s) => html`<span class="legend-it"><span class="sw" style=${"background:" + s.color}></span>${s.title}<b>${s.n}</b></span>`)}
  </div>`;
}

// DSec is one section of the drawer: a heading, an optional explanation, and
// the content. Every drawer is built from these, so they all read alike.
function DSec({ title, info, note, children }) {
  return html`<section class="dsec">
    <div class="dsec-h"><h4>${title}</h4>${info ? html`<${Info} text=${info} />` : ""}</div>
    ${note ? html`<p class="dsec-note">${note}</p>` : ""}
    ${children}
  </section>`;
}

function RouteLink({ method, path, detail, onRoute }) {
  return html`<button class="route-link" onClick=${() => onRoute(method, path)} title="Open this route in the Endpoint view">
    <span class="rl-method" style=${"color:" + (METHOD_COLOR[method] || "var(--muted)")}>${method}</span>
    <span class="rl-path">${path}</span>
    ${detail ? html`<span class="rl-detail">${detail}</span>` : ""}
    <span class="rl-go">›</span>
  </button>`;
}

/* ---- needs attention ------------------------------------------------ */

const SEV_ORDER = { crit: 0, warn: 1 };
const SEV_WORD = { crit: "fix", warn: "review" };

// TODO_META turns an issue kind into a to-do: how urgent it is, what it means
// in plain words, and — only where config can fix it — which Configure group
// holds the fix. Kinds fixed in code get no button, so none suggests a setting
// can repair a write the analysis could not follow.
const TODO_META = {
  "dangling-ref": {
    sev: "crit",
    title: "References to a schema that was never defined",
    why: "Clients generated from this spec will not compile until the component exists.",
  },
  "no-responses": {
    sev: "crit",
    title: "Operations with no responses",
    why: "The handler was found, but no response write was followed. Its trace shows where it stops.",
  },
  "unresolved-type": {
    sev: "crit",
    title: "Types that did not become a schema",
    why: "The body was found, but its Go type could not be read. A type mapping or external type fixes it.",
    fix: { group: "types", label: "Map a type" },
  },
  "missing-body": {
    sev: "warn",
    title: "Request bodies without a shape",
    why: "A body is decoded, but it is documented as a generic object.",
  },
  "default-status": {
    sev: "warn",
    title: "Error statuses that fell back to a default",
    why: "An error response is written, but its status code could not be determined.",
  },
};

// buildTodo merges everything actionable into one list: the per-route issues
// grouped by cause (counted by distinct operation, not by occurrence),
// unmapped auth middleware, and gated strict categories that failed.
function buildTodo(rep, s) {
  const groups = {};
  for (const i of rep.issues) {
    const m = TODO_META[i.kind];
    if (!m) continue;
    const g = (groups[i.kind] = groups[i.kind] || { key: i.kind, ...m, routes: new Map() });
    const k = i.method + " " + i.path;
    const r = g.routes.get(k) || { method: i.method, path: i.path, refs: new Set(), detail: i.detail };
    if (i.ref) r.refs.add(shortName(i.ref));
    g.routes.set(k, r);
  }
  const todo = Object.values(groups).map((g) => {
    const routes = [...g.routes.values()].map((r) => ({
      method: r.method,
      path: r.path,
      detail: r.refs.size ? [...r.refs].join(", ") : r.detail,
    }));
    return { ...g, routes, count: routes.length, unit: "operation", units: "operations" };
  });

  const gated = new Set(s.strict || []);
  const unmapped = s.unresolvedSecurity || [];
  if (unmapped.length) {
    // The gated `security` category counts this same cause, so a failing gate
    // raises this row rather than adding a second one.
    const gateFails = gated.has("security") && (s.strictFindings || []).some((f) => f.category === "security");
    todo.push({
      key: "unmapped-auth",
      sev: gateFails ? "crit" : "warn",
      title: "Auth middleware not mapped to a security scheme",
      why: "The routes behind it are documented as public." + (gateFails ? " This fails the gated strict check." : ""),
      fix: { group: "security", label: "Map them" },
      count: unmapped.length,
      unit: "middleware",
      units: "middleware",
      names: unmapped.map((m) => (m.recvType ? shortName(m.recvType) + "." : "") + (m.functionName || "?")),
    });
  }

  for (const f of s.strictFindings || []) {
    if (!gated.has(f.category)) continue;
    if (f.category === "security" && unmapped.length) continue;
    todo.push({
      key: "strict-" + f.category,
      sev: "crit",
      title: `Strict check failing: ${f.category}`,
      why: f.detail,
      fix: { group: "analysis", label: "Strict mode" },
      count: f.count,
      unit: "finding",
      units: "findings",
    });
  }
  return todo.sort((a, b) => SEV_ORDER[a.sev] - SEV_ORDER[b.sev] || b.count - a.count);
}

function TodoRow({ item, onRoute }) {
  const [open, setOpen] = useState(false);
  const routes = item.routes || [];
  const names = item.names || [];
  const expandable = routes.length > 0 || names.length > 0;
  const LIMIT = 12;
  return html`<div class=${"todo " + item.sev + (open ? " open" : "")}>
    <div class="todo-head">
      <span class="todo-sev">${SEV_WORD[item.sev]}</span>
      <button class="todo-text" disabled=${!expandable} aria-expanded=${open} onClick=${() => setOpen(!open)}>
        <span class="todo-title">${item.title}</span>
        <span class="todo-why">${item.why}</span>
      </button>
      <span class="todo-count" title=${plural(item.count, item.unit)}>${item.count}<small>${item.count === 1 ? item.unit : item.units}</small></span>
      <span class="todo-actions">
        ${item.fix ? html`<button class="btn sm" onClick=${() => openConfigGroup(item.fix.group)}>${item.fix.label} →</button>` : ""}
        ${expandable
          ? html`<button class="btn ghost sm todo-toggle" aria-expanded=${open} onClick=${() => setOpen(!open)}>${open ? "Hide" : "Show"} ${routes.length ? "routes" : "names"}</button>`
          : ""}
      </span>
    </div>
    ${open
      ? html`<div class="todo-body">
          ${routes.slice(0, LIMIT).map((r) => html`<${RouteLink} ...${r} onRoute=${onRoute} />`)}
          ${names.map((n) => html`<div class="todo-name mono">${n}</div>`)}
          ${routes.length > LIMIT ? html`<p class="dsec-note">…and ${routes.length - LIMIT} more — the Resolution card lists every one.</p>` : ""}
        </div>`
      : ""}
  </div>`;
}

/* ---- brief ---------------------------------------------------------- */

function Brief({ rep, todo, onExport }) {
  const crit = todo.filter((t) => t.sev === "crit").length;
  const a = rep.analysis || {};
  const h = rep.health;
  const tone = crit ? "crit" : todo.length ? "warn" : "ok";
  const headline = !todo.length
    ? "Nothing needs attention — every reference resolves and no gated check fails."
    : `${plural(todo.length, "thing")} ${todo.length === 1 ? "needs" : "need"} attention${crit ? ` — ${crit} to fix` : ""}.`;
  const facts = [
    a.primary ? `${a.primary}${a.frameworks.length > 1 ? ` + ${a.frameworks.length - 1} more` : ""}` : "",
    plural(rep.routes, "route"),
    plural(rep.operations, "operation"),
    plural(rep.components, "schema"),
  ].filter(Boolean);

  return html`<div class="brief">
    <${Gauge} value=${h.score} size=${84} label="clean" />
    <div class="brief-text">
      <div class="brief-head"><span class="tone-dot" style=${"background:" + TONE[tone].color}></span>${headline}</div>
      <div class="brief-sub">
        ${h.cleanRoutes} of ${h.totalRoutes} operations resolved cleanly
        <${Info} text=${INFO.health} />
      </div>
      <div class="brief-facts">${facts.map((f) => html`<span>${f}</span>`)}</div>
    </div>
    <span class="spacer"></span>
    <button class="btn export sm" disabled=${!rep.issues.length} title=${rep.issues.length ? "Export the issues, your config and context as Markdown for an AI assistant" : "No issues — nothing to export"} onClick=${onExport}>⤴ Export to AI</button>
  </div>`;
}

/* ---- tiles ---------------------------------------------------------- */

function Meter({ label, m, foot }) {
  const p = pct(m.have, m.total);
  const col = p >= 85 ? "var(--accent-2)" : p >= 60 ? "var(--warn)" : "var(--danger)";
  return html`<div class="meter">
    <div class="mtop"><span class="mt">${label}</span><span class="mp" style=${"color:" + col}>${m.total ? p + "%" : "—"}</span></div>
    <div class="mtrack"><span class="mfill" style=${`width:${p}%;background:${col}`}></span></div>
    <div class="mfoot">${foot}</div>
  </div>`;
}

// Each builder returns one tile: the glanceable face (value, bar, caption,
// tone) and the drawer's detail. The face never needs the detail to be read.

function resolutionTile(rep, ctx) {
  const r = rep.resolution;
  const total = r.full + r.partial + r.broken;
  const segs = [
    { n: r.full, color: "var(--accent-2)", title: "fully resolved" },
    { n: r.partial, color: "var(--warn)", title: "partial" },
    { n: r.broken, color: "var(--danger)", title: "broken" },
  ];
  const TAXO = {
    "default-status": ["Status defaulted", "var(--warn)"],
    "missing-body": ["Body not resolved", "var(--warn)"],
    "unresolved-type": ["External / unresolved type", "var(--faint)"],
    "dangling-ref": ["Dangling $ref", "var(--danger)"],
    "no-responses": ["No responses", "var(--danger)"],
    "wrapper-specialised": ["Wrapper specialised", "var(--info)"],
  };
  const taxo = rep.taxonomy
    .filter((t) => t.count > 0)
    .map((t) => ({ name: (TAXO[t.name] || [t.name])[0], count: t.count, color: (TAXO[t.name] || [])[1] }));
  const issues = rep.issues;
  // The value IS the brief's health score (operations with no warning-level
  // issue), so the page never shows two percentages for one question; the bar
  // keeps the full/partial/broken split visible.
  return {
    id: "resolution",
    label: "Resolution",
    question: "Did every operation resolve?",
    value: total ? rep.health.score + "%" : "—",
    unit: "clean",
    tone: r.broken ? "crit" : r.partial ? "warn" : "ok",
    segs,
    caption: r.broken || r.partial
      ? [`${r.full} fully resolved`, r.partial && `${r.partial} partial`, r.broken && `${r.broken} broken`].filter(Boolean).join(" · ")
      : "all fully resolved",
    detail: () => html`
      <${DSec} title="Operations by state" info=${INFO.resolution} note=${`${rep.health.cleanRoutes} of ${rep.health.totalRoutes} operations are clean — no warning-level issue. Partial operations are usable with a gap (a defaulted status, a generic body); broken ones reach the spec with a dangling reference, a placeholder type or no responses.`}>
        <${SegBar} segs=${segs} tall=${true} /><${Legend} segs=${segs} all=${true} />
      <//>
      <${DSec} title="Why not resolved" info=${INFO.taxonomy} note="One operation can appear under more than one cause.">
        ${taxo.length ? html`<${Bars} data=${taxo} />` : html`<p class="dsec-note">Nothing unresolved.</p>`}
      <//>
      <${DSec} title="Every issue, by route" note=${issues.length ? "Select a route to open its trace in the Endpoint view." : ""}>
        ${issues.length
          ? html`<div class="route-list">
              ${issues.map(
                (i) => html`<div class="issue-row">
                  <span class=${"badge " + (i.severity === "warn" ? "err" : "")}>${KIND_LABEL[i.kind] || i.kind}</span>
                  <${RouteLink} method=${i.method} path=${i.path} detail=${i.ref ? shortName(i.ref) : i.detail} onRoute=${ctx.onRoute} />
                </div>`,
              )}
            </div>`
          : html`<p class="dsec-note">No issues — every reference resolves.</p>`}
      <//>
    `,
  };
}

function bodiesTile(rep) {
  const rows = rep.statusBodies;
  // 204 No Content and 205 Reset Content must not carry a body (RFC 9110), so
  // an empty one is by design, not a write the analysis missed.
  const bodyExpected = (st) => st[0] === "2" && st !== "204" && st !== "205";
  const sum = (f, pred) => rows.filter((r) => (pred ? pred(r.status) : true)).reduce((a, r) => a + f(r), 0);
  const withSchema = sum((r) => r.withSchema);
  const freeForm = sum((r) => r.freeForm || 0);
  const unresolved = sum((r) => r.unresolved);
  const emptyBad = sum((r) => r.empty, bodyExpected);
  const emptyOK = sum((r) => r.empty, (st) => !bodyExpected(st));
  const segsOf = (r) => [
    { n: r.withSchema, color: "var(--accent-2)", title: "describe their fields" },
    { n: r.freeForm || 0, color: "var(--info)", title: "free-form object" },
    { n: r.unresolved, color: "var(--warn)", title: "type unresolved" },
    { n: bodyExpected(r.status) ? r.empty : 0, color: "var(--danger)", title: "no body where one is expected" },
    { n: bodyExpected(r.status) ? 0 : r.empty, color: "var(--muted)", title: "no body by design" },
  ];
  const segs = [
    { n: withSchema, color: "var(--accent-2)", title: "describe their fields" },
    { n: freeForm, color: "var(--info)", title: "free-form object" },
    { n: unresolved, color: "var(--warn)", title: "type unresolved" },
    { n: emptyBad, color: "var(--danger)", title: "no body where one is expected" },
    { n: emptyOK, color: "var(--muted)", title: "no body by design" },
  ];
  const parts = [unresolved && `${unresolved} unresolved`, emptyBad && `${emptyBad} missing a body`, freeForm && `${freeForm} free-form`].filter(Boolean);
  // Responses that carry a body, or should: the rest are empty by design and
  // say nothing about typing, so a spec of only 204s is not "0% typed".
  const bodied = withSchema + freeForm + unresolved + emptyBad;
  return {
    id: "bodies",
    label: "Response bodies",
    question: "Do responses say what they return?",
    value: bodied ? pct(withSchema, bodied) + "%" : "—",
    unit: "typed",
    tone: !bodied ? "neutral" : emptyBad || unresolved ? "warn" : "ok",
    segs,
    caption: !rows.length ? "no responses documented" : !bodied ? "no response carries a body" : parts.length ? parts.join(" · ") : "all bodies resolved",
    detail: () => html`
      <${DSec} title="By status code" info=${INFO.statusbodies}>
        <div class="status-grid">
          ${rows.map(
            (r) => html`
              <span class="mono" style=${`color:${statusColor(r.status)};font-weight:600`}>${r.status}</span>
              <${SegBar} segs=${segsOf(r)} />
              <span class="muted status-n">${r.withSchema}/${r.total}</span>
            `,
          )}
        </div>
        <${Legend} segs=${segs} />
      <//>
      <${DSec} title="What the gaps mean">
        <ul class="dlist">
          <li><b>No body where one is expected</b> — the route was found, but the value it writes was not. Open the route and read its trace.</li>
          <li><b>Type unresolved</b> — the write was found, but its Go type did not become a schema. A type mapping or an external type fixes it, not more tracing.</li>
          <li><b>Free-form object</b> — a <span class="mono">map[string]any</span>. That IS the type, so nothing is missing, but clients learn nothing about its fields.</li>
          <li><b>No body by design</b> — 204, 205, 3xx and most errors without a payload; not a gap.</li>
        </ul>
      <//>
    `,
  };
}

function coverageTile(rep) {
  const c = rep.coverage;
  // Authentication is the Security card's question, so it is not repeated here.
  const ms = [c.requestBody, c.errorResponses].filter((m) => m.total > 0);
  const have = ms.reduce((a, m) => a + m.have, 0);
  const total = ms.reduce((a, m) => a + m.total, 0);
  const low = Math.min(...ms.map((m) => pct(m.have, m.total)), 100);
  const segs = [
    { n: have, color: "var(--accent-2)", title: "documented" },
    { n: total - have, color: "var(--panel-3)", title: "not documented" },
  ];
  return {
    id: "coverage",
    label: "Coverage",
    question: "Are the common facets documented?",
    value: total ? pct(have, total) + "%" : "—",
    unit: "covered",
    tone: !total ? "neutral" : low < 60 ? "warn" : "ok",
    segs,
    caption: total
      ? [c.requestBody.total && `bodies ${pct(c.requestBody.have, c.requestBody.total)}%`, c.errorResponses.total && `errors ${pct(c.errorResponses.have, c.errorResponses.total)}%`].filter(Boolean).join(" · ")
      : "nothing to measure",
    detail: () => html`
      <${DSec} title="Facets" info=${INFO.coverage}>
        <div class="meters">
          ${Meter({ label: "Request body documented", m: c.requestBody, foot: `${c.requestBody.have} of ${c.requestBody.total} write operations declare a body` })}
          ${Meter({ label: "Error responses documented", m: c.errorResponses, foot: `${c.errorResponses.have} of ${c.errorResponses.total} operations declare a 4xx/5xx` })}
        </div>
      <//>
      <${DSec} title="Reading it">
        <ul class="dlist">
          <li>A write operation (POST, PUT, PATCH) without a body usually means the decode was not recognised — check the route's trace.</li>
          <li>An operation without a 4xx/5xx either cannot fail, or fails through a path the analysis did not follow (a returned error, a middleware).</li>
        </ul>
      <//>
    `,
  };
}

function securityTile(rep, ctx) {
  const sec = rep.security;
  const unmapped = ctx.s.unresolvedSecurity || [];
  const total = sec.protected + sec.public + sec.unsecured;
  const none = !(sec.schemesDefined || sec.protected || unmapped.length);
  // Unauthenticated operations stand out only beside protected ones: in an API
  // with no auth at all they are simply the API, not a gap.
  const segs = [
    { n: sec.protected, color: "var(--accent-2)", title: "protected" },
    { n: sec.public, color: "var(--info)", title: "public" },
    { n: sec.unsecured, color: sec.protected ? "var(--warn)" : "var(--muted)", title: "no auth" },
  ];
  const usage = (sec.bySchemeUsage || []).map((d) => ({ name: d.name, count: d.count, color: "var(--accent-2)" }));
  return {
    id: "security",
    label: "Security",
    question: "Which operations require authentication?",
    value: none ? "None" : pct(sec.protected, total) + "%",
    unit: none ? "detected" : "protected",
    tone: unmapped.length ? "warn" : "neutral",
    segs,
    caption: unmapped.length
      ? `${plural(unmapped.length, "middleware", "middleware")} not mapped`
      : sec.schemes.length
        ? sec.schemes.join(", ")
        : "no authentication detected",
    detail: () => html`
      ${none
        ? html`<p class="dsec-note">No security schemes, and every operation is open. If routes are guarded by a custom middleware, map it under Configure ▸ Security.</p>`
        : ""}
      <${DSec} title="Operations" info=${INFO.secops}>
        <div class="split-viz">
          <${Donut} data=${segs.filter((x) => x.n).map((x) => ({ name: x.title, count: x.n, color: x.color }))} size=${112} thickness=${16} centerLabel=${total} centerSub="ops" />
        </div>
      <//>
      <${DSec} title="Schemes" info=${INFO.secschemes} note=${sec.globalSecurity ? "A document-level security requirement applies by default." : ""}>
        ${sec.schemes.length ? html`<div class="chips">${sec.schemes.map((n) => html`<span class="chip">${n}</span>`)}</div>` : html`<p class="dsec-note">None defined.</p>`}
        ${usage.length ? html`<div style="margin-top:var(--sp-2)"><${Bars} data=${usage} /></div>` : ""}
      <//>
      ${unmapped.length
        ? html`<${DSec} title="Middleware not mapped to a scheme" note="Each guards routes, but matched no security mapping — so those routes read as public.">
            ${unmapped.map((m) => html`<div class="todo-name mono">${(m.recvType ? shortName(m.recvType) + "." : "") + (m.functionName || "?")}</div>`)}
            <button class="btn sm" style="margin-top:var(--sp-2)" onClick=${() => openConfigGroup("security")}>Map them →</button>
          <//>`
        : ""}
    `,
  };
}

function gateTile(rep, ctx) {
  const s = ctx.s;
  const cats = (s.strictCategories || []).length ? s.strictCategories : (s.strictFindings || []).map((f) => f.category);
  const byCat = {};
  for (const f of s.strictFindings || []) byCat[f.category] = f;
  const gated = new Set(s.strict || []);
  const found = cats.filter((c) => byCat[c]);
  const failing = found.filter((c) => gated.has(c));
  const count = found.reduce((a, c) => a + byCat[c].count, 0);
  const segs = [
    { n: cats.length - found.length, color: "var(--accent-2)", title: "categories clean" },
    { n: found.length - failing.length, color: "var(--warn)", title: "categories with findings, not gated" },
    { n: failing.length, color: "var(--danger)", title: "gated categories failing" },
  ];
  return {
    id: "gate",
    label: "Quality gate",
    question: "Would the --strict check pass?",
    value: cats.length ? String(count) : "—",
    unit: count === 1 ? "finding" : "findings",
    tone: failing.length ? "crit" : found.length ? "warn" : cats.length ? "ok" : "neutral",
    segs,
    caption: !cats.length
      ? "run a generation to check"
      : failing.length
        ? `failing: ${failing.join(", ")}`
        : !gated.size
          ? found.length
            ? `not gating · ${plural(found.length, "category", "categories")} would fail`
            : "no findings · not gating"
          : "passing",
    detail: () => html`
      <${DSec} title="By category" info=${INFO.gate}>
        <div class="gate-grid">
          ${cats.map((cat) => {
            const f = byCat[cat];
            const g = gated.has(cat);
            const col = !f ? "var(--accent-2)" : g ? "var(--danger)" : "var(--warn)";
            return html`<div class="gate-row">
              <span class="gate-count" style=${"color:" + col}>${f ? f.count : "✓"}</span>
              <span class="mono gate-cat">${cat}${g ? html`<span class="chip-count">gated</span>` : ""}</span>
              <span class="muted gate-det">${f ? f.detail : "nothing found"}</span>
            </div>`;
          })}
        </div>
      <//>
      <${DSec} title="Gating" note=${gated.size ? `Gated: ${[...gated].join(", ")}. A finding there fails the run (exit code 3 in CI).` : "No category is gated, so these findings never fail a run. Gate the ones your CI should refuse."}>
        <button class="btn sm" onClick=${() => openConfigGroup("analysis")}>Strict mode settings →</button>
      <//>
    `,
  };
}

function shapeTile(rep, ctx) {
  const segs = rep.byMethod.map((m) => ({ n: m.count, color: METHOD_COLOR[m.name] || "var(--muted)", title: m.name }));
  const maxType = Math.max(1, ...rep.topTypes.map((t) => t.count));
  return {
    id: "shape",
    label: "API shape",
    question: "What does the API look like?",
    value: String(rep.operations),
    unit: rep.operations === 1 ? "operation" : "operations",
    tone: "neutral",
    segs,
    caption: [plural(rep.byStatus.length, "status", "statuses"), plural(rep.byContentType.length, "media type"), plural(rep.components, "schema")].join(" · "),
    detail: () => html`
      <${DSec} title="Methods" info=${INFO.methods}>
        <${SegBar} segs=${segs} tall=${true} /><${Legend} segs=${segs} />
      <//>
      ${rep.byStatus.length
        ? html`<${DSec} title="Status codes" info=${INFO.status}>
            <div class="split-viz">
              <${Donut} data=${rep.byStatus.map((d) => ({ ...d, color: statusColor(d.name) }))} size=${112} thickness=${16} centerLabel=${rep.byStatus.reduce((a, b) => a + b.count, 0)} centerSub="responses" />
            </div>
          <//>`
        : ""}
      ${rep.byContentType.length ? html`<${DSec} title="Media types" info=${INFO.ctype}><${Bars} data=${rep.byContentType} color="var(--info)" /><//>` : ""}
      ${rep.byTag.length
        ? html`<${DSec} title="Tags" info=${INFO.tags} note="Select a tag to list its routes in the Endpoint view.">
            <div class="chips">
              ${rep.byTag.map((t) => html`<button class="chip" onClick=${() => ctx.onTag(t.name)}>${t.name}<span class="chip-count">${t.count}</span></button>`)}
            </div>
          <//>`
        : ""}
      ${rep.topTypes.length
        ? html`<${DSec} title="Most-referenced types" info=${INFO.toptypes}>
            <div class="ranklist">
              ${rep.topTypes.map(
                (t, i) => html`<div class=${"rank-row" + (i < 3 ? " top" : "")}>
                  <span class="rank-n">${i + 1}</span>
                  <div class="rank-body">
                    <div class="rank-top"><span class="rank-name" title=${t.name.replace(/_/g, ".")}>${shortName(t.name)}</span><span class="rank-count">${t.count}×</span></div>
                    <div class="rank-bar"><span style=${`width:${(t.count / maxType) * 100}%`}></span></div>
                  </div>
                </div>`,
              )}
            </div>
          <//>`
        : ""}
    `,
  };
}

function internalsTile(rep) {
  const a = rep.analysis;
  const cg = rep.callGraph || {};
  const ep = a.entrypoints;
  const itf = rep.interfaces;
  const vd = rep.verbDispatch;
  const segs = (cg.edgeKinds || []).map((k) => ({ n: k.count, color: EDGE_COLOR[k.name] || "var(--muted)", title: k.name + " calls" }));
  const hot = cg.hotFunctions || [];
  const busy = cg.busyPackages || [];
  return {
    id: "internals",
    label: "How it was read",
    question: "What was the spec derived from?",
    value: a.primary || (a.frameworks[0] ?? "custom"),
    unit: a.frameworks.length > 1 ? `+ ${a.frameworks.length - 1} more` : "",
    tone: "neutral",
    segs,
    caption: [a.engine && `${a.engine} tracker`, `${cg.edges || 0} call edges`, cg.packages && plural(cg.packages, "package")].filter(Boolean).join(" · "),
    detail: () => html`
      <${DSec} title="Frameworks" info=${INFO.analysis}>
        ${a.frameworks.length
          ? html`<div class="chips">
              ${a.frameworks.map((f) => html`<span class="chip" title=${f === a.primary ? "primary — its patterns lead where two frameworks could match the same call" : "secondary — merged in, receiver-scoped"}>${f}${f === a.primary ? html`<span class="chip-count">primary</span>` : ""}</span>`)}
            </div>`
          : html`<p class="dsec-note">${a.primary ? `${a.primary} — configured, not auto-detected.` : "No framework detected — the configured patterns were used as-is."}</p>`}
        ${a.frameworks.length > 1 ? html`<p class="dsec-note">Every framework contributes its patterns; the primary decides where they overlap.</p>` : ""}
      <//>
      ${ep
        ? html`<${DSec} title="CLI-dispatched entry points" info=${INFO.entrypoints}
            note=${ep.rooted > 0
              ? `${ep.rooted} of ${ep.declared} led to a route registration and were analysed as extra entry points.`
              : `None of the ${ep.declared} register a route. If routes are missing, they are registered somewhere this gate cannot see.`}>
            <${Legend} all=${true} segs=${[
              { n: ep.rooted, color: "var(--accent-2)", title: "rooted" },
              { n: ep.alreadyReachable, color: "var(--info)", title: "already reachable" },
              { n: ep.noRoutes, color: "var(--muted)", title: "register no route" },
            ]} />
          <//>`
        : ""}
      ${itf.total
        ? html`<${DSec} title="Interface resolution" info=${INFO.interfaces}>
            <${Legend} all=${true} segs=${[
              { n: itf.singleImpl, color: "var(--accent-2)", title: "single implementation" },
              { n: itf.ambiguous, color: "var(--warn)", title: "ambiguous" },
              { n: itf.unimplemented, color: "var(--faint)", title: "unimplemented" },
            ]} />
            ${itf.ambiguousList.length
              ? html`<p class="dsec-note">Ambiguous — may be kept general (<span class="mono">any</span>):</p><${Bars} data=${itf.ambiguousList.map((x) => ({ ...x, color: "var(--warn)" }))} />`
              : ""}
          <//>`
        : ""}
      ${vd.length
        ? html`<${DSec} title="Verb dispatch" info=${INFO.verbdispatch} note=${`${plural(vd.length, "handler")} split into one operation per method.`}>
            <div class="disp-list">${vd.map((d) => html`<div class="disp-row"><span class="mono" style="color:var(--accent)">${d.handler}</span><span class="muted">${(d.methods || []).join("  ")}</span></div>`)}</div>
          <//>`
        : ""}
      <${DSec} title="Call graph" info=${INFO.callgraph} note=${`${plural(cg.packages || 0, "package")} · ${plural(cg.functions || 0, "function")} · ${plural(cg.edges || 0, "call edge")}`}>
        ${segs.length ? html`<${SegBar} segs=${segs} tall=${true} /><${Legend} segs=${segs} />` : ""}
      <//>
      ${hot.length ? html`<${DSec} title="Hot functions" note="Your most-called functions — the shared hubs."><${Bars} data=${hot} color="var(--accent-2)" /><//>` : ""}
      ${busy.length ? html`<${DSec} title="Busiest packages" note="Where your code concentrates."><${Bars} data=${busy} color="var(--info)" /><//>` : ""}
    `,
  };
}

function Tile({ t, active, onOpen }) {
  return html`<button class=${"tile" + (active ? " active" : "")} onClick=${onOpen} aria-haspopup="dialog" title=${t.question}>
    <span class="tile-top">
      <span class="tone-dot" style=${"background:" + TONE[t.tone].color} title=${TONE[t.tone].word}></span>
      <span class="tile-label">${t.label}</span>
      <span class="tile-more" aria-hidden="true">›</span>
    </span>
    <span class="tile-value">${t.value}${t.unit ? html`<span class="tile-unit">${t.unit}</span>` : ""}</span>
    <${SegBar} segs=${t.segs} />
    <span class="tile-caption">${t.caption}</span>
  </button>`;
}

/* ---- drawer --------------------------------------------------------- */

// Drawer shows one tile's detail. It is the only place detail appears, so the
// reader never has to hunt for it; ←/→ step through the tiles in grid order and
// Esc returns to the grid with the tile that was open still highlighted.
function Drawer({ tiles, openId, setOpenId }) {
  const ref = useRef(null);
  const i = tiles.findIndex((t) => t.id === openId);

  useEffect(() => {
    if (i < 0) return;
    const onKey = (e) => {
      if (e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)) return;
      if (e.key === "Escape") setOpenId("");
      else if (e.key === "ArrowRight") setOpenId(tiles[(i + 1) % tiles.length].id);
      else if (e.key === "ArrowLeft") setOpenId(tiles[(i - 1 + tiles.length) % tiles.length].id);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [i, tiles.length]);

  // Each facet starts at its top, and focus moves into the drawer so the
  // keyboard follows what is on screen.
  useEffect(() => {
    if (i >= 0 && ref.current) {
      ref.current.scrollTop = 0;
      ref.current.focus({ preventScroll: true });
    }
  }, [openId]);

  if (i < 0) return "";
  const t = tiles[i];
  const prev = tiles[(i - 1 + tiles.length) % tiles.length];
  const next = tiles[(i + 1) % tiles.length];
  return html`
    <div class="drawer-backdrop" onClick=${() => setOpenId("")}></div>
    <aside class="drawer" role="dialog" aria-modal="true" aria-label=${t.label} tabindex="-1" ref=${ref}>
      <header class="drawer-head">
        <div class="drawer-kicker">At a glance · ${i + 1} of ${tiles.length}</div>
        <div class="drawer-title">
          <span class="tone-dot" style=${"background:" + TONE[t.tone].color}></span>
          <h3>${t.label}</h3>
          <span class="spacer"></span>
          <button class="drawer-x" aria-label="Close" title="Close (Esc)" onClick=${() => setOpenId("")}>✕</button>
        </div>
        <p class="drawer-q">${t.question}</p>
        <div class="drawer-face">
          <span class="tile-value">${t.value}${t.unit ? html`<span class="tile-unit">${t.unit}</span>` : ""}</span>
          <span class="tile-caption">${t.caption}</span>
        </div>
      </header>
      <div class="drawer-body">${t.detail()}</div>
      <footer class="drawer-foot">
        <button class="btn ghost sm" onClick=${() => setOpenId(prev.id)} title="Previous (←)">‹ ${prev.label}</button>
        <span class="spacer"></span>
        <button class="btn ghost sm" onClick=${() => setOpenId(next.id)} title="Next (→)">${next.label} ›</button>
      </footer>
    </aside>
  `;
}

/* ---- overview ------------------------------------------------------- */

export function Overview({ rep, onTag, onRoute }) {
  const s = useStore();
  const [openId, setOpenId] = useState("");
  const [exportOpen, setExportOpen] = useState(false);
  const ctx = { s, onTag, onRoute };
  const todo = buildTodo(rep, s);
  // Two rows, each a question: is the spec complete, and what is the API. The
  // drawer steps through both in this order.
  const quality = [resolutionTile(rep, ctx), bodiesTile(rep), coverageTile(rep), gateTile(rep, ctx)];
  const about = [securityTile(rep, ctx), shapeTile(rep, ctx), internalsTile(rep)];
  const tiles = [...quality, ...about];
  const row = (title, list) => html`<div class="tile-row">
    <div class="tile-row-h">${title}</div>
    <div class="tiles" style=${"--cols:" + list.length}>
      ${list.map((t) => html`<${Tile} key=${t.id} t=${t} active=${t.id === openId} onOpen=${() => setOpenId(t.id)} />`)}
    </div>
  </div>`;

  return html`
    <div class="overview">
      <${Brief} rep=${rep} todo=${todo} onExport=${() => setExportOpen(true)} />

      <div class=${"ov-main" + (todo.length ? " has-todo" : "")}>
      ${todo.length
        ? html`<section class="ov-block ov-todo">
            <div class="ov-h">
              <h3>Needs attention</h3>
              <span class="ov-sub">Most urgent first. Open a row to see what it affects.</span>
            </div>
            <div class="todo-list">${todo.map((t) => html`<${TodoRow} key=${t.key} item=${t} onRoute=${onRoute} />`)}</div>
          </section>`
        : ""}

      <section class="ov-block ov-glance">
        <div class="ov-h">
          <h3>At a glance</h3>
          <span class="ov-sub">Select a card for its detail. ← → step between them.</span>
        </div>
        ${row("How complete is the spec", quality)}
        ${row("What the API is", about)}
      </section>
      </div>
    </div>

    <${Drawer} tiles=${tiles} openId=${openId} setOpenId=${setOpenId} />
    <${ExportModal} open=${exportOpen} onClose=${() => setExportOpen(false)} scope="all" />
  `;
}
