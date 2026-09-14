package fynelyrics

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type lyricLine struct {
	widget.BaseWidget

	words             []word
	activeWordIdx     int
	SizeName          fyne.ThemeSizeName
	ColorName         fyne.ThemeColorName
	InactiveColorName fyne.ThemeColorName
	HoveredColorName  fyne.ThemeColorName
	Alignment         fyne.TextAlign
	Tappable          bool

	onTapped   func()
	hovered    bool
	textView   *fyne.Container
	baseText   *canvas.Text
	activeText *canvas.Text
	activeClip *lyricClip
}

func newLyricLine(words []word, onTapped func()) *lyricLine {
	l := &lyricLine{
		words:         words,
		activeWordIdx: -1,
		SizeName:      theme.SizeNameSubHeadingText,
		ColorName:     theme.ColorNameForeground,
		Alignment:     fyne.TextAlignLeading,
		onTapped:      onTapped,
	}
	l.ExtendBaseWidget(l)
	return l
}

var _ desktop.Hoverable = (*lyricLine)(nil)

func (l *lyricLine) MouseIn(*desktop.MouseEvent) {
	if l.Tappable {
		l.hovered = true
		l.Refresh()
	}
}

func (l *lyricLine) MouseMoved(*desktop.MouseEvent) {}

func (l *lyricLine) MouseOut() {
	l.hovered = false
	l.Refresh()
}

var _ desktop.Cursorable = (*lyricLine)(nil)

func (l *lyricLine) Cursor() desktop.Cursor {
	if l.Tappable {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

var _ fyne.Tappable = (*lyricLine)(nil)

func (l *lyricLine) Tapped(*fyne.PointEvent) {
	if l.Tappable {
		l.onTapped()
	}
}

// textSize returns the font size (pixels) used for lyric text.
func (l *lyricLine) textSize() float32 {
	return theme.SizeForWidget(l.SizeName, l)
}

// textColor returns the concrete color for a themed lyric color name.
func (l *lyricLine) textColor(col fyne.ThemeColorName) color.Color {
	if col == "" {
		col = theme.ColorNameForeground
	}
	return theme.ColorForWidget(col, l)
}

func (l *lyricLine) inactiveColor() fyne.ThemeColorName {
	if l.InactiveColorName != "" {
		return l.InactiveColorName
	}
	return theme.ColorNameDisabled
}

func (l *lyricLine) sungPrefix() string {
	if l.activeWordIdx < 0 {
		return ""
	}
	n := l.activeWordIdx + 1
	if n > len(l.words) {
		n = len(l.words)
	}
	var b []byte
	for i := 0; i < n; i++ {
		b = append(b, l.words[i].text...)
	}
	return string(b)
}

func (l *lyricLine) fullText() string {
	var b []byte
	for _, w := range l.words {
		b = append(b, w.text...)
	}
	return string(b)
}

func (l *lyricLine) updateTextView() {
	if l.textView == nil {
		l.textView = container.New(&lyricLineLayout{align: l.Alignment})
	}
	size := l.textSize()
	style := fyne.TextStyle{Bold: true}

	full := l.fullText()
	if full == "" {
		full = " "
	}

	var baseColor, activeColor fyne.ThemeColorName
	var prefix string
	hovered := l.hovered && l.HoveredColorName != ""
	if hovered {
		baseColor = l.HoveredColorName
		activeColor = l.HoveredColorName
		prefix = ""
	} else {
		baseColor = l.inactiveColor()
		activeColor = l.ColorName
		prefix = l.sungPrefix()
	}

	// base: the whole line, drawn in the inactive color using a single shaping
	// run so all glyphs share one baseline regardless of font fallback.
	if l.baseText == nil {
		l.baseText = canvas.NewText(full, l.textColor(baseColor))
		l.baseText.TextSize = size
		l.baseText.TextStyle = style
		l.baseText.Alignment = fyne.TextAlignLeading
	} else {
		l.baseText.Text = full
		l.baseText.TextSize = size
		l.baseText.TextStyle = style
		l.baseText.Color = l.textColor(baseColor)
	}

	multi := len(l.words) > 1
	var clipWidth float32
	lineHeight := l.baseText.MinSize().Height
	if multi {
		// active overlay: the same whole-line string, so shaping matches the
		// base exactly (no ghosting); a clip reveals only the sung prefix.
		if l.activeText == nil {
			l.activeText = canvas.NewText(full, l.textColor(activeColor))
			l.activeText.TextSize = size
			l.activeText.TextStyle = style
			l.activeText.Alignment = fyne.TextAlignLeading
		} else {
			l.activeText.Text = full
			l.activeText.TextSize = size
			l.activeText.TextStyle = style
			l.activeText.Color = l.textColor(activeColor)
		}
		if prefix != "" {
			pm := canvas.NewText(prefix, l.textColor(activeColor))
			pm.TextSize = size
			pm.TextStyle = style
			clipWidth = pm.MinSize().Width
		}
		if l.activeClip == nil {
			l.activeClip = newLyricClip(l.activeText)
		}
		l.textView.Objects = []fyne.CanvasObject{l.baseText, l.activeClip}
	} else {
		l.textView.Objects = []fyne.CanvasObject{l.baseText}
	}

	if ly, ok := l.textView.Layout.(*lyricLineLayout); ok {
		ly.align = l.Alignment
		ly.clipWidth = clipWidth
		ly.lineHeight = lineHeight
	}
	l.textView.Refresh()
}

func (l *lyricLine) Refresh() {
	l.updateTextView()
}

func (l *lyricLine) CreateRenderer() fyne.WidgetRenderer {
	l.updateTextView()
	return widget.NewSimpleRenderer(l.textView)
}

// lyricClip is a rectangular clipping region. It implements fyne.Scrollable so
// that Fyne 2.5.x drivers clip to its bounds, and its renderer implements
// IsClip so newer drivers (e.g. the supersonic fyne fork) clip as well.
type lyricClip struct {
	widget.BaseWidget
	Content fyne.CanvasObject
}

func newLyricClip(content fyne.CanvasObject) *lyricClip {
	c := &lyricClip{Content: content}
	c.ExtendBaseWidget(c)
	return c
}

func (c *lyricClip) Scrolled(*fyne.ScrollEvent) {}

func (c *lyricClip) CreateRenderer() fyne.WidgetRenderer {
	return &lyricClipRenderer{c: c, objects: []fyne.CanvasObject{c.Content}}
}

type lyricClipRenderer struct {
	c       *lyricClip
	objects []fyne.CanvasObject
}

func (r *lyricClipRenderer) Destroy() {}

func (r *lyricClipRenderer) Layout(fyne.Size) {
	r.objects[0].Resize(r.objects[0].MinSize())
	r.objects[0].Move(fyne.NewPos(0, 0))
}

func (r *lyricClipRenderer) MinSize() fyne.Size { return r.objects[0].MinSize() }

func (r *lyricClipRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *lyricClipRenderer) Refresh() {
	r.objects[0] = r.c.Content
	r.Layout(r.c.Size())
	r.objects[0].Refresh()
}

// IsClip marks this renderer as clipping. It is detected via a type assertion
// by drivers that support it.
func (r *lyricClipRenderer) IsClip() {}

// lyricLineLayout centers/aligns the base text and the clipped active overlay.
type lyricLineLayout struct {
	align      fyne.TextAlign
	clipWidth  float32
	lineHeight float32
}

func (l *lyricLineLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	var fullWidth float32
	if len(objects) > 0 {
		fullWidth = objects[0].MinSize().Width
	}
	var xStart float32
	switch l.align {
	case fyne.TextAlignCenter:
		xStart = (containerSize.Width - fullWidth) / 2
	case fyne.TextAlignTrailing:
		xStart = containerSize.Width - fullWidth
	}
	for i, o := range objects {
		min := o.MinSize()
		if i == 0 {
			o.Resize(min)
		} else {
			h := l.lineHeight
			if h == 0 {
				h = min.Height
			}
			o.Resize(fyne.NewSize(l.clipWidth, h))
		}
		o.Move(fyne.NewPos(xStart, 0))
	}
}

func (l *lyricLineLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objects {
		min := o.MinSize()
		if min.Width > w {
			w = min.Width
		}
		if min.Height > h {
			h = min.Height
		}
	}
	return fyne.NewSize(w, h)
}
