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
        <a class="btn ghost sm" href="/api/spec.yaml" target="_blank">spec.yaml</a>
        <a class="btn ghost sm" href="/api/spec.json" target="_blank">spec.json</a>
      </div>
      <iframe class="viewer-frame" src=${src} title="OpenAPI preview"></iframe>
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
