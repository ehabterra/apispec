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
              In <strong>Configure</strong>, use the filter box to jump to a
              setting, and <strong>Expand/Collapse all</strong> to scan
              quickly. Every section has an <strong>ⓘ</strong> with examples.
            </li>
            <li>
              Hover the <strong>ⓘ</strong> next to any metric in
              <strong>Insight</strong> to see exactly how it's computed.
            </li>
          </ul>
        </div>

        <div class="card">
          <h3>Configuration concepts</h3>
          <div class="desc">The knobs you'll reach for most — all in Configure.</div>
          <ul class="muted" style="margin:0;padding-left:18px;line-height:1.75">
            <li>
              <strong>Type mappings</strong> — give a Go type a specific
              OpenAPI representation when apispec can see it, e.g.
              <code>time.Time → string/date-time</code>,
              <code>uuid.UUID → string/uuid</code>.
            </li>
            <li>
              <strong>External types</strong> — describe types apispec
              <em>can't</em> see the source of (third-party/opaque), e.g.
              <code>gin.H → object</code>. This is the fix for an
              "unresolved/external placeholder type" on a non-module type.
            </li>
            <li>
              <strong>Overrides</strong> — force a summary, response
              status/type or tags for one handler, matched by its
              fully-qualified function name.
            </li>
            <li>
              <strong>Include / exclude</strong> — narrow analysis to the HTTP
              layer to speed up generation; tests and mocks are excluded by
              default.
            </li>
            <li>
              <strong>Request context</strong> — tells apispec which receivers
              are request contexts and which methods yield the body, so
              generic decoders (<code>json.Unmarshal</code>,
              <code>render.DecodeJSON</code>) aren't mistaken for body readers.
            </li>
          </ul>
        </div>

        <div class="card">
          <h3>Troubleshooting</h3>
          <ul class="muted" style="margin:0;padding-left:18px;line-height:1.75">
            <li>
              <strong>"Unresolved/external placeholder type"</strong> — if the
              type lives in <em>your</em> module it should resolve
              automatically; a placeholder usually means its package was
              skipped (run with <code>-verbose</code> and look for "Skipping
              package … due to errors") or uses generics/embedding the resolver
              didn't follow. Reach for <strong>External types</strong> only for
              genuinely third-party types.
            </li>
            <li>
              <strong>Spec looks stale after a code change</strong> — the spec
              is regenerated fresh every run, so a stale result means a stale
              binary. Check the build time next to the version badge; rebuild
              if it hasn't moved.
            </li>
            <li>
              <strong>A route's body/response is empty or wrong</strong> — open
              it in <strong>Insight</strong>, read the resolution trace, then
              add a <strong>Request body</strong>/<strong>Responses</strong>
              pattern or an <strong>Override</strong>.
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
