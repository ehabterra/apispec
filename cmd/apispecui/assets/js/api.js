// api.js — thin fetch wrappers around the apispecui server endpoints.

async function ok(r) {
  if (!r.ok) {
    const t = await r.text().catch(() => "");
    throw new Error(t || `${r.status} ${r.statusText}`);
  }
  return r;
}

function qs(obj) {
  const parts = Object.entries(obj)
    .filter(([, v]) => v !== undefined && v !== null && v !== "")
    .map(([k, v]) => `${k}=${encodeURIComponent(v)}`);
  return parts.length ? `?${parts.join("&")}` : "";
}

export async function getJSON(url) {
  return (await ok(await fetch(url))).json();
}
export async function getText(url) {
  return (await ok(await fetch(url))).text();
}
export async function postJSON(url, body) {
  return (
    await ok(
      await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
    )
  ).json();
}

export const api = {
  detect: (dir) => getJSON("/api/detect" + qs({ dir })),
  project: (dir) => postJSON("/api/project", { dir }),
  health: () => getJSON("/api/health"),
  browse: (path, files) => getJSON("/api/browse" + qs({ path, files })),
  generate: (body) => postJSON("/api/generate", body),
  progress: () => getJSON("/api/generate/progress"),
};
