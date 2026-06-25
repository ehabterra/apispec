#!/usr/bin/env bash
# Build a vertical 9:16 SHORT (1080x1920) from the same screen recording.
#
#   1) python3 gen_assets.py          # also renders the vertical assets (title_v, hook, capv_*, outro_v)
#   2) edit SRC + the TIMELINE below  # pick the punchiest 20-50s of footage
#   3) ./build_short.sh               # reframe + caption + concat -> OUT_SHORT
#
# Reframe: footage is scaled to a centred 1080x608 band; a blurred, darkened copy
# of the same frame fills the rest of the portrait canvas (so private regions —
# already blurred below — stay blurred in the fill too). A persistent `hook` band
# sits above the footage; the per-clip caption sits below it.
#
# Requires: ffmpeg, ffprobe  (this build has NO drawtext, hence PNG overlays).
set -euo pipefail
cd "$(dirname "$0")"

# ---------------- CONFIG (shared with build.sh) ----------------
SRC="$HOME/Desktop/in.mov"  # <- replace via drag/drop or tab completion (absolute path is safest)
OUT_SHORT="../apispecui-short.mp4"                        # final vertical file
A="assets"                                                # PNGs from gen_assets.py

# Privacy blur box, in SOURCE pixels: x y w h. (Same boxes as build.sh.)
BX=2000; BY=170; BW=1582; BH=130
AX=1400; AY=1050; AW=500; AH=80
DX=1400; DY=480;  DW=500; DH=80

# Encode settings — identical for every part so `concat -c copy` works.
ENC=(-c:v libx264 -pix_fmt yuv420p -r 30 -preset veryfast -crf 21 -an -video_track_timescale 30000)
# ---------------------------------------------------------------

rm -rf parts && mkdir -p parts
PARTS=()                          # collected in order
add(){ PARTS+=("$1"); }     # register a finished part for concat

# Reframe a (possibly privacy-blurred) source stream [src] into a 1080x1920 frame:
# blurred+darkened cover behind, sharp 1080-wide footage centred on top -> [frame].
REFRAME="[src]scale=1920:1080,setsar=1,fps=30,split[bg][fg];\
[bg]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,boxblur=30,eq=brightness=-0.28:saturation=0.85[bgv];\
[fg]scale=1080:-2[fgv];\
[bgv][fgv]overlay=(W-w)/2:(H-h)/2[frame]"

# still PNG -> faded clip (title / outro).  card_v <png> <dur> <fade> <name>
card_v(){ local png=$1 dur=$2 fd=$3 name=$4
  ffmpeg -y -loglevel error -loop 1 -framerate 30 -t "$dur" -i "$A/$png.png" \
    -vf "scale=1080:1920,setsar=1,format=yuv420p,fade=t=in:st=0:d=$fd,fade=t=out:st=$(echo "$dur-$fd"|bc):d=$fd" \
    "${ENC[@]}" "parts/$name.mp4"; add "$name.mp4"; }

# footage clip, reframed to vertical.  clip_v <start> <dur> <caption|none> <blur:0|1|2|3> <name>
# The persistent `hook` band is always overlaid on top.
clip_v(){ local s=$1 d=$2 cap=$3 blur=$4 name=$5
  local fo; fo=$(echo "$d-0.4"|bc)
  local pre="[0:v]null[src]"
  [ "$blur" = 1 ] && pre="[0:v]crop=${BW}:${BH}:${BX}:${BY},boxblur=16[b];[0:v][b]overlay=${BX}:${BY}[src]"
  [ "$blur" = 2 ] && pre="[0:v]crop=${AW}:${AH}:${AX}:${AY},boxblur=16[b];[0:v][b]overlay=${AX}:${AY}[src]"
  [ "$blur" = 3 ] && pre="[0:v]crop=${DW}:${DH}:${DX}:${DY},boxblur=16[b];[0:v][b]overlay=${DX}:${DY}[src]"
  if [ "$cap" = none ]; then
    ffmpeg -y -loglevel error -ss "$s" -i "$SRC" \
      -loop 1 -framerate 30 -i "$A/hook.png" \
      -filter_complex "$pre;$REFRAME;[1:v]format=rgba[hk];[frame][hk]overlay=0:0,format=yuv420p[v]" \
      -map "[v]" -t "$d" "${ENC[@]}" "parts/$name.mp4"
  else
    ffmpeg -y -loglevel error -ss "$s" -i "$SRC" \
      -loop 1 -framerate 30 -i "$A/$cap.png" \
      -loop 1 -framerate 30 -i "$A/hook.png" \
      -filter_complex "$pre;$REFRAME;\
[1:v]format=rgba,fade=t=in:st=0:d=0.4:alpha=1,fade=t=out:st=$fo:d=0.4:alpha=1[c];\
[frame][c]overlay=0:0[f1];[2:v]format=rgba[hk];[f1][hk]overlay=0:0,format=yuv420p[v]" \
      -map "[v]" -t "$d" "${ENC[@]}" "parts/$name.mp4"
  fi
  add "$name.mp4"; }

# ===================== TIMELINE (keep it short: aim 20-50s total) =====================
#    card_v <png>      <dur> <fade> <name>
#    clip_v <start> <dur> <caption>  <blur> <name>
card_v title_v        1.6 0.3 s00
clip_v 67  6   capv_generate 3 s01
clip_v 81  9   capv_explore  0 s02
clip_v 505 11  capv_insight  1 s03
card_v outro_v        2.6 0.3 s04
# =====================================================================================

printf "file '%s'\n" "${PARTS[@]}" > parts/list.txt
ffmpeg -y -loglevel error -f concat -safe 0 -i parts/list.txt -c copy "$OUT_SHORT"
echo "wrote $OUT_SHORT  ($(ffprobe -v error -show_entries format=duration -of csv=p=0 "$OUT_SHORT")s, 1080x1920)"
