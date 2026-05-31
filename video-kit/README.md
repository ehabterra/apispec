# video-kit — make a captioned demo from a screen recording

A tiny, scriptable pipeline (no GUI editor needed) to turn a long silent screen
recording into a short, captioned, privacy-blurred, compressed demo.

## The tools

| Tool | Role | Install (macOS) |
|------|------|-----------------|
| **ffprobe** | inspect a video (duration, resolution, fps, codecs) | `brew install ffmpeg` |
| **ffmpeg** | the workhorse: cut, blur, scale, overlay text, concat, compress | `brew install ffmpeg` |
| **rsvg-convert** | render caption/cover/card **SVG → PNG** (this ffmpeg has no `drawtext`) | `brew install librsvg` |
| **QuickTime / IINA** | *watch* the recording to find exact in/out timestamps | built-in / `brew install --cask iina` |

> The text is drawn as PNGs (via SVG) and composited with `overlay`, because
> this ffmpeg build was compiled without `drawtext`/libass. Check yours with
> `ffmpeg -hide_banner -filters | grep drawtext` — if present, you can burn text
> directly and skip the PNG step.

## The workflow (the loop)

1. **Inspect:** `ffprobe -v error -show_entries format=duration:stream=width,height,r_frame_rate -of default=noprint_wrappers=1 in.mov`
2. **Find timestamps (do this by watching):** open in QuickTime, play (`space`),
   pause, step frames with `←/→`, read the time. Note each segment's **start**
   and **end**; `dur = end − start`. *This is the step only you can do well — it's
   why your cut will be more accurate than a guessed one.*
   To peek at one moment without a player: `ffmpeg -ss 92 -i in.mov -frames:v 1 peek.jpg`
3. **Make assets:** edit and run `python3 gen_assets.py` → `assets/*.png`
   (title, section covers, terminal cards, lower-third captions).
4. **Render + assemble:** put your timestamps in the `TIMELINE` block of
   `build.sh`, then `./build.sh`.
5. **Verify:** spot-check frames — `ffmpeg -ss 78 -i ../apispecui-demo.mp4 -frames:v 1 chk.jpg` — and re-run.

## Files

- `gen_assets.py` — all text PNGs. Edit the lists at the bottom (labels, captions, commands).
- `build.sh` — the `TIMELINE` is the only thing you edit per video:
  - `card <png> <dur> <fade> <name>` — a still card (title / cover / terminal).
  - `clip <start> <dur> <caption|none> <blur:0|1> <name>` — a slice of footage.

## Gotchas (these bit me)

- **Caption fade needs a looped image.** A still PNG fed to `fade` has one frame
  at t=0, so the fade drives its alpha to ~0 and it vanishes. Always feed it as
  `-loop 1 -framerate 30 -i cap.png` so it's a real stream. (build.sh does this.)
- **`concat -c copy` requires identical encodes.** Every part is rendered with
  the same `ENC` settings (codec, fps, pixfmt, timescale). Change them in *one*
  place. If you mix settings, concat with `-c copy` produces broken playback —
  re-encode at concat time instead.
- **Blur box is in SOURCE pixels.** `BX BY BW BH` are measured on the original
  frame (here 3582-wide). If you re-record at a different resolution, re-measure
  by extracting a frame and eyeballing the region to hide.
- **Seek:** `-ss <start>` *before* `-i` (fast) + `-t <dur>` (unambiguous length).
  Avoid `-to` here — its meaning shifts depending on `-ss` placement.
- **Privacy:** screen recordings leak paths, hostnames, tokens. Blur them
  (`crop+boxblur+overlay`) or cover with a card. Verify by extracting frames.

## One-liners worth keeping

```bash
# compress only (no edits): 4K/60 silent -> 1080p/30, ~10x smaller
ffmpeg -i in.mov -vf "scale=1920:1080,fps=30" -c:v libx264 -crf 22 -preset veryfast -an out.mp4

# README GIF from a slice
ffmpeg -ss 80 -t 6 -i in.mov -vf "fps=12,scale=900:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" demo.gif

# blur a region for the whole clip
ffmpeg -i in.mov -filter_complex "[0:v]crop=1582:130:2000:170,boxblur=16[b];[0:v][b]overlay=2000:170" -an out.mp4
```
