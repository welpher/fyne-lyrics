package fynelyrics

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type ActiveLyricPosition int

const (
	// ActiveLyricPositionMiddle positions the active lyric line in the middle of the widget
	ActiveLyricPositionMiddle ActiveLyricPosition = iota

	// ActiveLyricPositionUpperMiddle positions the active lyric line
	// in the upper-middle of the widget, roughly 1/3 of the way down
	ActiveLyricPositionUpperMiddle
)

// LyricsViewer is a widget for displaying song lyrics.
// It supports synced and unsynced mode. In synced mode, the active line
// is highlighted and the widget can advance to the next line
// with an animated scroll. In unsynced mode all lyrics are shown
// in the active color and the user is allowed to scroll freely.
type LyricsViewer struct {
	widget.BaseWidget

	// Alignment controls the text alignment of the lyric lines
	Alignment fyne.TextAlign

	// TextSizeName is the theme size name that controls the size of the lyric lines.
	// Defaults to theme.SizeNameSubHeadingText.
	TextSizeName fyne.ThemeSizeName

	// ActiveLyricColorName is the theme color name that the currently active
	// lyric line will be drawn in synced mode, or all lyrics in non-synced mode.
	// Defaults to theme.ColorNameForeground.
	ActiveLyricColorName fyne.ThemeColorName

	// InactiveLyricColorName is the theme color name that the inactive lyric lines
	// will be drawn in synced mode. Defaults to theme.ColorNameDisabled.
	InactiveLyricColorName fyne.ThemeColorName

	// HoveredLyricColorName is the theme color name that hovered lyric lines
	// will be drawn in synced mode when an OnLyricTapped callback is set.
	// Defaults to theme.ColorNameHover.
	HoveredLyricColorName fyne.ThemeColorName

	// ActiveLyricPosition sets the vertical positioning of the active lyric line
	// in synced mode.
	ActiveLyricPosition ActiveLyricPosition

	// OnLyricTapped sets a callback function that is invoked when a
	// synced lyric line is tapped. The line number is *one-indexed*.
	// Typically used to seek to the timecode of the given lyric.
	// When showing unsynced lyrics, or if this callback is unset,
	// the visual styling of the widget will not indicate interactivity.
	OnLyricTapped func(lineNum int)

	// LineStarts optionally provides each line's start time (seconds),
	// matching len(lines). If nil, starts are derived from each line's
	// first tagged word.
	LineStarts []float64

	lines  []string
	synced bool

	lineWords  [][]word
	lineStarts []float64

	timeDriven  bool
	lastCurTime float64
	lastPushAt  time.Time
	predAnim    *fyne.Animation

	// one-indexed - 0 means before the first line
	// during an animation, currentLine is the line
	// that will be scrolled when the animation is finished
	currentLine int

	prototypeLyricLineSize fyne.Size

	scroll *container.Scroll
	vbox   *fyne.Container

	// nil when an animation is not currently running
	anim            *fyne.Animation
	animStartOffset float32
}

// NewLyricsViewer returns a new lyrics viewer.
func NewLyricsViewer() *LyricsViewer {
	s := &LyricsViewer{}
	s.ExtendBaseWidget(s)
	s.prototypeLyricLineSize = newLyricLine(parseWords("Hello World"), nil).MinSize()
	return s
}

// SetLyrics sets the lyrics and also resets the current line to 0 if synced.
func (l *LyricsViewer) SetLyrics(lines []string, synced bool) {
	l.lines = lines
	l.synced = synced
	l.currentLine = 0
	l.timeDriven = false
	l.stopPredictive()
	l.lineWords = make([][]word, len(lines))
	for i, ln := range lines {
		l.lineWords[i] = parseWords(ln)
	}
	l.lineStarts = l.computeLineStarts()
	if l.scroll != nil {
		if synced {
			l.scroll.Direction = container.ScrollNone
		} else {
			l.scroll.Direction = container.ScrollVerticalOnly
		}
	}
	l.updateContent()
}

func (l *LyricsViewer) computeLineStarts() []float64 {
	if len(l.LineStarts) == len(l.lines) {
		out := make([]float64, len(l.LineStarts))
		copy(out, l.LineStarts)
		return out
	}
	out := make([]float64, len(l.lineWords))
	for i, words := range l.lineWords {
		start := -1.0
		for _, w := range words {
			if w.start >= 0 {
				start = w.start
				break
			}
		}
		out[i] = start
	}
	return out
}

// SetCurrentLine sets the current line that the lyric viewer is scrolled to.
// Argument is *one-indexed* - SetCurrentLine(0) means setting the scroll to be
// before the first line. In unsynced mode this is a no-op. This function is
// typically called when the user has seeked the playing song to a new position.
func (l *LyricsViewer) SetCurrentLine(line int) {
	if l.timeDriven {
		return
	}
	if line < 0 || line > len(l.lines) {
		panic("SetCurrentLine: line number out of range")
	}
	if l.vbox == nil || !l.synced {
		l.currentLine = line
		return // renderer not created yet or unsynced mode
	}
	if l.checkStopAnimation() && l.currentLine > 1 {
		// we were in the middle of animation
		// make sure prev line is right color
		l.setLineActive(l.vbox.Objects[l.currentLine-1].(*lyricLine), false, true)
	}
	if l.currentLine != 0 {
		l.setLineActive(l.vbox.Objects[l.currentLine].(*lyricLine), false, true)
	}
	l.currentLine = line
	if l.currentLine != 0 {
		l.setLineActive(l.vbox.Objects[l.currentLine].(*lyricLine), true, true)
	}
	l.scroll.Offset.Y = l.offsetForLine(l.currentLine)
	l.scroll.Refresh()
}

// NextLine advances the lyric viewer to the next line with an animated scroll.
// In unsynced mode this is a no-op.
func (l *LyricsViewer) NextLine() {
	if l.timeDriven {
		return
	}
	if l.vbox == nil || !l.synced {
		return // no renderer yet, or unsynced lyrics (no-op)
	}

	if l.currentLine == len(l.lines) {
		return // already at last line
	}
	if l.checkStopAnimation() {
		// we were in the middle of animation - short-circuit it to completed
		// make sure prev and current lines are right color and scrolled to the end
		if l.currentLine > 1 {
			l.setLineActive(l.vbox.Objects[l.currentLine-1].(*lyricLine), false, true)
		}
		l.setLineActive(l.vbox.Objects[l.currentLine].(*lyricLine), true, true)
		l.scroll.Offset.Y = l.offsetForLine(l.currentLine)
	}
	l.currentLine++

	var prevLine, nextLine *lyricLine
	if l.currentLine > 1 {
		prevLine = l.vbox.Objects[l.currentLine-1].(*lyricLine)
	}
	if l.currentLine <= len(l.lines) {
		nextLine = l.vbox.Objects[l.currentLine].(*lyricLine)
	}

	l.setupScrollAnimation(prevLine, nextLine)
	l.anim.Start()
}

// SetPlayTime drives the lyric viewer from the current play position of the
// song. curTime is the current play time in seconds; seeked=true forces an
// immediate full reposition. SetPlayTime permanently switches the viewer
// into time-driven mode, after which NextLine and SetCurrentLine become
// no-ops. It must be called on the UI/main goroutine: it mutates widget
// state and starts/stops animations, which are not thread-safe resources.
func (l *LyricsViewer) SetPlayTime(curTime float64, seeked bool) {
	if l.vbox == nil || !l.synced {
		l.lastCurTime = curTime
		return
	}
	if !l.timeDriven {
		l.timeDriven = true
	}
	l.stopPredictive()
	l.lastCurTime = curTime
	l.lastPushAt = time.Now()

	target := locateActiveLine(l.lineStarts, curTime)
	if target < 0 {
		l.setActiveLine(0, 0, seeked)
	} else {
		words := l.lineWords[target]
		active := countActiveWords(words, curTime)
		l.setActiveLine(target+1, active-1, seeked)
	}
	l.schedulePredictive()
}

func (l *LyricsViewer) setActiveLine(oneIndexedLine, activeWordIdx int, forceScroll bool) {
	if oneIndexedLine == l.currentLine && !forceScroll {
		l.updateActiveWord(activeWordIdx)
		return
	}
	l.checkStopAnimation()
	if l.currentLine > 0 && l.currentLine <= len(l.lines) {
		l.setLineActive(l.vbox.Objects[l.currentLine].(*lyricLine), false, true)
	}
	l.currentLine = oneIndexedLine
	var prevLine, nextLine *lyricLine
	if l.currentLine > 1 {
		prevLine = l.vbox.Objects[l.currentLine-1].(*lyricLine)
	}
	if l.currentLine >= 1 && l.currentLine <= len(l.lines) {
		nextLine = l.vbox.Objects[l.currentLine].(*lyricLine)
		l.setLineActive(nextLine, true, false)
		l.updateActiveWord(activeWordIdx)
	}
	if forceScroll {
		l.scroll.Offset.Y = l.offsetForLine(l.currentLine)
		l.scroll.Refresh()
	} else {
		l.setupScrollAnimation(prevLine, nextLine)
		l.anim.Start()
	}
}

func (l *LyricsViewer) updateActiveWord(activeWordIdx int) {
	if l.currentLine < 1 || l.currentLine > len(l.lines) {
		return
	}
	ll := l.vbox.Objects[l.currentLine].(*lyricLine)
	if ll.activeWordIdx == activeWordIdx {
		return
	}
	ll.activeWordIdx = activeWordIdx
	ll.Refresh()
}

const predictiveHorizon = 1 * time.Second
const stallThreshold = 600 * time.Millisecond

func (l *LyricsViewer) schedulePredictive() {
	if l.currentLine < 1 || l.currentLine > len(l.lines) {
		return
	}
	words := l.lineWords[l.currentLine-1]
	b, ok := nextBoundary(words, l.lineStarts, l.currentLine-1, l.lastCurTime)
	if !ok {
		return
	}
	delta := b - l.lastCurTime
	deltaDuration := time.Duration(delta * float64(time.Second))
	if delta <= 0 || deltaDuration > predictiveHorizon {
		return
	}
	cur := l.lastCurTime
	l.predAnim = fyne.NewAnimation(deltaDuration, func(f float32) {
		if f < 1 {
			return
		}
		if time.Since(l.lastPushAt) > stallThreshold {
			l.predAnim = nil
			return
		}
		l.advanceTo(cur + delta)
	})
	l.predAnim.Curve = fyne.AnimationLinear
	l.predAnim.Start()
}

func (l *LyricsViewer) advanceTo(next float64) {
	l.lastCurTime = next
	target := locateActiveLine(l.lineStarts, next)
	var words []word
	if target >= 0 {
		words = l.lineWords[target]
	} else if l.currentLine >= 1 {
		words = l.lineWords[l.currentLine-1]
	}
	active := countActiveWords(words, next)
	if target >= 0 && target+1 != l.currentLine {
		l.setActiveLine(target+1, active-1, false)
	} else {
		l.updateActiveWord(active - 1)
	}
	l.schedulePredictive()
}

func (l *LyricsViewer) stopPredictive() {
	if l.predAnim != nil {
		l.predAnim.Stop()
		l.predAnim = nil
	}
}

func (l *LyricsViewer) Refresh() {
	l.updateContent()
}

func (l *LyricsViewer) MinSize() fyne.Size {
	// overridden because NoScroll will have minSize encompass the full lyrics
	// note also that leaving this to the renderer MinSize, based on the
	// VBox with RichText lines inside Scroll, may lead to race conditions
	// (https://github.com/fyne-io/fyne/issues/4890)
	minHeight := l.prototypeLyricLineSize.Height*3 + theme.Padding()*2
	return fyne.NewSize(l.prototypeLyricLineSize.Width, minHeight)
}

func (l *LyricsViewer) Resize(size fyne.Size) {
	l.updateSpacerSize(size)
	l.BaseWidget.Resize(size)
	if l.vbox == nil {
		return // renderer not created yet
	}
	if l.anim == nil {
		l.scroll.Offset = fyne.NewPos(0, l.offsetForLine(l.currentLine))
		l.scroll.Refresh()
	} else {
		// animation is running - update its reference scroll pos
		l.animStartOffset = l.offsetForLine(l.currentLine - 1)
	}
}

func (l *LyricsViewer) updateSpacerSize(size fyne.Size) {
	if l.vbox == nil {
		return // renderer not created yet
	}

	ht := size.Height / 2
	if l.ActiveLyricPosition == ActiveLyricPositionUpperMiddle {
		ht = size.Height / 3
	}

	var topSpaceHeight, bottomSpaceHeight float32
	if l.synced {
		topSpaceHeight = ht + l.prototypeLyricLineSize.Height/2
		// end spacer only needs to be big enough - can't be too big
		// so use a very simple height calculation
		bottomSpaceHeight = size.Height
	}
	l.vbox.Objects[0].(*vSpace).Height = topSpaceHeight
	l.vbox.Objects[len(l.vbox.Objects)-1].(*vSpace).Height = bottomSpaceHeight
}

func (l *LyricsViewer) updateContent() {
	if l.vbox == nil {
		return // renderer not created yet
	}
	l.checkStopAnimation()

	lnObj := len(l.vbox.Objects)
	if lnObj == 0 {
		l.vbox.Objects = append(l.vbox.Objects, NewVSpace(0), NewVSpace(0))
		lnObj = 2
	}
	l.updateSpacerSize(l.Size())
	endSpacer := l.vbox.Objects[lnObj-1]
	for i := range l.lines {
		lineNum := i + 1 // one-indexed
		useActiveColor := !l.synced || l.currentLine == lineNum
		if lineNum < lnObj-1 {
			rt := l.vbox.Objects[lineNum].(*lyricLine)
			if useActiveColor {
				l.setLineActive(rt, true, false)
			} else {
				l.setLineActive(rt, false, false)
			}
			l.setLineProperties(rt, lineNum, true)
		} else if lineNum < lnObj {
			// replacing end spacer (last element in Objects) with a new richtext
			l.vbox.Objects[lineNum] = l.newLyricLine(lineNum, useActiveColor)
		} else {
			// extending the Objects slice
			l.vbox.Objects = append(l.vbox.Objects, l.newLyricLine(lineNum, useActiveColor))
		}
	}
	for i := len(l.lines) + 1; i < lnObj; i++ {
		l.vbox.Objects[i] = nil
	}
	l.vbox.Objects = l.vbox.Objects[:len(l.lines)+1]
	l.vbox.Objects = append(l.vbox.Objects, endSpacer)
	l.vbox.Refresh()
	l.scroll.Offset.Y = l.offsetForLine(l.currentLine)
	l.scroll.Refresh()
}

func (l *LyricsViewer) setupScrollAnimation(currentLine, nextLine *lyricLine) {
	// scroll to the exact position that centers the active line
	l.animStartOffset = l.scroll.Offset.Y
	scrollDist := l.offsetForLine(l.currentLine) - l.animStartOffset
	var alreadyUpdated bool
	l.anim = fyne.NewAnimation(140*time.Millisecond, func(f float32) {
		l.scroll.Offset.Y = l.animStartOffset + f*scrollDist
		l.scroll.Refresh()
		if !alreadyUpdated && f >= 0.5 {
			if nextLine != nil {
				activeWord := nextLine.activeWordIdx
				l.setLineActive(nextLine, true, false)
				nextLine.activeWordIdx = activeWord
				nextLine.Refresh()
			}
			if currentLine != nil {
				l.setLineActive(currentLine, false, true)
			}
			alreadyUpdated = true
		}
		if f == 1 /*end of animation*/ {
			l.anim = nil
		}
	})
	l.anim.Curve = fyne.AnimationEaseInOut
}

func (l *LyricsViewer) offsetForLine(lineNum int /*one-indexed*/) float32 {
	if lineNum == 0 {
		return 0
	}
	pad := theme.Padding()
	offset := pad + l.prototypeLyricLineSize.Height/2
	for i := 1; i <= lineNum; i++ {
		if i > 1 {
			offset += l.vbox.Objects[i-1].MinSize().Height/2 + pad
		}
		offset += l.vbox.Objects[i].MinSize().Height / 2
	}
	return offset
}

func (l *LyricsViewer) newLyricLine(lineNum int, useActiveColor bool) *lyricLine {
	var words []word
	if lineNum >= 1 && lineNum <= len(l.lineWords) {
		words = l.lineWords[lineNum-1]
	}
	ll := newLyricLine(words, nil)
	l.setLineProperties(ll, lineNum, false)
	ll.HoveredColorName = l.hoveredLyricColor()
	if useActiveColor {
		ll.ColorName = l.activeLyricColor()
		ll.InactiveColorName = l.inactiveLyricColor()
		ll.activeWordIdx = len(words) - 1
	} else {
		ll.ColorName = l.inactiveLyricColor()
		ll.InactiveColorName = l.inactiveLyricColor()
		ll.activeWordIdx = -1
	}
	return ll
}

func (l *LyricsViewer) setLineProperties(ll *lyricLine, lineNum int, refresh bool) {
	ll.SizeName = l.textSizeName()
	ll.Alignment = l.Alignment
	ll.Tappable = l.synced && l.OnLyricTapped != nil
	ll.onTapped = func() {
		if l.OnLyricTapped != nil {
			l.OnLyricTapped(lineNum)
		}
	}
	if refresh {
		ll.Refresh()
	}
}

func (l *LyricsViewer) setLineActive(ll *lyricLine, active bool, refresh bool) {
	if active {
		ll.ColorName = l.activeLyricColor()
		ll.InactiveColorName = l.inactiveLyricColor()
		ll.activeWordIdx = len(ll.words) - 1
	} else {
		ll.ColorName = l.inactiveLyricColor()
		ll.InactiveColorName = l.inactiveLyricColor()
		ll.activeWordIdx = -1
	}
	if refresh {
		ll.Refresh()
	}
}

func (l *LyricsViewer) activeLyricColor() fyne.ThemeColorName {
	if l.ActiveLyricColorName != "" {
		return l.ActiveLyricColorName
	}
	return theme.ColorNameForeground
}

func (l *LyricsViewer) inactiveLyricColor() fyne.ThemeColorName {
	if l.InactiveLyricColorName != "" {
		return l.InactiveLyricColorName
	}
	return theme.ColorNameDisabled
}

func (l *LyricsViewer) hoveredLyricColor() fyne.ThemeColorName {
	if l.HoveredLyricColorName != "" {
		return l.HoveredLyricColorName
	}
	return theme.ColorNameHyperlink
}

func (l *LyricsViewer) textSizeName() fyne.ThemeSizeName {
	if l.TextSizeName != "" {
		return l.TextSizeName
	}
	return theme.SizeNameSubHeadingText
}

func (l *LyricsViewer) checkStopAnimation() bool {
	if l.anim != nil {
		l.anim.Stop()
		l.anim = nil
		return true
	}
	return false
}

func (l *LyricsViewer) CreateRenderer() fyne.WidgetRenderer {
	l.vbox = container.NewVBox()
	l.scroll = container.NewScroll(l.vbox)
	if l.synced {
		l.scroll.Direction = container.ScrollNone
	} else {
		l.scroll.Direction = container.ScrollVerticalOnly
	}
	l.updateContent()
	return widget.NewSimpleRenderer(l.scroll)
}

type vSpace struct {
	widget.BaseWidget

	Height float32
}

func NewVSpace(height float32) *vSpace {
	v := &vSpace{Height: height}
	v.ExtendBaseWidget(v)
	return v
}

func (v *vSpace) MinSize() fyne.Size {
	return fyne.NewSize(0, v.Height)
}

func (v *vSpace) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(layout.NewSpacer())
}
