// app.js — apispecui shell: top bar, icon rail, and the mode router.
// Mounts the whole UI with Preact + htm (no build step).
import { html, render, useEffect } from "/assets/js/preact.js";
import { useStore, setState } from "/assets/js/store.js";
import {
  detectInitial,
  generate,
  forceGenerate,
  stopGenerate,
  openCallGraph,
  openProject,
  closeBrowse,
  getBrowse,
  fmtDur,
} from "/assets/js/actions.js";
import { BrowseDialog } from "/assets/js/browse.js";
import { SpecMode } from "/assets/js/spec.js";
import { ConfigMode } from "/assets/js/config.js";
import { InsightMode } from "/assets/js/insight.js";
import { DocsMode } from "/assets/js/docs.js";

const RAIL = [
  ["start", "⚡", "Start / Spec"],
  ["configure", "⚙", "Configure"],
  ["insight", "◷", "Insight"],
  ["callgraph", "⌕", "Call graph ↗"],
  ["docs", "?", "Docs / help"],
];

// fmtBuilt renders the binary build time compactly (e.g. "May 30 14:32").
// This is the signal that exposes a stale binary: if it doesn't move after a
// rebuild, the running process isn't the one you just built.
function fmtBuilt(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// projectLabel renders the project as a module-relative path — the module
// name plus any subpath under it — instead of the absolute filesystem path.
// This keeps the username and parent directories (which often reveal client
// or employer names) off-screen in demos/screenshots while still identifying
// the project. Falls back to the basename when there's no module root.
function projectLabel(s) {
  if (!s.project) return "no project selected";
  const trim = (p) => p.replace(/\/+$/, "");
  const base = (p) => trim(p).split("/").filter(Boolean).pop() || p;
  const root = s.moduleRoot ? trim(s.moduleRoot) : "";
  const proj = trim(s.project);
  if (root && (proj === root || proj.startsWith(root + "/"))) {
    const rel = proj.slice(root.length).replace(/^\/+/, "");
    return rel ? base(root) + "/" + rel : base(root);
  }
  return base(proj);
}

function TopBar({ s }) {
  return html`
    <header class="topbar">
      <div class="brand">
        <span class="logo">▦</span> apispec
        ${s.apispecVersion
          ? html`<span
              class="version-badge"
              title=${`version: ${s.apispecVersion}\ncommit: ${s.apispecCommit || "?"}\nbuilt: ${s.apispecBuildTime || "?"}`}
              >${s.apispecVersion}${s.apispecBuildTime ? html` · built ${fmtBuilt(s.apispecBuildTime)}` : ""}</span
            >`
          : ""}
      </div>
      <div class="project" title=${s.project}>
        <span class="path">${projectLabel(s)}</span>
        <button class="btn ghost sm" onClick=${openProject}>📁</button>
      </div>
      <button class="btn" disabled=${s.generating || !s.project} onClick=${generate}>
        ${s.generating
          ? html`Generating…${s.genPhase ? html` ${s.genPhase}` : ""}${s.genElapsed ? html` <span class="gen-elapsed">${fmtDur(s.genElapsed)}</span>` : ""}`
          : "Generate ▸"}
      </button>
      ${s.generating ? html`<button class="btn danger" onClick=${stopGenerate} title="Stop the running engine">■ Stop</button>` : ""}
      ${(s.genBlocked || s.genStuckStopping) && !s.generating && s.project
        ? html`<button
            class="btn"
            style="background:var(--warn,#b80);border-color:var(--warn,#b80);color:#1a1300"
            onClick=${forceGenerate}
            title="Cancel the run that's still in flight / stuck stopping and start a fresh generation now"
          >
            ⚡ Force restart
          </button>`
        : ""}
      <span class="spacer"></span>
      ${s.status.text &&
      html`<span class=${"badge " + (s.status.kind === "err" ? "err" : s.status.kind === "ok" ? "ok" : s.status.kind === "warn" ? "warn" : "")}>
        <span class="dot"></span>${s.status.text}
      </span>`}
    </header>
  `;
}

function Rail({ s }) {
  return html`
    <nav class="rail">
      ${RAIL.map(
        ([id, glyph, tip]) => html`
          <button
            class=${"rail-btn" + (s.mode === id && id !== "callgraph" ? " active" : "")}
            onClick=${() => (id === "callgraph" ? openCallGraph() : setState({ mode: id }))}
          >
            ${glyph}<span class="tip">${tip}</span>
          </button>
        `,
      )}
    </nav>
  `;
}

// UnresolvedBanner points the user at the security-mapping picker after a
// generation that detected middleware it couldn't map to a scheme. Without it
// the picker is buried in a collapsed config section and easy to miss.
function UnresolvedBanner({ s }) {
  const n = (s.unresolvedSecurity || []).length;
  if (s.generating || n === 0 || s.mode === "configure") return "";
  return html`
    <div
      style="display:flex;align-items:center;gap:10px;padding:8px 14px;background:var(--warn-bg,#3a2c00);border-bottom:1px solid var(--warn,#b80);font-size:var(--fs-sm)"
    >
      <span>⚠ ${n} middleware detected on routes but not mapped to a security scheme — protected routes won't show as secured.</span>
      <span style="flex:1"></span>
      <button class="btn sm" onClick=${() => setState({ mode: "configure" })}>Map them →</button>
    </div>
  `;
}

function Main({ s }) {
  switch (s.mode) {
    case "configure":
      return html`<${ConfigMode} />`;
    case "insight":
      return html`<${InsightMode} />`;
    case "docs":
      return html`<${DocsMode} />`;
    default:
      return html`<${SpecMode} onGenerate=${generate} onBrowse=${openProject} />`;
  }
}

function App() {
  const s = useStore();
  useEffect(() => {
    detectInitial();
  }, []);

  const b = getBrowse();
  return html`
    <div class="shell">
      <${TopBar} s=${s} />
      <${Rail} s=${s} />
      <main class="shell-main">
        <div style="display:flex;flex-direction:column;flex:1 1 auto;min-width:0;min-height:0">
          <${UnresolvedBanner} s=${s} />
          <${Main} s=${s} />
        </div>
      </main>
      <${BrowseDialog}
        open=${b.open}
        mode=${b.mode}
        title=${b.title}
        start=${b.start}
        onClose=${closeBrowse}
        onPick=${b.onPick}
      />
    </div>
  `;
}

render(html`<${App} />`, document.getElementById("app"));
