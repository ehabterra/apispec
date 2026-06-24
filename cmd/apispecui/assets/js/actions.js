// actions.js — app-level actions and the browse-dialog controller.
// Lives in its own module so both app.js (shell) and config.js can
// trigger them without an import cycle.
import { getState, setState, setConfig, setStatus } from "/assets/js/store.js";
import { api, getJSON } from "/assets/js/api.js";

/* ---- browse dialog controller -------------------------------------- */

let browse = { open: false, mode: "project", title: "Open project", onPick: () => {} };

export function getBrowse() {
  return browse;
}
export function openBrowse(opts) {
  browse = {
    open: true,
    mode: "project",
    title: "Open project",
    onPick: () => {},
    start: getState().project || "",
    ...opts,
  };
  setState({});
}
export function closeBrowse() {
  browse = { ...browse, open: false };
  setState({});
}

/* ---- config <-> store mapping -------------------------------------- */

// Map the shared config sections (same camelCase keys in /api/detect,
// /api/load-config and /api/generate) into state.config.
function applyConfigSections(o) {
  if (!o) return;
  const c = getState().config;
  setConfig({
    info: o.info || c.info,
    servers: o.servers || [],
    security: o.security || [],
    securitySchemes: o.securitySchemes || {},
    securityMappings: o.securityMappings || [],
    tags: o.tags || [],
    externalDocs: o.externalDocs || null,
    defaults: o.defaults || {},
    typeMapping: o.typeMapping || [],
    externalTypes: o.externalTypes || [],
    include: o.include || {},
    exclude: o.exclude || {},
    overrides: o.overrides || [],
  });
}

export function applyDetect(d) {
  if (!d) return;
  setState({
    project: d.inputDir || d.moduleRoot || getState().project,
    moduleRoot: d.moduleRoot || "",
    modulePath: d.modulePath || "",
    framework: d.detectedFramework || getState().framework,
    supportedFrameworks: d.supportedFrameworks || getState().supportedFrameworks,
    openapiVersion: d.openapiVersion || getState().openapiVersion,
    frameworkConfig: d.frameworkConfig || null,
    detected: d,
    ready: true,
  });
  applyConfigSections(d);
}

export async function detectInitial() {
  try {
    const [detected, health] = await Promise.all([api.detect(), api.health()]);
    applyDetect(detected);
    if (health && health.version)
      setState({
        apispecVersion: health.version,
        apispecCommit: health.commit || "",
        apispecBuildTime: health.buildTime || "",
      });
    setStatus("ready", "ok");
  } catch (e) {
    setStatus(e.message, "err");
    setState({ ready: true });
  }
}

// Open the project picker wired to pickProject. (openBrowse alone
// defaults onPick to a no-op, so callers must use this for project mode.)
export function openProject() {
  openBrowse({ mode: "project", title: "Open project", onPick: pickProject });
}

export async function pickProject(dir) {
  closeBrowse();
  if (!dir) return;
  setStatus("loading project…");
  try {
    applyDetect(await api.project(dir));
    setStatus("project loaded", "ok");
  } catch (e) {
    setStatus(e.message, "err");
  }
}

/* ---- generate ------------------------------------------------------- */

function fullGenerateRequest() {
  const s = getState();
  const c = s.config;
  return {
    framework: s.framework,
    dir: s.project,
    openAPIVersion: s.openapiVersion,
    info: c.info,
    servers: c.servers,
    security: c.security,
    securitySchemes: c.securitySchemes,
    securityMappings: c.securityMappings,
    tags: c.tags,
    externalDocs: c.externalDocs,
    defaults: c.defaults,
    typeMapping: c.typeMapping,
    externalTypes: c.externalTypes,
    include: c.include,
    exclude: c.exclude,
    overrides: c.overrides,
    frameworkConfig: s.frameworkConfig || undefined,
  };
}

export async function generate() {
  const s = getState();
  if (!s.project || s.generating) return;
  const start = Date.now();
  setState({ generating: true, genPhase: "starting…", genElapsed: 0 });
  setStatus("generating…");
  // Live elapsed ticker — reassures the run is alive and makes a stall obvious
  // (a counter climbing on one phase reads as "stuck" where a static label hides it).
  const tick = setInterval(() => setState({ genElapsed: Date.now() - start }), 200);
  const poll = setInterval(async () => {
    try {
      const p = await api.progress();
      if (p && p.phase) setState({ genPhase: p.phase });
    } catch {
      /* ignore */
    }
  }, 600);
  try {
    const res = await api.generate(fullGenerateRequest());
    if (res && res.cancelled) {
      setStatus(`generation stopped · ${fmtDur(Date.now() - start)}`, "warn");
    } else {
      const skipped = res.skippedPackages || [];
      const took = fmtDur(Date.now() - start);
      setState({
        hasSpec: true,
        lastPaths: res.pathCount || 0,
        lastGenTick: Date.now(),
        mode: "start",
        specView: "swagger",
        skipped,
        unresolvedSecurity: res.unresolvedSecurity || [],
      });
      if (skipped.length) {
        setStatus(`generated ${res.pathCount || 0} paths · ${skipped.length} package(s) skipped · ${took}`, "warn");
      } else {
        setStatus(`generated ${res.pathCount || 0} paths in ${took}`, "ok");
      }
    }
  } catch (e) {
    // A rerun attempted while a stopped run is still winding down returns
    // 409 ("in progress / stopping") — surface that as a soft warning.
    if (/in progress|stopping/i.test(e.message)) {
      setStatus(e.message, "warn");
    } else {
      setStatus("generation failed: " + e.message, "err");
    }
  } finally {
    clearInterval(tick);
    clearInterval(poll);
    setState({ generating: false, genPhase: "", genElapsed: 0 });
  }
}

// fmtDur renders an elapsed millisecond span compactly: seconds with one
// decimal under a minute (e.g. "3.2s"), m:ss beyond (e.g. "1:23"). Empty for
// zero/negative so the live counter doesn't flash "0.0s" before the first tick.
export function fmtDur(ms) {
  if (!ms || ms < 0) return "";
  if (ms < 60000) return (ms / 1000).toFixed(1) + "s";
  const sec = Math.round(ms / 1000);
  return Math.floor(sec / 60) + ":" + String(sec % 60).padStart(2, "0");
}

// stopGenerate cancels the in-flight generation. The generate() call
// then resolves with {cancelled:true} and clears the generating flag, so
// the user can immediately rerun.
export async function stopGenerate() {
  setStatus("stopping…", "warn");
  try {
    await fetch("/api/generate/cancel", { method: "POST" });
  } catch (e) {
    setStatus("stop failed: " + e.message, "err");
  }
}

export function openCallGraph() {
  window.open("/diagram", "_blank", "noopener");
}

// Reload the detection patterns to the selected framework's defaults
// (handy after editing, to revert).
export async function resetFrameworkDefaults() {
  const fw = getState().framework || "net/http";
  setStatus("loading " + fw + " defaults…");
  try {
    const d = await getJSON("/api/default-framework?framework=" + encodeURIComponent(fw));
    setState({ frameworkConfig: d.frameworkConfig || null });
    setStatus("loaded " + fw + " detection defaults", "ok");
  } catch (e) {
    setStatus(e.message, "err");
  }
}

/* ---- config file: load / save -------------------------------------- */

export function openLoadConfig() {
  openBrowse({
    mode: "open-file",
    title: "Load config file",
    onPick: loadConfigFile,
  });
}

export function openSaveConfig() {
  openBrowse({
    mode: "save-file",
    title: "Save config as",
    onPick: saveConfigFile,
  });
}

async function loadConfigFile(path) {
  closeBrowse();
  if (!path) return;
  setStatus("loading config…");
  try {
    const d = await getJSON("/api/load-config?path=" + encodeURIComponent(path));
    applyConfigSections(d.config);
    if (d.config && d.config.framework) {
      setState({ frameworkConfig: d.config.framework });
    }
    setStatus("config loaded from " + path, "ok");
  } catch (e) {
    setStatus("load failed: " + e.message, "err");
  }
}

async function saveConfigFile(path) {
  closeBrowse();
  if (!path) return;
  await doSave(path, false);
}

async function doSave(path, overwrite) {
  setStatus("saving config…");
  try {
    const body = { ...fullGenerateRequest(), savePath: path, overwrite };
    const r = await fetch("/api/save-config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (r.status === 409) {
      if (confirm("File already exists:\n" + path + "\n\nOverwrite?")) {
        return doSave(path, true);
      }
      setStatus("save cancelled");
      return;
    }
    if (!r.ok) throw new Error((await r.text()) || r.status);
    const res = await r.json();
    setStatus(`saved config → ${res.path} (${res.bytes} bytes)`, "ok");
  } catch (e) {
    setStatus("save failed: " + e.message, "err");
  }
}
