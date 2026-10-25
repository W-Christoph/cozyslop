# Experimental subtitle detection

Settings → Room → Subtitle cover offers three modes:

- Always cover: manually positioned covers, with no detection work.
- Detect region: cover a padded bounding rectangle around text-like rows inside each search box.
- Detect presence: cover the entire search box when text-like rows are found.

The two boxes are independent search regions. Feathering extends outside the
fully covered region; it does not change the draggable box or its saved size.

This first implementation detects connected bright white/yellow shapes with
dark borders and groups similarly sized shapes into rows. It does not recognize
words or distinguish subtitles from other text. Dark subtitles, touching glyphs,
single-character lines and low contrast text may be missed. Two/three-character
replies are accepted with tighter glyph-spacing and size checks. Scenery can cause false
positives. Use manual mode when dependable coverage matters.

## Processing budget

One lazy browser worker handles both boxes. Each crop is downscaled, preserving
aspect ratio, to at most 640 × 240 pixels without upscaling. There is at most one
request in flight. The detection interval is adjustable in Settings → Room →
Subtitle cover from 25 to 250 ms in 25 ms steps, defaulting to 50 ms, and is saved
per browser. Slow checks increase the interval. Hidden tabs, hidden video, paused playback and audio-only mode do not
sample. Disabling the feature terminates the worker. Errors or a three-second
worker timeout switch to full covers.

Frame copying/readback happens on the main thread; pixel analysis runs in the
worker. Frames stay in the browser. There is no OCR model download, server
processing, or additional stream connection. Blur still uses local video layers.
Analysis time is not a measurement of total CPU/GPU or battery use.

Synthetic headless Chromium testing on the development machine measured about
0.5 ms per 640 × 240 crop, excluding readback and rendering. Do not extrapolate
this into a device performance guarantee. The browser test prints a fresh
measurement. Real-film accuracy and mobile performance remain unmeasured.

Detected bounds expand immediately, retain the last 600 ms of coverage, then
ease inward with a 400 ms time constant. Each box has independent tracking,
reset when detection stops or the search boxes change.

A new subtitle can show for the sampling interval plus processing time before
coverage starts. At least two consecutive misses and 500 ms since the last
detection remove a cover, preserving the flicker protection at faster sample rates; slow
devices can therefore also retain covers longer. This is not frame-exact masking.

## Verification

Run `npm run test:subtitles` and `npm run build` from `web/`.
For synthetic video integration checks, run Vite, then:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright/test node web/tests/subtitle-browser.cjs
```

The browser test checks regional and whole-box coverage, text disappearing,
outward feather geometry, two boxes, worker cleanup, and worker-failure fallback.
It requires an installed Chromium for Playwright. Set `SUBTITLE_TEST_URL` if Vite
is not at `http://127.0.0.1:5173`.
