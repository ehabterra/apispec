// app.js — apispecui shell: top bar, icon rail, and the mode router.
// Mounts the whole UI with Preact + htm (no build step).
import { html, render, useEffect } from "/assets/js/preact.js";
import { useStore, setState } from "/assets/js/store.js";
import {
  detectInitial,
  generate,
  stopGenerate,
  openCallGraph,
  openProject,
  closeBrowse,
  getBrowse,
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
        <span class="path">${s.project || "no project selected"}</span>
        <button class="btn ghost sm" onClick=${openProject}>📁</button>
      </div>
      <span class="spacer"></span>
      ${s.status.text &&
      html`<span class=${"badge " + (s.status.kind === "err" ? "err" : s.status.kind === "ok" ? "ok" : s.status.kind === "warn" ? "warn" : "")}>
        <span class="dot"></span>${s.status.text}
      </span>`}
      ${s.generating ? html`<button class="btn danger" onClick=${stopGenerate} title="Stop the running engine">■ Stop</button>` : ""}
      <button class="btn" disabled=${s.generating || !s.project} onClick=${generate}>
        ${s.generating ? `Generating… ${s.genPhase || ""}` : "Generate ▸"}
      </button>
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
      <main class="shell-main"><${Main} s=${s} /></main>
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
