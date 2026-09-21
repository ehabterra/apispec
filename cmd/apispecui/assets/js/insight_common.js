// insight_common.js — pieces shared by the Insight Overview and Endpoint
// views: the small bar chart, name/status helpers, the metric explanations,
// and the Export-to-AI modal.
import { html, useState, useEffect } from "/assets/js/preact.js";

export function Bars({ data, color }) {
  const max = Math.max(1, ...data.map((d) => d.count));
  return html`<div class="bars">
    ${data.map((d) => {
      const col = d.color || color;
      return html`<div class="bar-row" title=${`${d.name}: ${d.count}`}>
        <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${d.name}</span>
        <div class="bar-track"><div class="bar-fill" style=${`width:${(d.count / max) * 100}%${col ? ";background:" + col : ""}`}></div></div>
        <span class="muted" style="text-align:right">${d.count}</span>
      </div>`;
    })}
  </div>`;
}

export const shortName = (n) => n.split("_").pop() || n;
export const statusColor = (s) =>
  ({ "2": "var(--accent-2)", "3": "var(--info)", "4": "var(--warn)", "5": "var(--danger)" })[s[0]] || "var(--muted)";
export const gradeColor = (g) =>
  ({ A: "var(--grade-a)", B: "var(--grade-b)", C: "var(--grade-c)", D: "var(--grade-d)" })[g] || "var(--muted)";
export const KIND_LABEL = {
  "dangling-ref": "dangling $ref",
  "unresolved-type": "unresolved type",
  "no-responses": "no responses",
  "missing-body": "missing body",
  "wrapper-specialised": "wrapper specialised",
  "default-status": "default status",
};

export const INFO = {
  health:
    "Share of routes whose request/response schemas fully resolve — no dangling $refs, unresolved/placeholder types, or synthesized path params.",
  components: "Named schemas emitted under components/schemas in the generated spec.",
  routes: "Distinct path templates (e.g. /users/{id}).",
  operations: "Method + path combinations (one path can have GET, POST, …).",
  methods: "HTTP methods across all operations.",
  status: "Response status codes declared across the API.",
  statusbodies:
    "For each status code: how many of its responses carry a resolved schema, how many were found but whose Go type could not be mapped, and how many document no body at all. At a 2xx an empty body usually means the write wasn't followed; an unresolved type means it was found but needs a type mapping.",
  analysis:
    "How this spec was produced rather than what's in it: the frameworks detected in the project, which one's patterns take precedence, the tracker engine, and what the entry-point gate did.",
  entrypoints:
    "Functions parked in a struct field that a library calls back — a urfave/cli Action, a cobra Run/RunE. Nothing in your code calls them, so their routes are only reachable if apispec roots them. It roots one only when its subtree actually registers a route.",
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
  alerts:
    "The issues that need you, grouped by cause and ranked by severity. Each is the count of operations affected — the per-route breakdown is in 'Needs attention' below.",
  resolution:
    "Every operation by how completely it resolved. Fully resolved = no issues. Partial = it works but a detail is missing (a defaulted status, a generic request body). Broken = a dangling $ref, an unresolved/external type, or no responses reaches the generated spec.",
  taxonomy:
    "The root cause behind each unresolved detail — one operation can appear in more than one bucket. This is what to fix to raise resolution health.",
  coverage:
    "How completely common facets are documented, as a share of the operations they apply to: request bodies on write operations, a 4xx/5xx on every operation, and authentication.",
  interfaces:
    "Interface method calls resolved to concrete implementations, read from the analyzer's implementation index (no extra traversal). Interfaces with several implementations may be kept general (erased to any) when the concrete type is ambiguous — those are the ones that cost schema precision.",
  verbdispatch:
    "Handlers that serve several HTTP methods from one function via a switch r.Method (or if r.Method ==). apispec splits each into its own operation.",
  gate:
    "The CLI's --strict check, run on every generation: each category counts a kind of shortfall that makes the spec quietly incomplete. A category ticked under Configure ▸ Strict mode is GATED — a finding there fails the run (exit code 3 in CI). Unticked categories are still counted, so you see what a gate would catch before turning it on.",
};

/* ---- export modal --------------------------------------------------- */

export function ExportModal({ open, onClose, scope, method, path, trace }) {
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

  // Only the latest request may fill the box. Toggling "redact" while the
  // previous export is in flight would otherwise let the UNREDACTED response
  // land last under a ticked box, and Copy/Download would hand it out. The
  // stale text is also cleared up front, so nothing is copyable until the
  // export that matches the checkbox has arrived.
  useEffect(() => {
    if (!open) return;
    const ctl = new AbortController();
    setMd("");
    setLoading(true);
    fetch(url(), { signal: ctl.signal })
      .then((r) => r.text())
      .then((text) => {
        if (!ctl.signal.aborted) setMd(text);
      })
      .catch((e) => {
        if (e.name !== "AbortError") setMd("Export failed: " + e.message);
      })
      .finally(() => {
        if (!ctl.signal.aborted) setLoading(false);
      });
    return () => ctl.abort();
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
          <button class="btn secondary" disabled=${loading || !md} onClick=${download}>⤓ Download .md</button>
          <button class="btn" disabled=${loading || !md} onClick=${copy}>${copied ? "✓ Copied" : "Copy"}</button>
        </div>
      </div>
    </div>
  `;
}
