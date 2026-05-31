// store.js — a tiny hook-based global store. No external state library:
// components call useStore() to subscribe and re-render on setState().
import { useState, useEffect } from "/assets/js/preact.js";

const state = {
  mode: "start", // start | configure | insight | docs   (callgraph opens a tab)
  ready: false, // detect() finished
  project: "", // absolute project dir
  moduleRoot: "",
  modulePath: "",
  framework: "net/http",
  supportedFrameworks: ["gin", "chi", "echo", "fiber", "mux", "net/http"],
  openapiVersion: "3.1.0",
  // Full structured config edited by Configure mode. Seeded from
  // /api/detect and sent (structured) to /api/generate.
  config: {
    info: { title: "Generated API", version: "1.0.0", description: "" },
    servers: [],
    security: [],
    securitySchemes: {},
    tags: [],
    externalDocs: null,
    defaults: {},
    typeMapping: [],
    externalTypes: [],
    include: {},
    exclude: {},
    overrides: [],
  },
  frameworkConfig: null, // full pattern config when loaded/edited via YAML
  detected: null, // raw /api/detect response (used by legacy advanced editor)
  apispecVersion: "",
  apispecCommit: "",
  apispecBuildTime: "",
  status: { kind: "", text: "" }, // kind: "" | ok | warn | err
  generating: false,
  genPhase: "",
  genElapsed: 0, // ms since the current generation started (live ticker)
  hasSpec: false,
  lastPaths: 0,
  skipped: [], // [{package, reason}] dropped due to type errors (project didn't build)
  specView: "swagger", // swagger | redoc | scalar
  panelCollapsed: false,
};

const listeners = new Set();

export function getState() {
  return state;
}

export function setState(patch) {
  Object.assign(state, patch);
  listeners.forEach((fn) => fn());
}

export function setStatus(text, kind = "") {
  setState({ status: { text, kind } });
}

// Merge a patch into state.config and notify subscribers.
export function setConfig(patch) {
  Object.assign(state.config, patch);
  setState({});
}

// Subscribe a component to store changes.
export function useStore() {
  const [, force] = useState(0);
  useEffect(() => {
    const fn = () => force((n) => (n + 1) % 1e9);
    listeners.add(fn);
    return () => listeners.delete(fn);
  }, []);
  return state;
}
