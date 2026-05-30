// browse.js — the location dialog. Three modes:
//   project    → pick a folder (open project)
//   open-file  → pick a .yaml/.yml config file to load
//   save-file  → pick a folder + filename to save the config to
// Fixed default size, user-resizable (CSS resize:both), scrollable
// listing, and remembers its size in localStorage.
import { html, useState, useEffect, useRef } from "/assets/js/preact.js";
import { api } from "/assets/js/api.js";

const SIZE_KEY = "apispec.browse.size";

export function BrowseDialog({ open, mode = "project", title = "Open project", start = "", onClose, onPick }) {
  const [cur, setCur] = useState(null);
  const [path, setPath] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState(""); // selected file (open-file)
  const [filename, setFilename] = useState("apispec.yaml"); // save-file
  const modalRef = useRef(null);

  const fileMode = mode === "open-file" || mode === "save-file";

  async function load(p) {
    setLoading(true);
    setErr("");
    setSelected("");
    try {
      const d = await api.browse(p, fileMode ? "yaml" : undefined);
      setCur(d);
      setPath(d.path);
    } catch (e) {
      setErr(e.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!open) return;
    // Start browsing from the current project path (shown in the textbox)
    // rather than the home directory.
    load(start || "");
    if (mode === "save-file") setFilename("apispec.yaml");
    const el = modalRef.current;
    if (el) {
      try {
        const saved = JSON.parse(localStorage.getItem(SIZE_KEY) || "null");
        if (saved && saved.w && saved.h) {
          el.style.width = saved.w + "px";
          el.style.height = saved.h + "px";
        }
      } catch {
        /* ignore */
      }
    }
  }, [open, mode, start]);

  useEffect(() => {
    if (!open || !modalRef.current || typeof ResizeObserver === "undefined") return;
    const el = modalRef.current;
    const ro = new ResizeObserver(() => {
      localStorage.setItem(SIZE_KEY, JSON.stringify({ w: el.offsetWidth, h: el.offsetHeight }));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [open]);

  if (!open) return null;

  const join = (dir, name) => `${(dir || "").replace(/\/+$/, "")}/${name}`;

  const rows = [];
  if (cur && cur.parent) {
    rows.push(html`<div class="row-item" onClick=${() => load(cur.parent)}>
      <span class="glyph">↑</span><span>..</span>
    </div>`);
  }
  (cur?.entries || []).forEach((e) => {
    if (e.dir) {
      rows.push(html`<div
        class=${"row-item" + (e.hasGoMod ? " has-gomod" : "")}
        onClick=${() => load(e.path)}
        onDblClick=${() => mode === "project" && e.hasGoMod && onPick(e.path)}
        title=${e.path}
      >
        <span class="glyph">${e.hasGoMod ? "🟢" : "📁"}</span><span>${e.name}</span>
      </div>`);
    } else {
      rows.push(html`<div
        class=${"row-item file" + (selected === e.path ? " has-gomod" : "")}
        onClick=${() =>
          mode === "save-file" ? setFilename(e.name) : setSelected(e.path)}
        onDblClick=${() => mode === "open-file" && onPick(e.path)}
        style=${selected === e.path ? "background:var(--panel-2)" : ""}
        title=${e.path}
      >
        <span class="glyph">📄</span><span>${e.name}</span>
      </div>`);
    }
  });

  let foot;
  if (mode === "project") {
    foot = html`
      <button class="btn ghost" onClick=${onClose}>Cancel</button>
      <button class="btn" onClick=${() => onPick(cur && cur.path)}>Open this folder</button>
    `;
  } else if (mode === "open-file") {
    foot = html`
      <span class="muted" style="font-size:var(--fs-sm);overflow:hidden;text-overflow:ellipsis">
        ${selected ? selected.split("/").pop() : "select a .yaml file"}
      </span>
      <span class="spacer"></span>
      <button class="btn ghost" onClick=${onClose}>Cancel</button>
      <button class="btn" disabled=${!selected} onClick=${() => onPick(selected)}>Open file</button>
    `;
  } else {
    foot = html`
      <input
        class="input"
        style="flex:1"
        value=${filename}
        onInput=${(e) => setFilename(e.target.value)}
        placeholder="apispec.yaml"
      />
      <button class="btn ghost" onClick=${onClose}>Cancel</button>
      <button
        class="btn"
        disabled=${!filename.trim() || !cur}
        onClick=${() => onPick(join(cur.path, filename.trim()))}
      >
        Save here
      </button>
    `;
  }

  return html`
    <div class="modal-backdrop open" onClick=${(e) => e.target === e.currentTarget && onClose()}>
      <div class="modal" ref=${modalRef} role="dialog" aria-label=${title}>
        <div class="modal-head">
          <h3>📁 ${title}</h3>
          <input
            class="input"
            style="flex:1"
            value=${path}
            onInput=${(e) => setPath(e.target.value)}
            onKeyDown=${(e) => e.key === "Enter" && load(path.trim())}
          />
          <button class="btn ghost sm" onClick=${() => load(path.trim())}>Go</button>
        </div>
        <div class="modal-body">
          ${err && html`<div class="pad" style="color:var(--danger)">${err}</div>`}
          ${loading && !cur && html`<div class="pad muted">loading…</div>`}
          ${!err && rows}
          ${!err && fileMode && cur && !(cur.entries || []).some((e) => !e.dir) && !loading
            ? html`<div class="pad muted" style="font-size:var(--fs-sm)">no .yaml files here</div>`
            : ""}
        </div>
        <div class="modal-foot">
          ${cur && cur.parent
            ? html`<button class="btn ghost sm" onClick=${() => load(cur.parent)}>↑ Parent</button>`
            : ""}
          ${mode === "project" ? html`<span class="spacer"></span>` : ""}
          ${foot}
        </div>
      </div>
    </div>
  `;
}
