// config.js — Configure mode. Native Preact editors for the full config:
// API info/servers/security/tags/defaults/filters/type-mappings/overrides
// AND the framework detection patterns (routes / request body / responses
// / parameters / mounts / request context). Everything is bound to the
// store and sent (structured) to /api/generate — no legacy editor.
import { html, useState, useEffect, createContext, useContext } from "/assets/js/preact.js";
import { useStore, setConfig, setState, getState } from "/assets/js/store.js";
import { openLoadConfig, openSaveConfig, resetFrameworkDefaults, generate } from "/assets/js/actions.js";
import { Info } from "/assets/js/components/charts.js";

// Shares the config toolbar's filter query and expand/collapse signal with
// every Section without threading props through each one.
const ConfigUI = createContext({ query: "", bulk: { n: 0, open: false } });

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

// applySchemaPatch merges a patch into an OpenAPI Schema (a type mapping's
// `openapiType`), dropping keys that become empty so the YAML stays clean
// (no `pattern: ""`). Empty objects are KEPT — `additionalProperties: {}` and
// `items: {}` are meaningful ("any").
function applySchemaPatch(schema, patch) {
  const next = { ...(schema || {}) };
  for (const [k, v] of Object.entries(patch)) {
    const empty =
      v === "" || v === null || v === undefined || (typeof v === "number" && Number.isNaN(v)) || (Array.isArray(v) && v.length === 0);
    if (empty) delete next[k];
    else next[k] = v;
  }
  return next;
}

const SCHEMA_TYPES = ["", "string", "integer", "number", "boolean", "array", "object"];
// coerce enum/example text to the schema's declared type so the spec is typed.
const coerceVal = (t, raw) =>
  t === "integer" ? parseInt(raw, 10) : t === "number" ? parseFloat(raw) : t === "boolean" ? raw === "true" : raw;

// SchemaEditor edits an OpenAPI Schema (the `openapiType` of a type mapping or
// external type). Type/format/$ref are always shown; the rest of the OpenAPI
// vocabulary lives under a disclosure so simple cases stay one line.
function SchemaEditor({ schema, onPatch }) {
  const sc = schema || {};
  const [adv, setAdv] = useState(false);
  const t = sc.type || "";

  const numF = (label, key, val, ph) => html`<div class="field">
    <label>${label}</label>
    <input class="input" type="number" value=${val ?? ""} placeholder=${ph || ""}
      onInput=${(e) => onPatch({ [key]: e.target.value === "" ? "" : Number(e.target.value) })} />
  </div>`;
  const chk = (label, key, val, on) => html`<label class="row" style="cursor:pointer;gap:6px;margin:0 0 6px">
    <input type="checkbox" checked=${!!val} onChange=${(e) => onPatch({ [key]: on ? on(e.target.checked) : e.target.checked || "" })} />
    <span>${label}</span>
  </label>`;
  const setItems = (p) => onPatch({ items: applySchemaPatch(sc.items, p) });

  return html`
    <div class="row">
      <div class="field">
        <label>OpenAPI type</label>
        <select class="input" value=${t} onChange=${(e) => onPatch({ type: e.target.value })}>
          ${SCHEMA_TYPES.map((o) => html`<option value=${o}>${o || "—"}</option>`)}
        </select>
      </div>
      ${txt("Format", sc.format, (e) => onPatch({ format: e.target.value }), "uuid, date-time, int64…")}
    </div>
    ${txt("$ref (reference a component instead)", sc["$ref"], (e) => onPatch({ ["$ref"]: e.target.value }), "#/components/schemas/Money")}

    <div class="schema-adv-toggle" onClick=${() => setAdv((v) => !v)}>
      <span class="chev">${adv ? "▾" : "▸"}</span> OpenAPI features (enum, pattern, ranges, items…)
    </div>
    ${adv &&
    html`<div class="schema-adv">
      ${txt("Description", sc.description, (e) => onPatch({ description: e.target.value }))}
      ${txt("Example", sc.example, (e) => onPatch({ example: e.target.value === "" ? "" : coerceVal(t, e.target.value) }))}
      ${txt("Enum (comma-separated)", (sc.enum || []).join(", "), (e) =>
        onPatch({
          enum: e.target.value
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean)
            .map((x) => coerceVal(t, x)),
        }),
        "active, inactive, pending",
      )}
      ${t === "string"
        ? html`${txt("Pattern (regex)", sc.pattern, (e) => onPatch({ pattern: e.target.value }), "^[a-z]+$")}
            <div class="row">${numF("Min length", "minLength", sc.minLength)}${numF("Max length", "maxLength", sc.maxLength)}</div>`
        : ""}
      ${t === "integer" || t === "number"
        ? html`<div class="row">${numF("Minimum", "minimum", sc.minimum)}${numF("Maximum", "maximum", sc.maximum)}${numF("Multiple of", "multipleOf", sc.multipleOf)}</div>
            <div class="row">${chk("Exclusive min", "exclusiveMinimum", sc.exclusiveMinimum)}${chk("Exclusive max", "exclusiveMaximum", sc.exclusiveMaximum)}</div>`
        : ""}
      ${t === "array"
        ? html`<div class="schema-items">
            <div class="schema-adv-label">Items</div>
            <div class="row">
              <div class="field">
                <label>Item type</label>
                <select class="input" value=${sc.items?.type || ""} onChange=${(e) => setItems({ type: e.target.value })}>
                  ${SCHEMA_TYPES.map((o) => html`<option value=${o}>${o || "—"}</option>`)}
                </select>
              </div>
              ${txt("Item format", sc.items?.format, (e) => setItems({ format: e.target.value }))}
            </div>
            ${txt("Item $ref", sc.items?.["$ref"], (e) => setItems({ ["$ref"]: e.target.value }), "#/components/schemas/Tag")}
            <div class="row">${numF("Min items", "minItems", sc.minItems)}${numF("Max items", "maxItems", sc.maxItems)}</div>
            ${chk("Unique items", "uniqueItems", sc.uniqueItems)}
          </div>`
        : ""}
      ${t === "object" ? PropsEditor(sc, onPatch) : ""}
      <div class="row">${chk("Deprecated", "deprecated", sc.deprecated)}${chk("Read only", "readOnly", sc.readOnly)}${chk("Write only", "writeOnly", sc.writeOnly)}</div>
    </div>`}
  `;
}

// PropsEditor edits an object schema's named `properties` (each itself a full
// schema, so objects/arrays nest) plus the `required` list and
// `additionalProperties`. Rendered inside SchemaEditor's object branch.
function PropsEditor(sc, onPatch) {
  const props = sc.properties || {};
  const required = sc.required || [];
  const addProp = () => {
    let name = "field",
      n = 1;
    while (props[name]) name = `field${++n}`;
    onPatch({ properties: { ...props, [name]: { type: "string" } } });
  };
  const renameProp = (oldK, raw) => {
    const newK = (raw || "").trim();
    if (!newK || newK === oldK || props[newK]) return; // ignore empty / dup
    const next = {};
    for (const [k, v] of Object.entries(props)) next[k === oldK ? newK : k] = v;
    onPatch({ properties: next, required: required.map((r) => (r === oldK ? newK : r)) });
  };
  const setPropSchema = (k, v) => onPatch({ properties: { ...props, [k]: v } });
  const delProp = (k) => {
    const next = { ...props };
    delete next[k];
    onPatch({ properties: Object.keys(next).length ? next : "", required: required.filter((r) => r !== k) });
  };
  const toggleReq = (k, on) => onPatch({ required: on ? [...new Set([...required, k])] : required.filter((r) => r !== k) });
  const numF = (label, key, val) => html`<div class="field">
    <label>${label}</label>
    <input class="input" type="number" value=${val ?? ""} onInput=${(e) => onPatch({ [key]: e.target.value === "" ? "" : Number(e.target.value) })} />
  </div>`;

  return html`<div class="schema-items">
    <div class="schema-adv-label">Properties</div>
    ${Object.entries(props).map(
      ([name, ps]) => html`
        <div class="card" style="margin:6px 0">
          <div class="row">
            <div class="field" style="flex:1">
              <label>Property name</label>
              <input class="input" value=${name} placeholder="fieldName" onChange=${(e) => renameProp(name, e.target.value)} />
            </div>
            <label class="row" style="cursor:pointer;gap:4px;align-self:flex-end;margin-bottom:8px;white-space:nowrap">
              <input type="checkbox" checked=${required.includes(name)} onChange=${(e) => toggleReq(name, e.target.checked)} /><span>required</span>
            </label>
            ${RowDelete(() => delProp(name))}
          </div>
          <${SchemaEditor} schema=${ps} onPatch=${(p) => setPropSchema(name, applySchemaPatch(ps, p))} />
        </div>
      `,
    )}
    <button class="btn ghost sm" onClick=${addProp}>+ Add property</button>
    <label class="row" style="cursor:pointer;gap:6px;margin:8px 0 0">
      <input type="checkbox" checked=${!!sc.additionalProperties} onChange=${(e) => onPatch({ additionalProperties: e.target.checked ? {} : "" })} />
      <span>Allow additional properties</span>
    </label>
    <div class="row">${numF("Min properties", "minProperties", sc.minProperties)}${numF("Max properties", "maxProperties", sc.maxProperties)}</div>
  </div>`;
}

function Section({ title, hint, help, desc, children, openDefault = false }) {
  const { query, bulk } = useContext(ConfigUI);
  const [open, setOpen] = useState(openDefault);

  // Apply an Expand-all / Collapse-all signal (bump bulk.n to broadcast).
  useEffect(() => {
    if (bulk && bulk.n > 0) setOpen(bulk.open);
  }, [bulk && bulk.n]);

  // Filtering: hide non-matching sections and auto-expand matches so the
  // result is immediately readable. Matches title, help and description.
  const q = (query || "").trim().toLowerCase();
  if (q && !(title + " " + (help || "") + " " + (desc || "")).toLowerCase().includes(q)) return "";
  const isOpen = q ? true : open;

  const hasCount = hint !== undefined && hint !== null && hint !== "";
  return html`
    <div class=${"section" + (isOpen ? " open" : "")}>
      <div class="section-head" onClick=${() => setOpen((o) => !o)}>
        <span class="chev">▸</span><span>${title}</span>
        ${help && html`<${Info} text=${help} />`}
        <span class="spacer"></span>
        ${hasCount ? html`<span class="sec-count">${hint}</span>` : ""}
      </div>
      ${isOpen &&
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

  // Config toolbar: live filter + expand/collapse-all (shared via ConfigUI).
  const [query, setQuery] = useState("");
  const [bulk, setBulk] = useState({ n: 0, open: false });
  const expandAll = (open) => setBulk((b) => ({ n: b.n + 1, open }));

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
    <${ConfigUI.Provider} value=${{ query, bulk }}>
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

      <div class="content">
        <div class="config-toolbar pad">
          <span class="ct-search">
            <span class="ct-ico">⌕</span>
            <input class="input" placeholder="Filter settings…" value=${query} onInput=${(e) => setQuery(e.target.value)} />
            ${query && html`<button class="ct-clear" title="Clear filter" onClick=${() => setQuery("")}>✕</button>`}
          </span>
          <span class="spacer"></span>
          <button class="btn ghost sm" title="Expand all sections" onClick=${() => expandAll(true)}>⊞ Expand all</button>
          <button class="btn ghost sm" title="Collapse all sections" onClick=${() => expandAll(false)}>⊟ Collapse all</button>
        </div>
        <div style="width:100%">
          <${Section} title="API information" help="Document metadata shown at the top of the spec and docs UI — the API title, version and description. Example: title 'User Service API', version '1.0.0'. The description supports Markdown (headings, lists, links)." openDefault=${true}>
            ${txt("Title", c.info?.title, (e) => setInfo({ title: e.target.value }))}
            ${txt("Version", c.info?.version, (e) => setInfo({ version: e.target.value }))}
            ${area("Description", c.info?.description, (e) => setInfo({ description: e.target.value }))}
          <//>

          <${Section} title="External docs" help="An optional link to documentation hosted elsewhere (e.g. your developer portal or a guide). Renders as a 'Find out more' link in Swagger/Redoc. Example: URL https://docs.example.com, description 'Full developer guide'.">

            ${txt("URL", c.externalDocs?.url, (e) => setExtDocs({ url: e.target.value }), "https://…")}
            ${txt("Description", c.externalDocs?.description, (e) => setExtDocs({ description: e.target.value }))}
          <//>

          <${Section} title="Servers" help="Base URLs the API is served from, shown in the Servers dropdown of Swagger/Redoc/Scalar. List one per environment, e.g. https://api.example.com (Production) and http://localhost:8080 (Local). URLs may use variables like https://{host}:{port}/v1." hint=${`${(c.servers || []).length}`}>
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

          <${Section} title="Tags" help="Named groups that organise operations into sections in the docs UI. Routes are usually tagged automatically from their mount/group prefix; add tags here to give them an order and a description. Example: name 'Users', description 'Account and profile endpoints'." hint=${`${(c.tags || []).length}`}>
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

          <${Section} title="Defaults" help="Fallback content-types and status code used when a handler doesn't make them explicit in code. Typical values: request & response content-type application/json, default response status 200. A per-pattern 'Default content-type' or an override can still take precedence.">

            ${txt("Request content-type", c.defaults?.requestContentType, (e) => setDefaults({ requestContentType: e.target.value }), "application/json")}
            ${txt("Response content-type", c.defaults?.responseContentType, (e) => setDefaults({ responseContentType: e.target.value }), "application/json")}
            ${txt("Default response status", c.defaults?.responseStatus, (e) => setDefaults({ responseStatus: parseInt(e.target.value, 10) || 0 }), "200")}
          <//>

          <${Section} title="Include / exclude filters" help="Scope the analysis: include limits it to the listed packages/files/functions/types, exclude removes them (exclude wins). One entry per line, glob-style. Tests and mocks are auto-excluded already. Examples — exclude files: **/*_test.go ; exclude packages: github.com/me/api/internal/mocks ; include packages: github.com/me/api/handlers (narrow a huge repo to just the HTTP layer to speed up generation).">

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

          <${Section} title="Type mappings" help="Map a Go type to an explicit OpenAPI schema for every occurrence. Use when apispec CAN see the type but you want a specific representation. The editor exposes the full OpenAPI vocabulary — type/format, enum, pattern, min/max & length, array items, additionalProperties, $ref, example, deprecated. Examples: time.Time → string/date-time · uuid.UUID → string/uuid · domain.UserStatus → string with enum [active, inactive, pending] · Money → $ref #/components/schemas/Money." hint=${`${(c.typeMapping || []).length}`}>
            ${(c.typeMapping || []).map(
              (m, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Mapping ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("typeMapping", i))}
                  </div>
                  ${txt("Go type", m.goType, (e) => updAt("typeMapping", i, { goType: e.target.value }), "uuid.UUID")}
                  <${SchemaEditor} schema=${m.openapiType} onPatch=${(p) => updAt("typeMapping", i, { openapiType: applySchemaPatch(m.openapiType, p) })} />
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("typeMapping", { goType: "", openapiType: { type: "string" } })}>+ Add mapping</button>
          <//>

          <${Section} title="External types" help="Like type mappings, but for types apispec can't see the source of (third-party / opaque). The fix for an 'unresolved/external placeholder type' on a NON-module type — in-module types should resolve automatically, so investigate those instead. Examples: gin.H → object (additionalProperties: true) · primitive.ObjectID → string · shared.Response → object with properties {code: integer, message: string, data: object}." hint=${`${(c.externalTypes || []).length}`}>
            ${(c.externalTypes || []).map(
              (m, i) => html`
                <div class="card">
                  <div class="row">
                    <strong style="font-size:var(--fs-sm)">Type ${i + 1}</strong>
                    <span class="spacer"></span>${RowDelete(() => delAt("externalTypes", i))}
                  </div>
                  ${txt("Name", m.name, (e) => updAt("externalTypes", i, { name: e.target.value }), "primitive.ObjectID")}
                  <${SchemaEditor} schema=${m.openapiType} onPatch=${(p) => updAt("externalTypes", i, { openapiType: applySchemaPatch(m.openapiType, p) })} />
                </div>
              `,
            )}
            <button class="btn secondary sm" onClick=${() => addTo("externalTypes", { name: "", openapiType: { type: "string" } })}>+ Add external type</button>
          <//>

          <${Section} title="Overrides" help="Per-handler escape hatch when inference is wrong or incomplete: force a summary, response status/type or tags. Matched by fully-qualified function name. Example: function github.com/me/api/handlers.GetUser → summary 'Fetch a user by ID', response status 200, response type github.com/me/api/models.User, tags Users." hint=${`${(c.overrides || []).length}`}>
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

          <${Section} title="Security schemes" help="Define auth schemes that operations reference; the active scheme becomes the Authorize button in the docs UI. Examples: a JWT 'bearerAuth' (type http, scheme bearer, bearerFormat JWT), an 'apiKeyAuth' (type apiKey, in header, name X-API-Key), or oauth2. Reference them from the top-level Security requirement." hint=${`${Object.keys(c.securitySchemes || {}).length}`}>
            <${SecuritySchemes} c=${c} />
          <//>

          <${Section} title="Security mappings" help="Map auth middleware to a security scheme so apispec can mark the routes it guards as protected. Each mapping matches the middleware by function name / package / receiver-type regex and applies the chosen scheme. Most well-known libraries (echo-jwt, gin-jwt, gofiber/contrib/jwt, golang-jwt, …) and custom wrappers around them are detected automatically; use these mappings for project-specific middleware that apispec couldn't resolve (see 'Unresolved auth middleware' after generating)." hint=${`${(c.securityMappings || []).length}`}>
            <${UnresolvedMiddleware} c=${c} />
            <${SecurityMappings} c=${c} />
          <//>

          <div class="section-group">Detection — how routes &amp; types are discovered</div>

          <${Section} title="Routes" help="How route registrations are recognised, and where the method, path and handler are read from. Each pattern matches a call by the called name (Call regex) and its receiver type (Receiver type regex). Example (Gin): r.GET('/users/{id}', h) — Call regex ^(?i)(GET|POST|PUT|DELETE|PATCH)$, receiver ^.*gin\\.\\*(Engine|RouterGroup)$, method from the call name, path from arg 0, handler from arg 1." hint=${`${(fc.routePatterns || []).length}`}>
            <${PatternList} items=${fc.routePatterns} fields=${PATTERN_FIELDS.routePatterns} onChange=${(a) => setFC("routePatterns", a)} />
          <//>
          <${Section} title="Request body" help="How request-body decoding is recognised and which argument's type becomes the body schema. Example (Gin): c.ShouldBindJSON(&req) — Call regex ^(?i)(ShouldBind|BindJSON|ShouldBindJSON)$, 'Type from arg' on, 'Dereference pointer' on (strips the * from *req). For generic decoders (json.Unmarshal, render.DecodeJSON) also turn on 'Require request source' and configure Request context below." hint=${`${(fc.requestBodyPatterns || []).length}`}>
            <${PatternList} items=${fc.requestBodyPatterns} fields=${PATTERN_FIELDS.requestBodyPatterns} onChange=${(a) => setFC("requestBodyPatterns", a)} />
          <//>
          <${Section} title="Responses" help="How response writes are recognised, and where the status code and body type are read from. Example (Gin): c.JSON(200, user) — Call regex ^(?i)(JSON|XML|YAML|ProtoBuf)$, 'Status from arg' index 0, 'Type from arg' index 1. Use 'Default status' for writers without an explicit code (e.g. 200), and 'Default content-type' to override per pattern." hint=${`${(fc.responsePatterns || []).length}`}>
            <${PatternList} items=${fc.responsePatterns} fields=${PATTERN_FIELDS.responsePatterns} onChange=${(a) => setFC("responsePatterns", a)} />
          <//>
          <${Section} title="Parameters" help="How path/query/header/cookie/form parameter reads are recognised. The 'Parameter location' sets where it appears in the spec. Examples (Gin): c.Param('id') → location path · c.Query('q') → location query · c.GetHeader('X-Token') → location header. The parameter name is read from the named-argument index." hint=${`${(fc.paramPatterns || []).length}`}>
            <${PatternList} items=${fc.paramPatterns} fields=${PATTERN_FIELDS.paramPatterns} onChange=${(a) => setFC("paramPatterns", a)} />
          <//>
          <${Section} title="Mounts / groups" help="How sub-router mounts/groups are recognised so nested routes inherit the right path prefix. Examples: Chi r.Mount('/api', sub) or r.Route('/v1', fn) · Gin r.Group('/v1'). Set 'Path from arg' (the prefix) and 'Router from arg' (the sub-router being mounted) and mark 'Is mount'." hint=${`${(fc.mountPatterns || []).length}`}>
            <${PatternList} items=${fc.mountPatterns} fields=${PATTERN_FIELDS.mountPatterns} onChange=${(a) => setFC("mountPatterns", a)} />
          <//>
          <${Section} title="Request context" help="Disambiguates generic decoders. json.Decode / json.Unmarshal / render.DecodeJSON decode request bodies AND unrelated data (config files, internal payloads). A decoder counts as a request body only when its source traces back to a body accessor on a request-context value. Type regexes = the request types to watch (e.g. ^\\*?net/http\\.Request$, ^.*gin\\.\\*Context$). Body accessors = methods that yield the body (e.g. ^Body$, ^GetRawData$). Leave empty to fall back to receiver-only matching.">
            <${RequestContextEditor} rc=${fc.requestContext} onChange=${(v) => setFC("requestContext", v)} />
          <//>
        </div>
      </div>
    </div>
    <//>
  `;
}

/* ---- framework detection pattern editors --------------------------- */

const COMMON_MATCH = [
  ["callRegex", "Call regex", "text", "Regex matching the called method/function name. Anchor with ^…$ and use (?i) for case-insensitive. e.g. ^(?i)(GET|POST|PUT|DELETE)$ matches Gin's verb methods; ^ShouldBindJSON$ matches one decoder."],
  ["recvTypeRegex", "Receiver type regex", "text", "Regex matching the fully-qualified receiver/owner type of the call. e.g. ^.*gin\\.\\*(Engine|RouterGroup)$ (Gin), ^github\\.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$ (Chi), ^.*echo(/v\\d)?\\.(Echo|Group)$ (Echo). Leave blank to match any receiver."],
  ["functionNameRegex", "Function name regex", "text", "Optional — match by the ENCLOSING function's name instead of the call (e.g. only routes registered inside RegisterRoutes)."],
];

const PATTERN_FIELDS = {
  routePatterns: [
    ...COMMON_MATCH,
    ["handlerArgIndex", "Handler arg index", "int", "0-based position of the handler argument. e.g. in r.GET(path, handler) the handler is index 1."],
    ["pathArgIndex", "Path arg index", "int", "0-based position of the path argument. e.g. in r.GET(path, handler) the path is index 0."],
    ["methodArgIndex", "Method arg index", "int", "0-based position of the HTTP-method argument, for routers that take the method as a value. e.g. r.Handle(method, path, h) → 0."],
    ["methodFromCall", "Method from call name", "bool", "Derive the HTTP method from the called function name (e.g. GET())."],
    ["methodFromHandler", "Method from handler", "bool", "Derive the method from the handler function name."],
    ["pathFromArg", "Path from arg", "bool", "Read the path from the path argument."],
    ["handlerFromArg", "Handler from arg", "bool", "Resolve the handler from the handler argument."],
  ],
  requestBodyPatterns: [
    ...COMMON_MATCH,
    ["typeArgIndex", "Type arg index", "int", "Index of the argument whose type is the request body. e.g. render.DecodeJSON(r, &req) → 1 (combine with Dereference)."],
    ["typeFromArg", "Type from arg", "bool", "Use the matched argument's type as the body schema. e.g. c.ShouldBindJSON(&req) → uses *req's type."],
    ["typeFromReturn", "Type from return", "bool", "Use the call's RETURN type as the body type instead of an argument."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type, so *CreateUserRequest becomes CreateUserRequest. Usually on for &req decoders."],
    ["requireRequestSource", "Require request source", "bool", "Only classify as a request body when the value traces back to the HTTP request — prevents config/file decoders (json.Unmarshal of a file) from being mistaken for body decoders. Pair with Request context below."],
    ["bodyFromReceiver", "Body from receiver", "bool", "The body comes from the call RECEIVER, not an argument — e.g. a json.Decoder bound to r.Body: dec.Decode(&req)."],
    ["bodySourceArgIndex", "Body source arg index", "int", "Index of the argument carrying the request-body source (the io.Reader/request), used with Require request source."],
    ["allowForGetMethods", "Allow for GET/HEAD", "bool", "Permit a request body on GET/HEAD routes (off by default, since those rarely carry one)."],
  ],
  responsePatterns: [
    ...COMMON_MATCH,
    ["statusArgIndex", "Status arg index", "int", "Index of the argument holding the HTTP status code. e.g. c.JSON(200, body) → 0."],
    ["typeArgIndex", "Type arg index", "int", "Index of the argument whose type is the response body. e.g. c.JSON(200, body) → 1."],
    ["statusFromArg", "Status from arg", "bool", "Read the status code from the status argument (vs using Default status)."],
    ["typeFromArg", "Type from arg", "bool", "Use the matched argument's type as the response schema."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type (e.g. *User → User)."],
    ["defaultStatus", "Default status", "int", "Status used when the writer has no explicit code. e.g. 200 for c.JSON-style writers, 204 for an empty response."],
    ["defaultContentType", "Default content-type", "text", "Content-type for this pattern when not otherwise known. e.g. text/plain; charset=utf-8 for a c.String writer."],
  ],
  paramPatterns: [
    ...COMMON_MATCH,
    ["paramIn", "Parameter location", "select:path,query,header,cookie,form", "Where this parameter appears in the spec. e.g. c.Param→path, c.Query→query, c.GetHeader→header, c.Cookie→cookie, c.PostForm→form."],
    ["paramArgIndex", "Param arg index", "int", "Index of the argument holding the parameter NAME. e.g. c.Query('q') → 0."],
    ["typeArgIndex", "Type arg index", "int", "Index of the argument whose type is the parameter type (for typed getters)."],
    ["typeFromArg", "Type from arg", "bool", "Use the matched argument's type as the parameter type (otherwise defaults to string)."],
    ["deref", "Dereference pointer", "bool", "Strip a leading * from the resolved type."],
  ],
  mountPatterns: [
    ...COMMON_MATCH,
    ["pathArgIndex", "Path arg index", "int", "Index of the mount path/prefix argument. e.g. r.Mount('/api', sub) → 0."],
    ["routerArgIndex", "Router arg index", "int", "Index of the sub-router argument. e.g. r.Mount('/api', sub) → 1."],
    ["pathFromArg", "Path from arg", "bool", "Read the prefix from the path argument and prepend it to nested routes."],
    ["routerFromArg", "Router from arg", "bool", "Follow the sub-router argument so its routes are attached under the prefix."],
    ["isMount", "Is mount", "bool", "Treat this call as a router mount/group (e.g. Chi Mount/Route, Gin Group)."],
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

// escapeRe escapes regex metacharacters so an identity string (pkg path,
// function name) can be used as an anchored exact-match regex.
function escapeRe(s) {
  return (s || "").replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// defaultSchemeFor returns a sensible securityScheme definition for a well-known
// scheme name, so picking it auto-creates a usable scheme.
function defaultSchemeFor(name) {
  switch (name) {
    case "basicAuth":
      return { type: "http", scheme: "basic" };
    case "apiKeyAuth":
      return { type: "apiKey", in: "header", name: "Authorization" };
    default:
      return { type: "http", scheme: "bearer", bearerFormat: "JWT" };
  }
}

// firstSchemeName extracts the scheme name from a mapping's first requirement.
function firstSchemeName(m) {
  const req = (m.schemes && m.schemes[0]) || {};
  return Object.keys(req)[0] || "";
}

// SecurityMappings edits the middleware-identity -> scheme mappings.
function SecurityMappings({ c }) {
  const maps = c.securityMappings || [];
  const set = (arr) => setConfig({ securityMappings: arr });
  const upd = (i, patch) => set(maps.map((m, j) => (j === i ? { ...m, ...patch } : m)));
  const del = (i) => set(maps.filter((_, j) => j !== i));
  const setScheme = (i, name) => upd(i, { schemes: [{ [name]: [] }] });
  const add = () =>
    set([...maps, { functionNameRegex: "", pkgRegex: "", recvTypeRegex: "", schemes: [{ bearerAuth: [] }] }]);
  const schemeNames = Object.keys(c.securitySchemes || {});
  return html`
    ${maps.map(
      (m, i) => html`
        <div class="card">
          <div class="row">
            <strong style="font-size:var(--fs-sm)">Mapping ${i + 1}</strong>
            <span class="spacer"></span>${RowDelete(() => del(i))}
          </div>
          ${txt("Function name regex", m.functionNameRegex, (e) => upd(i, { functionNameRegex: e.target.value }), "^authMiddleware$")}
          <div class="row">
            ${txt("Package regex", m.pkgRegex, (e) => upd(i, { pkgRegex: e.target.value }), "github\\.com/me/.*")}
            ${txt("Receiver type regex", m.recvTypeRegex, (e) => upd(i, { recvTypeRegex: e.target.value }), "Handler")}
          </div>
          <label class="lbl">Scheme</label>
          <input class="input" list="known-schemes" value=${firstSchemeName(m)} onChange=${(e) => setScheme(i, e.target.value.trim())} placeholder="bearerAuth" />
        </div>
      `,
    )}
    <datalist id="known-schemes">
      ${[...new Set([...schemeNames, "bearerAuth", "basicAuth", "apiKeyAuth"])].map((n) => html`<option value=${n}></option>`)}
    </datalist>
    <button class="btn secondary sm" onClick=${add}>+ Add mapping</button>
  `;
}

// UnresolvedMiddleware lists auth middleware that the last generation detected
// but could not map to a scheme, with a one-click picker to map each.
function UnresolvedMiddleware({ c }) {
  const s = useStore();
  const items = s.unresolvedSecurity || [];
  if (!items.length) return null;

  const mapOne = (mw, schemeName) => {
    schemeName = (schemeName || "bearerAuth").trim() || "bearerAuth";
    // Ensure the scheme exists.
    const schemes = { ...(c.securitySchemes || {}) };
    if (!schemes[schemeName]) schemes[schemeName] = defaultSchemeFor(schemeName);
    // Build an anchored mapping from the middleware identity.
    const m = { schemes: [{ [schemeName]: [] }] };
    if (mw.functionName) m.functionNameRegex = "^" + escapeRe(mw.functionName) + "$";
    if (mw.pkg) m.pkgRegex = "^" + escapeRe(mw.pkg) + "$";
    if (mw.recvType) m.recvTypeRegex = "^" + escapeRe(mw.recvType) + "$";
    setConfig({ securitySchemes: schemes, securityMappings: [...(c.securityMappings || []), m] });
    // Drop it from the pending list immediately; a regenerate refreshes the rest.
    const key = (x) => `${x.pkg}.${x.recvType}.${x.functionName}`;
    setState({ unresolvedSecurity: items.filter((x) => key(x) !== key(mw)) });
  };

  const label = (mw) =>
    (mw.pkg ? mw.pkg + "." : "") + (mw.recvType ? "(" + mw.recvType + ")." : "") + (mw.functionName || "?");

  return html`
    <div class="card" style="border-color:var(--warn,#b80)">
      <div class="row">
        <strong style="font-size:var(--fs-sm)">Unresolved auth middleware (${items.length})</strong>
      </div>
      <div class="muted" style="font-size:var(--fs-sm);margin:4px 0 8px">
        Detected on routes but not matched to a scheme. Pick a scheme to map each, then re-generate.
      </div>
      ${items.map(
        (mw) => html`
          <div class="row" style="align-items:center;gap:8px;margin-bottom:6px">
            <code style="flex:1;overflow:auto">${label(mw)}</code>
            <input class="input" style="max-width:140px" list="known-schemes" placeholder="bearerAuth"
              onKeyDown=${(e) => { if (e.key === "Enter") mapOne(mw, e.target.value); }} />
            <button class="btn secondary sm" onClick=${(e) => mapOne(mw, e.target.previousElementSibling.value)}>Map</button>
          </div>
        `,
      )}
      <button class="btn sm" onClick=${() => generate()}>Re-generate with mappings</button>
    </div>
  `;
}
