// Screencast arithmetic, kept apart from the kit so it can be tested without a
// browser.
//
// CDP's Page.startScreencast sends a frame only when the page CHANGES, stamped
// with the wall-clock time it was composited. Playback needs a constant rate,
// so each frame is held until the next one arrived — an ffconcat script with a
// duration per frame — and ffmpeg's fps filter resamples that to 60.

export function concatScript(frames, endTs) {
  if (!frames.length) throw new Error('concatScript: no frames were captured')
  const lines = ['ffconcat version 1.0']
  frames.forEach((f, i) => {
    const next = i + 1 < frames.length ? frames[i + 1].ts : endTs
    // Two frames can share a timestamp. A zero duration makes the concat
    // demuxer drop the entry, and with it a frame that really was shown.
    const d = Math.max(next - f.ts, 0.001)
    lines.push(`file '${f.file}'`, `duration ${d.toFixed(6)}`)
  })
  // The demuxer ignores the LAST entry's duration unless the file is listed
  // once more, so without this the final hold is lost.
  lines.push(`file '${frames[frames.length - 1].file}'`)
  return lines.join('\n') + '\n'
}

// Seconds into the clip for a wall-clock moment in ms. Beats are timed with
// Date.now(); the clip starts at the first composited frame.
export const clipTime = (wallMs, firstTs) => Math.max(0, wallMs / 1000 - firstTs)

// Longest interval between consecutive frames that both fall inside the window.
// A long gap in a beat that should be MOVING is a stall worth looking at; in a
// beat that holds still it is just the page not changing.
export function maxGap(frames, fromTs = -Infinity, toTs = Infinity) {
  let worst = 0
  for (let i = 1; i < frames.length; i++) {
    const a = frames[i - 1].ts
    const b = frames[i].ts
    if (a >= fromTs && b <= toTs) worst = Math.max(worst, b - a)
  }
  return worst
}

// Every beat time is computed against this clock. If a Chromium build stamps
// frames with something other than epoch seconds, the cut lands on the wrong
// moments in silence — so refuse it on the first frame.
export function assertEpochSeconds(ts, nowMs) {
  if (!(Math.abs(ts * 1000 - nowMs) < 60_000)) {
    throw new Error(`screencast timestamp ${ts} is not epoch seconds (now ${nowMs / 1000})`)
  }
}
