# Inline Karaoke (Word-by-Word Highlighting) Support — Design Document

Date: 2026-09-10
Status: Implemented

## Background and Goals

fyne-lyrics is a Fyne lyric-display widget supporting synced and unsynced modes.
Originally it was driven from the outside at "line" granularity via
`NextLine()` / `SetCurrentLine(n)`.

It needs to support word-by-word lyric formats with inline tags, for example:

```lrc
[00:00.00]<00:00.00>床<00:00.40>前<00:00.80>明<00:01.20>月<00:01.60>光<00:02.00> <00:02.40>(Moonlight)<00:03.00>...
```

That is, each line of text embeds `<MM:SS.CC>` tags indicating the start time
(hundredths of a second) of the following character/word.
Goal: within the active line, light up characters progressively as playback
advances (karaoke effect).

### Relationship with the downstream consumer (supersonic)

supersonic already strips the leading `[...]` line tags (storing the line start
time in its own `LyricLine.Start`) and passes `lyric.Lines[i].Text` to
fyne-lyrics. It has been confirmed that:

- lrclib parsing path: `Text` **retains** the inline `<...>` tags.
- subsonic structured-lyrics path: `Text` is plain text and `Start` is stored
  separately, with **no** inline tags.

Therefore the widget must not assume every line carries inline tags; parsing
must support both cases and degrade gracefully when tags are absent.

## Design Principles

- **Fully backward compatible**: no existing API changes; normal lyrics behave
  exactly as before.
- **Mode is determined by whether `SetPlayTime` is called**:
  - Never called → keep the existing `NextLine`/`SetCurrentLine` behavior.
  - Once called → enter "time-driven mode", where the widget advances the line,
    lights characters, and scrolls on its own for each `SetPlayTime`.
- **Performance**: the push supplies a coarse time baseline; a short-range
  predictive timer refines word-boundary precision. Redraw only on state change.

## New Public API

### 1. Play-time input (push mode)

```go
// Called by the consumer on every play-time update. curTime is the current
// play time in seconds; seeked indicates whether this update came from a seek.
// Until SetPlayTime is called the widget keeps the existing NextLine /
// SetCurrentLine behavior; once called it enters time-driven mode.
// Must be called on the UI thread: it mutates widget state and starts/stops
// animations.
func (l *LyricsViewer) SetPlayTime(curTime float64, seeked bool)
```

Push rather than pull (a function handle), because:

- supersonic already has an `OnPlayTimeUpdate` polling loop (normally 250ms,
  100ms with waveform); the consumer just forwards to a callback it already
  has, with zero extra burden.
- The call cadence is set by the supplier's polling rate (~250ms), not
  per-word high frequency, so there is no performance concern.
- The widget maintains only one **short-range predictive timer** for
  word-boundary precision; no long-lived goroutine and no self-built scheduler.

### 2. Optional line start times (robustness)

```go
// Per-line start time (seconds), strictly one-to-one with SetLyrics' lines.
// When nil, a line's active window is derived from its first tagged word
// ([first word of this line, first word of next line)).
// Mainly lets lyrics without inline tags (e.g. the subsonic structured path)
// still switch lines correctly.
LineStarts []float64
```

This field is optional; leaving it unset does not affect the mainstream case
that relies on inline tags.

## Inline Tag Parsing

New parsing logic splits each line received by `SetLyrics` into word segments:

```go
type word struct {
    text  string
    start float64 // seconds; -1 means no timing (untagged text)
}
```

- Detect `<MM:SS.CC>`:
  - The text following a tag (up to the next `<`) becomes that segment's `text`.
  - `start = min*60 + ss + cc/100`.
  - Unpaired text before a tag becomes an untimed segment (`start = -1`),
    displayed in order.
- No tags at all → the whole line is a single untimed segment and goes through
  the fallback path (display unaffected).
- The parser is a standalone pure function (`parseWords`) for easy unit testing.

## Rendering (Time-Driven Mode)

### Key constraint: font fallback and baselines

Under a real GL driver (especially macOS with supersonic's Fyne fork), **each
independent text object performs its own font fallback**. When a line is split
into multiple text objects (one per character/word), the same glyph may resolve
to different fallback fonts, so **baselines differ and characters sit at
different heights**.

Note: `RenderedTextSize` reports identical baselines for all characters (e.g.
19.23 for all), but the actual rendering differs — the **measured value is
unreliable**. So "manually aligning multiple text objects by their measured
baseline" cannot fix this.

**The only reliable approach is a single shaping run**: when laying out a
*single string*, go-text aligns all fallback runs to one baseline; split across
multiple strings, each is on its own.

### Implementation: whole-line base + clipped same-string overlay

`lyricLine` no longer creates one text segment per word. Instead:

- `baseText`: a single `canvas.Text` of the whole line (all words concatenated),
  in `InactiveLyricColor`.
- `activeText`: another `canvas.Text` of **exactly the same whole-line string**,
  in `ActiveLyricColor`.
- `activeText` is wrapped in a clipping widget `lyricClip` whose width equals the
  **pixel width of the sung prefix**.

How it works:

- Both layers use the identical string → shaping is per-glyph identical → glyphs
  coincide pixel-for-pixel, **no ghosting**.
- Clipping reveals only the sung prefix → word-by-word highlighting.
- The clip width is measured with a `canvas.Text` of the prefix (the same
  measurement path as rendering), so the boundary never cuts into the next glyph.

**Cross-version clipping compatibility** (critical):

| Fyne version | Clipping trigger |
|---|---|
| Fyne 2.5.x | object implements `fyne.Scrollable` |
| supersonic's Fyne fork | renderer implements `IsClip()` |

`lyricClip` implements **both**, so clipping works on both versions.
In synced mode `scroll.Direction = ScrollNone` (scrolling is already disabled),
so the `Scrollable` marker does not affect scrolling; unsynced mode has no inline
tags, produces no multi-word lines, and therefore never uses clipping.

### Line window derivation

When `LineStarts` is nil, a line's active time window is
`[first tagged word of this line, first tagged word of next line)`.
When it cannot be derived, the whole line is shown statically in
InactiveLyricColor.

### Active-line scrolling

Reuses the existing `offsetForLine` logic:

- Normal line change: animate the scroll to the exact target offset
  (`offsetForLine(currentLine)`), centered.
- `seeked=true` (seek): **jump instantly** to the exact offset, no animation.
- Word-by-word lighting only updates the overlay's clip width; it does not
  trigger line scrolling.

## Internal Advancement: Push + Predictive Timer

In time-driven mode, `SetPlayTime` (~every 250ms) provides a coarse time
baseline, and a **short-range predictive timer** refines word-boundary precision
so a word lights exactly at its `start` instead of waiting for the next push.

The predictive timer is implemented with **`fyne.Animation`** (not
`time.AfterFunc`): Fyne 2.5.3 has no `fyne.Do`, whereas a `fyne.Animation`
callback runs on the UI thread and stops automatically when the widget is not
visible — no goroutine and no thread-scheduling code. Each `SetPlayTime` cancels
and reschedules this animation.

**On each `SetPlayTime(curTime, seeked)`:**

1. Record `lastCurTime = curTime`, `lastPushAt = now` (wall clock); cancel the
   pending predictive animation.
2. Locate the current line + `activeWordIdx` from `curTime` (`seeked=true` forces
   a full reposition; otherwise incremental).
3. Find the next word/line boundary `T_next` (the nearest one > `curTime`).
4. If `T_next - curTime <= predictiveHorizon` (1s), schedule
   `fyne.Animation(T_next - curTime)`; otherwise don't (wait for the next push).

**When the predictive animation completes (callback on the UI thread):**

1. `elapsed = now - lastPushAt`.
2. **Stall detection**: if `elapsed > stallThreshold` (600ms, ~2.5× the 250ms
   poll) → treat as paused/stopped, cancel further scheduling and return (the
   next `SetPlayTime` resumes).
3. Otherwise: light the words in the current line with `start <= T_next`
   (advance `activeWordIdx`), switching lines/scrolling if needed.
4. Find the next boundary and schedule again per step 4.

**Pause safety**: supersonic stops pushing `OnPlayTimeUpdate` while paused
(confirmed: `OnPaused → stopPollTimePos`). Pure push freezes naturally; the
predictive timer stays pause-safe via stall detection — during a pause it
advances at most about `stallThreshold` (~1 word) incorrectly, and the first push
after resume corrects it immediately from the real `curTime`.

**Bounds**:

- `predictiveHorizon` limits a single prediction span, so an instrumental section
  does not schedule a far-future timer making a long wall-clock assumption.
- Variable-rate playback (rate≠1) makes the prediction drift; the next push
  corrects it, which is acceptable.
- `SetLyrics` / widget destruction / `seeked=true` cancel the pending predictive
  animation.

### Rendering cost

On each position change, the text and color of the two whole-line
`canvas.Text` objects (base/active) are rebuilt. Each line has only two text
objects, so the cost is negligible; no per-segment incremental coloring.

## Highlight Granularity (v1 scope)

- **Word-by-word color change** (default, v1): when a word reaches its `start`,
  the clip width advances past it and that word switches to the Active color.
- **Smooth gradient** (optional enhancement, not implemented): interpolating the
  color between words by progress is what mainstream apps do, but it requires
  per-frame redraw and is expensive. YAGNI — left for later.

## Compatibility and Integration

- All legacy APIs (`SetLyrics`/`SetCurrentLine`/`NextLine`/`OnLyricTapped`) are
  preserved.
- When `SetPlayTime` is never called, behavior is identical to before.
- Plain lyrics (no inline tags) still display in time-driven mode, just without
  word-by-word effect; line switching is still correct when `LineStarts` is
  provided.
- supersonic integration:

```go
pm.OnPlayTimeUpdate(func(cur, _ float64, seeked bool) {
    fyne.Do(func() { lyricsViewer.SetPlayTime(cur, seeked) })
})
```

The logic in its `UpdatePlayPos`/`OnSeeked` that drives
`NextLine`/`SetCurrentLine` is disabled and replaced by forwarding to
`SetPlayTime`; `SetLyrics` additionally passes `LineStarts`.

## Error and Edge Handling

- A lyric line with empty text → no word segments; an empty line is shown
  (unchanged).
- Malformed tag (e.g. `<xx:xx>` unparseable) → skip the tag, keep the text
  literally.
- `LineStarts` length mismatching `lines` → use the shorter; the remainder is
  derived from each line's first word or shown statically.
- `SetPlayTime` with negative or clearly invalid `curTime` → treat as 0, i.e. the
  earliest position.
- `advanceTo` skips safely when `currentLine == 0` (no line entered yet), no
  out-of-range access.
- Repeated `SetLyrics` → full reset (current line back to the first,
  `activeWordIdx` cleared, `timeDriven` reset) and cancel the pending predictive
  animation.
- Multiple `SetPlayTime` calls within one play-time update → idempotent; redraw
  only when the position changes.

## Test Strategy

- **Parsing** (`wordparse_test.go`): standard `<MM:SS.CC>`, untagged lines, mixed
  text, malformed tags, CJK/spaces/multibyte.
- **Pure logic** (`karaoke_test.go`): `locateActiveLine` line location,
  `countActiveWords` count of sung words, `nextBoundary` next boundary.
- **Viewer** (`lyricsviewer_test.go`): `SetLyrics` parses `lineWords`/`lineStarts`;
  external `LineStarts` takes precedence.
- **Rendering** (`lyricline_test.go`): a multi-word line's base and active layers
  use the identical string, share the same position, and a clipping overlay
  exists.
- **Line transition** (`line_transition_test.go`): when crossing a line boundary,
  the new line's `activeWordIdx` is computed from the target line's words, so the
  line does not "flash fully lit" first.
- **Compatibility**: existing tests/behavior do not regress when `SetPlayTime` is
  never called.

## File List (implementation)

- `wordparse.go` / `wordparse_test.go`: `<MM:SS.CC>` parsing pure functions.
- `karaoke.go` / `karaoke_test.go`: time-driven pure logic (line location, sung
  word count, next boundary).
- `lyricline.go` / `lyricline_test.go`: single-shaping rendering + clipped
  overlay.
- `lyricsviewer.go` / `lyricsviewer_test.go`: `SetPlayTime`, `LineStarts`,
  predictive timer, line switching.
- `line_transition_test.go`: line-transition word-progress regression.
- `demo/karaoke/main.go`: demo with an inlined lyric (simulated playback +
  tap-to-seek).
