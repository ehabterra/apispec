#!/usr/bin/env python3
"""
Generate the title card, outro card, section covers, terminal cards and
lower-third captions as PNGs (transparent where needed) via SVG -> rsvg-convert.

Why SVG: this ffmpeg build has no `drawtext`/libass, so we can't burn text
directly. Instead we render crisp PNGs here and `overlay` them in build.sh.

Edit the lists at the bottom, then:  python3 gen_assets.py
Outputs land in ./assets/*.png   (1920x1080 each)
"""
import subprocess, html, os

OUT = os.path.join(os.path.dirname(__file__), "assets")
os.makedirs(OUT, exist_ok=True)

# --- theme (matches apispec's dark UI) ---
ACC, ACC2 = "#58a6ff", "#3fb950"          # accent blue, green
TXT, MUT, STROKE = "#e6edf3", "#9fb0c0", "#30363d"
PANEL = "#0e1116"
FONT = "Helvetica, Arial, sans-serif"
MONO = "Menlo, 'SF Mono', Consolas, monospace"
GRAD = ('<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1">'
        '<stop offset="0" stop-color="#0b0e13"/><stop offset="1" stop-color="#141c27"/>'
        '</linearGradient></defs><rect width="1920" height="1080" fill="url(#g)"/>')


def render(name, body):
    svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="1920" height="1080" '
           f'viewBox="0 0 1920 1080">{body}</svg>')
    open(f"{OUT}/{name}.svg", "w").write(svg)
    subprocess.run(["rsvg-convert", "-w", "1920", "-h", "1080",
                    f"{OUT}/{name}.svg", "-o", f"{OUT}/{name}.png"], check=True)


def title(name, big, sub):
    render(name, GRAD + f'''
<rect x="828" y="612" width="264" height="6" rx="3" fill="{ACC}"/>
<text x="960" y="500" font-family="{FONT}" font-size="150" font-weight="700" fill="{ACC}" text-anchor="middle">{html.escape(big)}</text>
<text x="960" y="585" font-family="{FONT}" font-size="46" fill="{TXT}" text-anchor="middle">{html.escape(sub)}</text>''')


def cover(name, num, label):          # section divider
    render(name, GRAD + f'''
<text x="960" y="470" font-family="{FONT}" font-size="40" font-weight="700" letter-spacing="6" fill="{ACC}" text-anchor="middle">STEP {num}</text>
<text x="960" y="585" font-family="{FONT}" font-size="104" font-weight="700" fill="{TXT}" text-anchor="middle">{html.escape(label)}</text>
<rect x="860" y="628" width="200" height="6" rx="3" fill="{ACC}"/>''')


def terminal(name, title_bar, lines):  # lines: list of (prompt, text, color)
    rows, y = "", 470
    for pr, txt, col in lines:
        rows += f'<text x="372" y="{y}" font-family="{MONO}" font-size="34" fill="{MUT}">{html.escape(pr)}</text>'
        rows += f'<text x="410" y="{y}" font-family="{MONO}" font-size="34" fill="{col}">{html.escape(txt)}</text>'
        y += 62
    render(name, GRAD + f'''
<rect x="310" y="330" width="1300" height="420" rx="16" fill="#0d1117" stroke="{STROKE}"/>
<rect x="310" y="330" width="1300" height="46" rx="16" fill="#161b22"/>
<rect x="310" y="360" width="1300" height="16" fill="#161b22"/>
<circle cx="346" cy="353" r="8" fill="#ff5f56"/><circle cx="374" cy="353" r="8" fill="#ffbd2e"/><circle cx="402" cy="353" r="8" fill="#27c93f"/>
<text x="960" y="360" font-family="{FONT}" font-size="24" fill="{MUT}" text-anchor="middle">{html.escape(title_bar)}</text>
{rows}''')


def lower(name, text):                 # lower-third caption (transparent canvas)
    render(name, f'''<svg xmlns="http://www.w3.org/2000/svg" width="1920" height="1080" viewBox="0 0 1920 1080">
<rect x="360" y="946" width="1200" height="104" rx="20" fill="{PANEL}" fill-opacity="0.86" stroke="{STROKE}"/>
<rect x="360" y="946" width="10" height="104" rx="5" fill="{ACC}"/>
<text x="968" y="1006" font-family="{FONT}" font-size="44" font-weight="600" fill="{TXT}" text-anchor="middle">{html.escape(text)}</text></svg>''')


# ---------- vertical 9:16 (1080x1920) variants for the short, build_short.sh ----------
# Same theme, portrait canvas. Footage sits in a centred 1080x608 band; hooks go
# above it (top), captions below it (bottom), so neither covers the demo.
WV, HV = 1080, 1920
GRADV = ('<defs><linearGradient id="gv" x1="0" y1="0" x2="0" y2="1">'
         '<stop offset="0" stop-color="#0b0e13"/><stop offset="1" stop-color="#141c27"/>'
         '</linearGradient></defs><rect width="1080" height="1920" fill="url(#gv)"/>')


def render_v(name, body):
    svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{WV}" height="{HV}" '
           f'viewBox="0 0 {WV} {HV}">{body}</svg>')
    open(f"{OUT}/{name}.svg", "w").write(svg)
    subprocess.run(["rsvg-convert", "-w", str(WV), "-h", str(HV),
                    f"{OUT}/{name}.svg", "-o", f"{OUT}/{name}.png"], check=True)


def _wrap(text, n):                    # greedy word-wrap to <= n chars/line
    out, cur = [], ""
    for w in text.split():
        if not cur or len(cur) + 1 + len(w) <= n:
            cur = (cur + " " + w).strip()
        else:
            out.append(cur); cur = w
    if cur: out.append(cur)
    return out


def _block(cx, top, h, size, weight, fill, lines):   # vertically-centred text block
    dy = round(size * 1.2)
    base = top + (h - ((len(lines) - 1) * dy + size)) / 2 + size * 0.78
    return "".join(
        f'<text x="{cx}" y="{base + i*dy:.0f}" font-family="{FONT}" font-size="{size}" '
        f'font-weight="{weight}" fill="{fill}" text-anchor="middle">{html.escape(t)}</text>'
        for i, t in enumerate(lines))


def _panel_v(name, top, lines, size, accent_left):   # rounded panel + centred text
    pad, dy = 34, round(size * 1.2)
    h = 2 * pad + (len(lines) - 1) * dy + size
    bar = (f'<rect x="60" y="{top}" width="12" height="{h}" rx="6" fill="{ACC}"/>' if accent_left
           else f'<rect x="60" y="{top}" width="960" height="8" rx="4" fill="{ACC}"/>')
    render_v(name,
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{WV}" height="{HV}" viewBox="0 0 {WV} {HV}">'
        f'<rect x="60" y="{top}" width="960" height="{h}" rx="22" fill="{PANEL}" fill-opacity="0.92" stroke="{STROKE}"/>'
        + bar + _block(540, top, h, size, 600, TXT, lines) + '</svg>')


def title_v(name, big, sub):           # opening card
    render_v(name, GRADV
        + f'<text x="540" y="840" font-family="{FONT}" font-size="120" font-weight="700" fill="{ACC}" text-anchor="middle">{html.escape(big)}</text>'
        + f'<rect x="420" y="912" width="240" height="6" rx="3" fill="{ACC}"/>'
        + _block(540, 950, 210, 42, 400, TXT, _wrap(sub, 24)))


def outro_v(name, big, sub):           # closing card
    render_v(name, GRADV
        + _block(540, 740, 380, 72, 700, TXT, _wrap(big, 16))
        + f'<text x="540" y="1210" font-family="{FONT}" font-size="40" fill="{ACC}" text-anchor="middle">{html.escape(sub)}</text>')


def hook_v(name, text):                # persistent top band over footage (transparent)
    _panel_v(name, 210, _wrap(text, 22), 48, accent_left=False)


def cap_v(name, text):                 # lower-third caption for vertical (transparent)
    _panel_v(name, 1360, _wrap(text, 26), 42, accent_left=True)


# ============== EDIT BELOW ==============
title("title", "apispec", "OpenAPI 3.1 from your Go source — no annotations")
title("outro", "Generate OpenAPI from Go — try it", "★  github.com/ehabterra/apispec")  # or customise

cover("cov_install",   "01", "Install")
cover("cov_run",       "02", "Run")
cover("cov_generate",  "03", "Generate")
cover("cov_explore",   "04", "Explore the spec")
cover("cov_configure", "05", "Configure")
cover("cov_insight",   "06", "Insight")

terminal("card_install", "zsh — install", [
    ("$", "go install github.com/ehabterra/apispec/cmd/apispecui@latest", TXT),
    ("", "", TXT),
    ("$", "export PATH=$HOME/go/bin:$PATH", TXT),
    ("#", "installs the apispecui binary", MUT),
])
terminal("card_run", "zsh — run", [
    ("$", "apispecui --dir ./my-project", TXT),
    ("", "", TXT),
    ("▸", "serving on  http://localhost:8088", ACC2),
    ("▸", "open it in your browser", MUT),
])

lower("cap_generate",  "Generate from real code — 13 paths in ~1s")
lower("cap_explore",   "Request & response schemas — inferred, not annotated")
lower("cap_configure", "Tune route & type detection in the browser")
lower("cap_insight",   "Insight: trace every endpoint's call path")

# --- vertical 9:16 short (used by build_short.sh) ---
title_v("title_v", "apispec", "OpenAPI 3.1 from your Go source")
outro_v("outro_v", "Generate OpenAPI from Go", "github.com/ehabterra/apispec")
hook_v("hook", "Go → OpenAPI 3.1, no annotations")   # persistent top band
cap_v("capv_generate", "13 paths generated in ~1s")
cap_v("capv_explore",  "Schemas inferred, not annotated")
cap_v("capv_insight",  "Trace every endpoint's call path")
# ========================================
print("assets ->", OUT)
