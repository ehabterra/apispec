// config.js — Configure mode. Native Preact editors for the full config:
// API info/servers/security/tags/defaults/filters/type-mappings/overrides
// AND the framework detection patterns (routes / request body / responses
// / parameters / mounts / request context). Everything is bound to the
// store and sent (structured) to /api/generate — no legacy editor.
import { html, useState } from "/assets/js/preact.js";
import { useStore, setConfig, setState } from "/assets/js/store.js";
import { openLoadConfig, openSaveConfig, resetFrameworkDefaults } from "/assets/js/actions.js";
import { Info } from "/assets/js/components/charts.js";

/* ---- small field helpers ------------------------------------------- */
const txt = (label, value, onInput, ph = "") => html`
  <div class="field">
    <label>${label}</label>
    <input class="input" value=${value || ""} placeholder=${ph} onInput=${onInput} />
  </div>
`;
const area = (label, value, onInput, ph = "") => html`
  <div class="field">
    <label>${label}</label>
    <textarea class="input" style="min-height:70px" placeholder=${ph} onInput=${onInput}>
${value || ""}</textarea
    >
  </div>
`;
const lines = (arr) => (arr || []).join("\n");
const toLines = (v) =>
  v
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);

function Section({ title, hint, help, desc, children, openDefault = false }) {
  const [open, setOpen] = useState(openDefault);
  return html`
    <div class=${"section" + (open ? " open" : "")}>
      <div class="section-head" onClick=${() => setOpen((o) => !o)}>
        <span class="chev">▸</span><span>${title}</span>
        ${help && html`<${Info} text=${help} />`}
        ${hint && html`<span class="spacer"></span><span class="muted" style="font-size:var(--fs-xs)">${hint}</span>`}
      </div>
      ${open &&
      html`<div class="section-body">
        ${desc && html`<p class="muted" style="font-size:var(--fs-sm);margin:0 0 var(--sp-3)">${desc}</p>`}
        ${children}
      </div>`}
    </div>
  `;
}

const RowDelete = (onClick) =>
  html`<button class="btn ghost sm" title="remove" onClick=${onClick}>✕</button>`;

export function ConfigMode() {
  const s = useStore();
  const c = s.config;

  // generic list ops bound to a config key
  const setKey = (k, v) => setConfig({ [k]: v });
  const updAt = (k, i, patch) => {
    const a = [...(c[k] || [])];
    a[i] = { ...a[i], ...patch };
    setKey(k, a);
  };
  const addTo = (k, item) => setKey(k, [...(c[k] || []), item]);
  const delAt = (k, i) =>
    setKey(
      k,
      (c[k] || []).filter((_, j) => j !== i),
    );

  const setInfo = (patch) => setConfig({ info: { ...c.info, ...patch } });
  const setDefaults = (patch) => setConfig({ defaults: { ...c.defaults, ...patch } });
  const setFilter = (which, field, value) =>
    setConfig({ [which]: { ...(c[which] || {}), [field]: value } });
  const setExtDocs = (patch) =>
    setConfig({ externalDocs: { ...(c.externalDocs || {}), ...patch } });

  // Framework detection patterns live on state.frameworkConfig (seeded
  // from /api/detect). setFC replaces one pattern list / the request
  // context.
  const fc = s.frameworkConfig || {};
  const setFC = (key, value) => setState({ frameworkConfig: { ...fc, [key]: value } });

  return html`
    <div class="mode-split">
      <div class="leftpanel">
        <div class="pad" style="border-bottom:1px solid var(--border)">
          <strong>Configure</strong>
          <div class="muted" style="font-size:var(--fs-sm)">
            Everything here feeds Generate directly. Detection patterns
            control how routes &amp; types are discovered.
          </div>
        </div>
        <div class="stack pad">
          <button class="btn ghost" onClick=${openLoadConfig}>⤓ Load config file…</button>
          <button class="btn ghost" onClick=${openSaveConfig}>↧ Save config as…</button>
          <a class="btn ghost" href="/api/config.yaml" target="_blank">↧ Download config.yaml</a>
        </div>
        <div class="spacer"></div>
        <div class="pad" style="border-top:1px solid var(--border)">
          <div class="muted" style="font-size:var(--fs-sm);margin-bottom:6px">
            Edited the detection patterns and want to start over?
          </div>
          <button class="btn ghost sm" onClick=${resetFrameworkDefaults}>↺ Reset to framework defaults</button>
        </div>
      </div>

      <div class="content pad">
        <div style="width:100%">
          <${Section} title="API information" openDefault=${true}>
            ${txt("Title", c.info?.title, (e) => setInfo({ title: e.target.value }))}
            ${txt("Version", c.info?.version, (e) => setInfo({ version: e.target.value }))}
            ${area("Description", c.info?.description, (e) => setInfo({ description: e.target.value }))}
          <//>

          <${Section} title="External docs">
            ${txt("URL", c.externalDocs?.url, (e) => setExtDocs({ url: e.target.value }), "https://…")}
            ${txt("Description", c.externalDocs?.description, (e) => setExtDocs({ description: e.target.value }))}
          <//>

          <${Section} title="Servers" hint=${`${(c.servers || []).length}`}>
            ${(c.servers || []).map(
              (sv, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Server ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("servers", i))}
                  </div>
                  ${txt("URL", sv.url, (e) => updAt("servers", i, { url: e.target.value }), "https://api.example.com")}
                  ${txt("Description", sv.description, (e) => updAt("servers", i, { description: e.target.value }))}
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("servers", { url: "", description: "" })}>+ Add server</button>
          <//>

          <${Section} title="Tags" hint=${`${(c.tags || []).length}`}>
            ${(c.tags || []).map(
              (t, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Tag ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("tags", i))}
                  </div>
                  ${txt("Name", t.name, (e) => updAt("tags", i, { name: e.target.value }))}
                  ${txt("Description", t.description, (e) => updAt("tags", i, { description: e.target.value }))}
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("tags", { name: "", description: "" })}>+ Add tag</button>
          <//>

          <${Section} title="Defaults">
            ${txt("Request content-type", c.defaults?.requestContentType, (e) => setDefaults({ requestContentType: e.target.value }), "application/json")}
            ${txt("Response content-type", c.defaults?.responseContentType, (e) => setDefaults({ responseContentType: e.target.value }), "application/json")}
            ${txt("Default response status", c.defaults?.responseStatus, (e) => setDefaults({ responseStatus: parseInt(e.target.value, 10) || 0 }), "200")}
          <//>

          <${Section} title="Include / exclude filters">
            <div class="grid-cards" style="grid-template-columns:1fr 1fr">
              ${["include", "exclude"].map(
                (which) => html`
                  <div>
                    <strong style="font-size:var(--fs-sm);text-transform:capitalize">${which}</strong>
                    ${["packages", "files", "functions", "types"].map((f) =>
                      area(
                        f,
                        lines(c[which]?.[f]),
                        (e) => setFilter(which, f, toLines(e.target.value)),
                        "one per line",
                      ),
                    )}
                  </div>
                `,
              )}
            </div>
          <//>

          <${Section} title="Type mappings" hint=${`${(c.typeMapping || []).length}`}>
            ${(c.typeMapping || []).map(
              (m, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Mapping ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("typeMapping", i))}
                  </div>
                  ${txt("Go type", m.goType, (e) => updAt("typeMapping", i, { goType: e.target.value }), "uuid.UUID")}
                  <div class="row">
                    ${txt("OpenAPI type", m.openapiType?.type, (e) => updAt("typeMapping", i, { openapiType: { ...m.openapiType, type: e.target.value } }), "string")}
                    ${txt("Format", m.openapiType?.format, (e) => updAt("typeMapping", i, { openapiType: { ...m.openapiType, format: e.target.value } }), "uuid")}
                  </div>
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("typeMapping", { goType: "", openapiType: { type: "string" } })}>+ Add mapping</button>
          <//>

          <${Section} title="External types" hint=${`${(c.externalTypes || []).length}`}>
            ${(c.externalTypes || []).map(
              (m, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Type ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("externalTypes", i))}
                  </div>
                  ${txt("Name", m.name, (e) => updAt("externalTypes", i, { name: e.target.value }), "primitive.ObjectID")}
                  <div class="row">
                    ${txt("OpenAPI type", m.openapiType?.type, (e) => updAt("externalTypes", i, { openapiType: { ...m.openapiType, type: e.target.value } }), "string")}
                    ${txt("Format", m.openapiType?.format, (e) => updAt("externalTypes", i, { openapiType: { ...m.openapiType, format: e.target.value } }))}
                  </div>
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("externalTypes", { name: "", openapiType: { type: "string" } })}>+ Add external type</button>
          <//>

          <${Section} title="Overrides" hint=${`${(c.overrides || []).length}`}>
            ${(c.overrides || []).map(
              (o, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Override ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("overrides", i))}
                  </div>
                  ${txt("Function name", o.functionName, (e) => updAt("overrides", i, { functionName: e.target.value }), "pkg.Handler")}
                  ${txt("Summary", o.summary, (e) => updAt("overrides", i, { summary: e.target.value }))}
                  <div class="row">
                    ${txt("Response status", o.responseStatus, (e) => updAt("overrides", i, { responseStatus: parseInt(e.target.value, 10) || 0 }))}
                    ${txt("Response type", o.responseType, (e) => updAt("overrides", i, { responseType: e.target.value }))}
                  </div>
                  ${txt("Tags (comma-separated)", (o.tags || []).join(", "), (e) => updAt("overrides", i, { tags: e.target.value.split(",").map((x) => x.trim()).filter(Boolean) }))}
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("overrides", { functionName: "" })}>+ Add override</button>
          <//>

          <${Section} title="Security schemes" hint=${`${Object.keys(c.securitySchemes || {}).length}`}>
            <${SecuritySchemes} c=${c} />
          <//>

          <div class="section-group">Detection — how routes &amp; types are discovered</div>

          <${Section} title="Routes" help="Patterns that recognise route registration calls (e.g. e.GET(path, handler)) and where to read the method, path and handler." hint=${`${(fc.routePatterns || []).length}`}>
            <${PatternList} items=${fc.routePatterns} fields=${PATTERN_FIELDS.routePatterns} onChange=${(a) => setFC("routePatterns", a)} />
          <//>
          <${Section} title="Request body" help="Patterns that recognise request-body decoding (e.g. c.Bind(&req)) and which argument's type is the body." hint=${`${(fc.requestBodyPatterns || []).length}`}>
            <${PatternList} items=${fc.requestBodyPatterns} fields=${PATTERN_FIELDS.requestBodyPatterns} onChange=${(a) => setFC("requestBodyPatterns", a)} />
          <//>
          <${Section} title="Responses" help="Patterns that recognise response writes (e.g. c.JSON(status, body)) and where to read the status and body type." hint=${`${(fc.responsePatterns || []).length}`}>
            <${PatternList} items=${fc.responsePatterns} fields=${PATTERN_FIELDS.responsePatterns} onChange=${(a) => setFC("responsePatterns", a)} />
          <//>
          <${Section} title="Parameters" help="Patterns that recognise path/query/header/cookie parameter reads (e.g. c.Param('id'))." hint=${`${(fc.paramPatterns || []).length}`}>
            <${PatternList} items=${fc.paramPatterns} fields=${PATTERN_FIELDS.paramPatterns} onChange=${(a) => setFC("paramPatterns", a)} />
          <//>
          <${Section} title="Mounts / groups" help="Patterns that recognise sub-router mounts/groups (e.g. r.Group('/v1')) so nested routes get the right prefix." hint=${`${(fc.mountPatterns || []).length}`}>
            <${PatternList} items=${fc.mountPatterns} fields=${PATTERN_FIELDS.mountPatterns} onChange=${(a) => setFC("mountPatterns", a)} />
          <//>
          <${Section} title="Request context" help="Constrains which values count as the request body: the handler parameter types to watch and how the body is accessed.">
            <${RequestContextEditor} rc=${fc.requestContext} onChange=${(v) => setFC("requestContext", v)} />
          <//>
        </div>
      </div>
    </div>
  `;
}

/* ---- framework detection pattern editors --------------------------- */

const COMMON_MATCH = [
  ["callRegex", "Call regex", "text", "Regex matching the called function name, e.g. ^(?i)(GET|POST|PUT|DELETE)$"],
  ["recvTypeRegex", "Receiver type regex", "text", "Regex matching the receiver/owner type, e.g. echo(/v\\d)?\\.(Echo|Group)"],
  ["functionNameRegex", "Function name regex", "text", "Optional — match by the enclosing function's name instead of the call."],
];

const PATTERN_FIELDS = {
  routePatterns: [
    ...COMMON_MATCH,
    ["handlerArgIndex", "Handler arg index", "int", "0-based position of the handler argument."],
    ["pathArgIndex", "Path arg index", "int", "0-based position of the path argument."],
    ["methodArgIndex", "Method arg index", "int", "0-based position of the HTTP-method argument (if any)."],
    ["methodFromCall", "Method from call name", "bool", "Derive the HTTP method from the called function name (e.g. GET())."],
    ["methodFromHandler", "Method from handler", "bool", "Derive the method from the handler function name."],
    ["pathFromArg", "Path from arg", "bool", "Read the path from the path argument."],
    ["handlerFromArg", "Handler from arg", "bool", "Resolve the handler from the handler argument."],
  ],
  requestBodyPatterns: [
    ...COMMON_MATCH,
    ["typeArgIndex", "Type arg index", "int", "Argument whose type is the request body."],
    ["typeFromArg", "Type from arg", "bool", "Use the argument's type as the body type."],
    ["typeFromReturn", "Type from return", "bool", "Use the call's return type as the body type."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type."],
    ["requireRequestSource", "Require request source", "bool", "Only when the value originates from the HTTP request body."],
    ["bodyFromReceiver", "Body from receiver", "bool", "The body comes from the call receiver (e.g. a decoder bound to r.Body)."],
    ["bodySourceArgIndex", "Body source arg index", "int", "Argument carrying the request-body source."],
    ["allowForGetMethods", "Allow for GET/HEAD", "bool", "Permit a request body on GET/HEAD routes."],
  ],
  responsePatterns: [
    ...COMMON_MATCH,
    ["statusArgIndex", "Status arg index", "int", "Argument holding the HTTP status code."],
    ["typeArgIndex", "Type arg index", "int", "Argument whose type is the response body."],
    ["statusFromArg", "Status from arg", "bool", "Read the status from the status argument."],
    ["typeFromArg", "Type from arg", "bool", "Use the argument's type as the response type."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type."],
    ["defaultStatus", "Default status", "int", "Status used when none is detected (e.g. 200)."],
    ["defaultContentType", "Default content-type", "text", "Overrides the default content type for this pattern."],
  ],
  paramPatterns: [
    ...COMMON_MATCH,
    ["paramIn", "Parameter location", "select:path,query,header,cookie,form", "Where this parameter is read from."],
    ["paramArgIndex", "Param arg index", "int", "Argument holding the parameter name."],
    ["typeArgIndex", "Type arg index", "int", "Argument whose type is the parameter type."],
    ["typeFromArg", "Type from arg", "bool", "Use the argument's type as the parameter type."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type."],
  ],
  mountPatterns: [
    ...COMMON_MATCH,
    ["pathArgIndex", "Path arg index", "int", "Argument holding the mount path / prefix."],
    ["routerArgIndex", "Router arg index", "int", "Argument holding the sub-router."],
    ["pathFromArg", "Path from arg", "bool", "Read the mount path from the path argument."],
    ["routerFromArg", "Router from arg", "bool", "Track the sub-router from the router argument."],
    ["isMount", "Is mount", "bool", "Treat this call as a router mount/group."],
  ],
};

function fieldEditor(p, f, set) {
  const [key, label, type, help] = f;
  const val = p[key];
  const lbl = html`<label>${label} ${help ? html`<${Info} text=${help} />` : ""}</label>`;
  if (type === "bool") {
    return html`<div class="field" style="margin-bottom:8px">
      <label class="row" style="cursor:pointer;gap:6px">
        <input type="checkbox" checked=${!!val} onChange=${(e) => set(e.target.checked)} />
        <span>${label}</span>${help ? html`<${Info} text=${help} />` : ""}
      </label>
    </div>`;
  }
  if (type === "int") {
    return html`<div class="field">
      ${lbl}<input class="input" type="number" value=${val ?? ""} onInput=${(e) => set(e.target.value === "" ? 0 : parseInt(e.target.value, 10) || 0)} />
    </div>`;
  }
  if (type.startsWith("select:")) {
    const opts = type.slice(7).split(",");
    return html`<div class="field">
      ${lbl}<select class="input" value=${val || ""} onChange=${(e) => set(e.target.value)}>
        <option value="">—</option>
        ${opts.map((o) => html`<option value=${o}>${o}</option>`)}
      </select>
    </div>`;
  }
  return html`<div class="field">${lbl}<input class="input" value=${val || ""} onInput=${(e) => set(e.target.value)} /></div>`;
}

function PatternList({ items, fields, onChange }) {
  const list = items || [];
  const upd = (i, patch) => {
    const a = [...list];
    a[i] = { ...a[i], ...patch };
    onChange(a);
  };
  return html`
    ${list.length === 0 ? html`<p class="muted" style="font-size:var(--fs-sm)">No patterns — Generate falls back to the framework defaults.</p>` : ""}
    ${list.map(
      (p, i) => html`<div class="card">
        <div class="row"><strong style="font-size:var(--fs-sm)">Pattern ${i + 1}</strong><span class="spacer"></span>${RowDelete(() => onChange(list.filter((_, j) => j !== i)))}</div>
        ${fields.map((f) => fieldEditor(p, f, (val) => upd(i, { [f[0]]: val })))}
      </div>`,
    )}
    <button class="btn secondary sm" onClick=${() => onChange([...list, {}])}>+ Add pattern</button>
  `;
}

function RequestContextEditor({ rc, onChange }) {
  const set = (k, v) => onChange({ ...(rc || {}), [k]: v });
  return html`
    ${area("Handler param type regexes (one per line)", lines(rc?.typeRegexes), (e) => set("typeRegexes", toLines(e.target.value)), "^github.com/labstack/echo(/v\\d+)?\\.Context$")}
    ${area("Body accessors (one per line)", lines(rc?.bodyAccessors), (e) => set("bodyAccessors", toLines(e.target.value)), "Request().Body")}
  `;
}

function SecuritySchemes({ c }) {
  const schemes = c.securitySchemes || {};
  const names = Object.keys(schemes);
  const setSchemes = (obj) => setConfig({ securitySchemes: obj });
  const upd = (name, patch) => setSchemes({ ...schemes, [name]: { ...schemes[name], ...patch } });
  const rename = (oldN, newN) => {
    if (!newN || newN === oldN) return;
    const o = { ...schemes };
    o[newN] = o[oldN];
    delete o[oldN];
    setSchemes(o);
  };
  const del = (name) => {
    const o = { ...schemes };
    delete o[name];
    setSchemes(o);
  };
  const add = () => {
    let n = "auth";
    let i = 1;
    while (schemes[n]) n = "auth" + ++i;
    setSchemes({ ...schemes, [n]: { type: "http", scheme: "bearer" } });
  };
  return html`
    ${names.map(
      (name) => html`
        <div class="card">
          <div class="row">
            <input class="input" style="max-width:160px" value=${name} onChange=${(e) => rename(name, e.target.value.trim())} />
            <select class="input" style="max-width:150px" value=${schemes[name].type} onChange=${(e) => upd(name, { type: e.target.value })}>
              ${["http", "apiKey", "oauth2", "openIdConnect"].map((t) => html`<option value=${t}>${t}</option>`)}
            </select>
            <span class="spacer"></span>${RowDelete(() => del(name))}
          </div>
          ${schemes[name].type === "http" &&
          txt("Scheme", schemes[name].scheme, (e) => upd(name, { scheme: e.target.value }), "bearer")}
          ${schemes[name].type === "apiKey" &&
          html`<div class="row">
            ${txt("In", schemes[name].in, (e) => upd(name, { in: e.target.value }), "header")}
            ${txt("Param name", schemes[name].name, (e) => upd(name, { name: e.target.value }), "X-API-Key")}
          </div>`}
          ${schemes[name].type === "openIdConnect" &&
          txt("OpenID Connect URL", schemes[name].openIdConnectUrl, (e) => upd(name, { openIdConnectUrl: e.target.value }))}
        </div>
      `,
    )}
    <button class="btn secondary sm" onClick=${add}>+ Add scheme</button>
  `;
}
