// docs.js — Docs / help mode. Phase 1 provides onboarding content and
// links; Phase 2 expands it with the integrate guide and inline examples.
import { html } from "/assets/js/preact.js";

const LINKS = [
  ["Supported frameworks", "https://github.com/ehabterra/apispec#framework-support"],
  ["Configuration guide", "https://github.com/ehabterra/apispec#configuration"],
  ["Go language support", "https://github.com/ehabterra/apispec#go-language-support"],
  ["Project README", "https://github.com/ehabterra/apispec"],
];

export function DocsMode() {
  return html`
    <div class="content pad">
      <div style="max-width:760px;margin:0 auto">
        <h2 style="margin-bottom:var(--sp-3)">Help & documentation</h2>

        <div class="card">
          <h3>How it works</h3>
          <div class="desc">From Go source to OpenAPI, in one pass.</div>
          <ol class="muted" style="margin:0;padding-left:18px;line-height:1.7">
            <li>Pick your Go module (📁 in the top bar).</li>
            <li>APISpec detects the web framework automatically.</li>
            <li>
              It walks the call graph from route registration to the real
              handler and infers request/response types from your code.
            </li>
            <li>Hit <strong>Generate</strong>; preview in Swagger/Redoc/Scalar.</li>
            <li>Explore the call graph via <strong>Call graph ↗</strong> (opens a new tab).</li>
          </ol>
        </div>

        <div class="card">
          <h3>Tips</h3>
          <ul class="muted" style="margin:0;padding-left:18px;line-height:1.7">
            <li>Folders with a <code>go.mod</code> are marked 🟢 in the project picker.</li>
            <li>The project picker is resizable — drag its corner; the size is remembered.</li>
            <li>
              Need framework patterns, type mappings or raw-YAML? Open the
              advanced editor from <strong>Configure → Advanced</strong>.
            </li>
          </ul>
        </div>

        <div class="card">
          <h3>Reference</h3>
          <div class="stack">
            ${LINKS.map(
              ([label, href]) => html`<a href=${href} target="_blank" rel="noopener">${label} ↗</a>`,
            )}
          </div>
        </div>
      </div>
    </div>
  `;
}
