// spec.js — Start/Spec mode: onboarding empty-state before a spec exists,
// then the OpenAPI preview (Swagger/Redoc/Scalar) once generated.
// The viewers are third-party HTML pages loaded in an iframe (fine — they
// are external renderers). The call-graph (apidiag) is NOT here: it opens
// in a separate browser tab from the rail.
import { html, useState } from "/assets/js/preact.js";
import { useStore, setState } from "/assets/js/store.js";

const VIEWS = [
  ["swagger", "Swagger UI"],
  ["redoc", "Redoc"],
  ["scalar", "Scalar"],
];

export function SpecMode({ onGenerate, onBrowse }) {
  const s = useStore();
  const [embed, setEmbed] = useState(null); // null | "swagger" | "redoc" | "scalar"

  if (!s.hasSpec) {
    return html`<${Onboarding} s=${s} onGenerate=${onGenerate} onBrowse=${onBrowse} />`;
  }

  // cache-bust so the iframe reloads the freshly generated spec
  const src = `/${s.specView}?t=${s.lastGenTick || ""}`;
  return html`
    <div class="mode-full">
      <div class="viewer-toolbar">
        <div class="seg">
          ${VIEWS.map(
            ([id, label]) => html`
              <button
                class=${s.specView === id ? "active" : ""}
                onClick=${() => setState({ specView: id })}
              >
                ${label}
              </button>
            `,
          )}
        </div>
        <span class="spacer"></span>
        <span class="badge ok"><span class="dot"></span>${s.lastPaths} paths</span>
        <button class="btn ghost sm" title="How to embed this viewer in your own app" onClick=${() => setEmbed(s.specView)}>
          <span style="color:var(--accent)">${"</>"}</span> Embed
        </button>
        <a class="btn ghost sm" href="/api/spec.yaml" target="_blank">spec.yaml</a>
        <a class="btn ghost sm" href="/api/spec.json" target="_blank">spec.json</a>
      </div>
      ${s.skipped && s.skipped.length ? html`<${SkippedBanner} skipped=${s.skipped} />` : ""}
      <iframe class="viewer-frame" src=${src} title="OpenAPI preview"></iframe>
      ${embed ? html`<${EmbedModal} view=${embed} onPick=${setEmbed} onClose=${() => setEmbed(null)} />` : ""}
    </div>
  `;
}

// EMBED holds copy-paste integration snippets for each viewer — a drop-in CDN
// page and a React component — so users can host the same docs in their own app.
// `/openapi.json` is a placeholder for wherever they serve the generated spec.
const EMBED = {
  swagger: {
    title: "Swagger UI",
    blurb: "The interactive “Try it out” viewer. The bundle is ~1.5 MB — use the CDN form for a zero-build setup.",
    npm: "npm install swagger-ui-dist   # or: swagger-ui-react",
    html: `<!doctype html>
<html>
<head>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      tryItOutEnabled: true,
    });
  </script>
</body>
</html>`,
    react: `import SwaggerUI from "swagger-ui-react";
import "swagger-ui-react/swagger-ui.css";

export default function Docs() {
  return <SwaggerUI url="/openapi.json" />;
}`,
  },
  redoc: {
    title: "Redoc",
    blurb: "A clean, single-page reference — read-only (no “Try it out”). Great for a public API portal.",
    npm: "npm install redoc",
    html: `<!doctype html>
<html>
<body>
  <redoc spec-url="/openapi.json"></redoc>
  <script src="https://cdn.jsdelivr.net/npm/redoc@2/bundles/redoc.standalone.js"></script>
</body>
</html>`,
    react: `import { RedocStandalone } from "redoc";

export default function Docs() {
  return <RedocStandalone specUrl="/openapi.json" />;
}`,
  },
  scalar: {
    title: "Scalar",
    blurb: "Modern and fast, with built-in API testing and strong OpenAPI 3.1 support.",
    npm: "npm install @scalar/api-reference   # or @scalar/api-reference-react",
    html: `<!doctype html>
<html>
<body>
  <script id="api-reference" data-url="/openapi.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`,
    react: `import { ApiReferenceReact } from "@scalar/api-reference-react";
import "@scalar/api-reference-react/style.css";

export default function Docs() {
  return <ApiReferenceReact configuration={{ spec: { url: "/openapi.json" } }} />;
}`,
  },
};

// CodeBlock renders a snippet with a header label and a copy button that
// flips to ✓ briefly on success.
function CodeBlock({ label, code }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      /* ignore */
    }
  };
  return html`
    <div class="embed-block">
      <div class="embed-block-head">
        <span class="embed-block-label">${label}</span>
        <button class=${"btn ghost sm embed-copy" + (copied ? " ok" : "")} onClick=${copy}>
          ${copied ? "✓ Copied" : "Copy"}
        </button>
      </div>
      <pre class="embed-code"><code>${code}</code></pre>
    </div>
  `;
}

// EmbedModal: per-viewer "host this in your app" guide (CDN drop-in + React).
function EmbedModal({ view, onPick, onClose }) {
  const sn = EMBED[view] || EMBED.swagger;
  return html`
    <div class="embed-backdrop" onClick=${onClose}>
      <div class="embed-modal" role="dialog" aria-label="Embed viewer" onClick=${(e) => e.stopPropagation()}>
        <div class="embed-head">
          <div>
            <h3><span style="color:var(--accent)">${"</>"}</span> Embed this spec in your app</h3>
            <p class="muted">Host the same API docs in your own project — pick a viewer.</p>
          </div>
          <button class="embed-x" title="Close" onClick=${onClose}>✕</button>
        </div>

        <div class="seg embed-seg">
          ${VIEWS.map(
            ([id, label]) => html`
              <button class=${view === id ? "active" : ""} onClick=${() => onPick(id)}>${label}</button>
            `,
          )}
        </div>

        <div class="embed-body">
          <p class="embed-blurb">${sn.blurb}</p>
          <${CodeBlock} label="1 · Drop-in HTML (CDN)" code=${sn.html} />
          <${CodeBlock} label="2 · React component" code=${sn.react} />
          <div class="embed-npm">Install: <code>${sn.npm}</code></div>
        </div>

        <div class="embed-foot">
          <span class="muted">
            Snippets point at <code>/openapi.json</code> — replace it with wherever you serve the spec.
            apispecui serves it at <code>/api/spec.json</code>.
          </span>
        </div>
      </div>
    </div>
  `;
}

// SkippedBanner warns that some in-module packages were dropped because they
// failed to type-check — so the spec is likely incomplete (missing routes).
// The usual cause is the project not building (e.g. an unresolved dependency).
function SkippedBanner({ skipped }) {
  const [open, setOpen] = useState(false);
  return html`
    <div class="skip-banner">
      <div class="skip-head">
        <span class="badge warn"><span class="dot"></span>${skipped.length} package(s) skipped</span>
        <span class="skip-msg">failed to type-check — the spec may be missing routes. Ensure the project builds (<code>go build ./...</code>).</span>
        <span class="spacer"></span>
        <button class="btn ghost sm" onClick=${() => setOpen((o) => !o)}>${open ? "Hide" : "Details"}</button>
        <button class="btn ghost sm" title="Dismiss" onClick=${() => setState({ skipped: [] })}>✕</button>
      </div>
      ${open
        ? html`<div class="skip-list">
            ${skipped.map(
              (p) => html`<div class="skip-item">
                <span class="mono">${p.package}</span>${p.reason ? html`<span class="muted"> — ${p.reason}</span>` : ""}
              </div>`,
            )}
          </div>`
        : ""}
    </div>
  `;
}

function Onboarding({ s, onGenerate, onBrowse }) {
  const [showWhat, setShowWhat] = useState(false);
  return html`
    <div class="empty">
      <div class="empty-inner">
        <h2>Generate an OpenAPI spec from your Go code</h2>
        <p>Three steps. No annotations — APISpec reads your real handlers.</p>

        <div class="card" style="text-align:left;margin-top:var(--sp-5)">
          <div class="field">
            <label>① Project</label>
            <div class="row">
              <input class="input mono" readonly value=${s.project || "(none selected)"} />
              <button class="btn ghost" onClick=${onBrowse}>📁 Browse…</button>
            </div>
          </div>
          <div class="field">
            <label>② Framework ${s.detected ? html`<span class="muted">(detected)</span>` : ""}</label>
            <select
              class="input"
              value=${s.framework}
              onChange=${(e) => setState({ framework: e.target.value })}
            >
              ${s.supportedFrameworks.map((f) => html`<option value=${f}>${f}</option>`)}
            </select>
          </div>
          <div class="field">
            <label>Analysis engine <span class="muted">(lazy is recommended)</span></label>
            <select
              class="input"
              value=${s.legacyTracker ? "legacy" : "lazy"}
              onChange=${(e) => setState({ legacyTracker: e.target.value === "legacy" })}
            >
              <option value="lazy">Lazy tracker (default)</option>
              <option value="legacy">Legacy tracker (eager)</option>
            </select>
          </div>
          <div class="field" style="margin-bottom:0">
            <label>③ Generate</label>
            <button class="btn" disabled=${s.generating || !s.project} style="display: block" onClick=${onGenerate}>
              ${s.generating ? `Generating… ${s.genPhase || ""}` : "Generate spec ▸"}
            </button>
          </div>
        </div>

        <div class="card" style="text-align:left;margin-top:var(--sp-3)">
          <div class="row" style="cursor:pointer" onClick=${() => setShowWhat((v) => !v)}>
            <strong>What APISpec does</strong><span class="spacer"></span>
            <span class="muted">${showWhat ? "▾" : "▸"}</span>
          </div>
          ${showWhat &&
          html`<p class="muted" style="margin-top:var(--sp-2)">
            APISpec walks your call graph from route registration to the real
            handler and infers request/response types from actual code —
            struct tags, literals, generics, wrapper/envelope responses. It
            detects routes for Gin, Echo, Chi, Fiber, Gorilla Mux and
            net/http. Pick your project, confirm the framework, and generate.
          </p>`}
        </div>
      </div>
    </div>
  `;
}
