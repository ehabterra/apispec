#!/usr/bin/env bash
# Build a captioned, privacy-blurred, compressed demo from a screen recording.
#
#   1) python3 gen_assets.py          # render title/covers/cards/captions PNGs
#   2) edit SRC + the TIMELINE below  # put YOUR accurate in/out times here
#   3) ./build.sh                     # cut + blur + caption + concat -> OUT
#
# Requires: ffmpeg, ffprobe  (this build has NO drawtext, hence PNG overlays).
set -euo pipefail
cd "$(dirname "$0")"

# ---------------- CONFIG ----------------
SRC="$HOME/Desktop/Screen Recording 2026-05-31 at 1.59.28 PM.mov"  # <- your recording (absolute path is safest)
OUT="../apispecui-demo.mp4"                               # final file
A="assets"                                                # PNGs from gen_assets.py

# Privacy blur box, in SOURCE pixels: x y w h. (Measured for the 3582-wide cap.)
BX=2000; BY=170; BW=1582; BH=130
AX=1400; AY=1050; AW=500; AH=80
DX=1400; DY=480; DW=500; DH=80

# Encode settings — identical for every part so `concat -c copy` works.
ENC=(-c:v libx264 -pix_fmt yuv420p -r 30 -preset veryfast -crf 21 -an -video_track_timescale 30000)
# ----------------------------------------

rm -rf parts && mkdir -p parts
PARTS=()                          # collected in order
add(){ PARTS+=("$1"); }     # register a finished part for concat

# still PNG -> faded clip (title, covers, terminal cards).  card <png> <dur> <fade> <name>
card(){ local png=$1 dur=$2 fd=$3 name=$4
  ffmpeg -y -loglevel error -loop 1 -framerate 30 -t "$dur" -i "$A/$png.png" \
    -vf "scale=1920:1080,setsar=1,format=yuv420p,fade=t=in:st=0:d=$fd,fade=t=out:st=$(echo "$dur-$fd"|bc):d=$fd" \
    "${ENC[@]}" "parts/$name.mp4"; add "$name.mp4"; }

# footage clip.  clip <start> <dur> <caption|none> <blur:0|1> <name>
clip(){ local s=$1 d=$2 cap=$3 blur=$4 name=$5
  local fo; fo=$(echo "$d-0.4"|bc)
  local pre="[0:v]scale=1920:1080,setsar=1,fps=30[base]"
  [ "$blur" = 1 ] && pre="[0:v]crop=${BW}:${BH}:${BX}:${BY},boxblur=16[b];[0:v][b]overlay=${BX}:${BY}[pv];[pv]scale=1920:1080,setsar=1,fps=30[base]"
  [ "$blur" = 2 ] && pre="[0:v]crop=${AW}:${AH}:${AX}:${AY},boxblur=16[b];[0:v][b]overlay=${AX}:${AY}[pv];[pv]scale=1920:1080,setsar=1,fps=30[base]"
  [ "$blur" = 3 ] && pre="[0:v]crop=${DW}:${DH}:${DX}:${DY},boxblur=16[b];[0:v][b]overlay=${DX}:${DY}[pv];[pv]scale=1920:1080,setsar=1,fps=30[base]"
  if [ "$cap" = none ]; then
    ffmpeg -y -loglevel error -ss "$s" -i "$SRC" -filter_complex "$pre;[base]format=yuv420p[v]" \
      -map "[v]" -t "$d" "${ENC[@]}" "parts/$name.mp4"
  else
    ffmpeg -y -loglevel error -ss "$s" -i "$SRC" -loop 1 -framerate 30 -i "$A/$cap.png" -filter_complex \
      "$pre;[1:v]format=rgba,fade=t=in:st=0:d=0.4:alpha=1,fade=t=out:st=$fo:d=0.4:alpha=1[c];[base][c]overlay=0:0,format=yuv420p[v]" \
      -map "[v]" -t "$d" "${ENC[@]}" "parts/$name.mp4"
  fi
  add "$name.mp4"; }

# ===================== TIMELINE (edit START/DUR with YOUR watched times) =====================
#    card  <png>          <dur> <fade> <name>
#    clip  <start> <dur>  <caption>     <blur> <name>
card title          3.0 0.4 p00
card cov_install    2.2 0.2 p01
card card_install   4.0 0.3 p02
card cov_run        2.2 0.2 p03
card card_run       4.0 0.3 p04
card cov_generate   2.2 0.2 p05
clip 64  2  cap_generate  2 p06
clip 67  7.5  cap_generate  3 p07
clip 76  5  cap_generate  2 p08
card cov_explore    2.2 0.2 p09
clip 81 16  cap_explore   0 p10
card cov_configure  2.2 0.2 p11
clip 212 10  cap_configure 0 p12
card cov_insight    2.2 0.2 p13
clip 505 25  cap_insight   1 p14
clip 548.5 17  cap_insight   1 p15
clip 580 5  cap_insight   1 p16
card outro          5.0 0.4 p17
# ============================================================================================

printf "file '%s'\n" "${PARTS[@]}" > parts/list.txt
ffmpeg -y -loglevel error -f concat -safe 0 -i parts/list.txt -c copy "$OUT"
echo "wrote $OUT  ($(ffprobe -v error -show_entries format=duration -of csv=p=0 "$OUT")s)"
