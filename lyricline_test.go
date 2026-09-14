package fynelyrics

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

func collectTexts(o fyne.CanvasObject, out *[]*canvas.Text) {
	if t, ok := o.(*canvas.Text); ok {
		*out = append(*out, t)
		return
	}
	if w, ok := o.(fyne.Widget); ok {
		for _, c := range test.WidgetRenderer(w).Objects() {
			collectTexts(c, out)
		}
		return
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			collectTexts(ch, out)
		}
	}
}

func TestKaraokeLineRendersOnOneRow(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	l := NewLyricsViewer()
	l.ActiveLyricPosition = ActiveLyricPositionUpperMiddle
	lines := []string{
		"<00:00.00>狂<00:00.36>人<00:00.72>日<00:01.09>记<00:01.45> <00:01.82>(<00:02.18>Live)",
		"<00:07.29>词<00:09.11>：<00:10.93>林<00:12.75>夕",
	}
	l.SetLyrics(lines, true)
	l.SetPlayTime(0, true)
	l.Resize(fyne.NewSize(640, 480))

	ll1 := l.vbox.Objects[1].(*lyricLine)
	if len(ll1.words) < 2 {
		t.Fatalf("expected multi-word line, got %d", len(ll1.words))
	}
	var texts []*canvas.Text
	collectTexts(ll1, &texts)
	if len(texts) != 2 {
		t.Fatalf("expected base + active texts, got %d", len(texts))
	}
	base, active := texts[0], texts[1]
	if base.Text != ll1.fullText() {
		t.Errorf("base text %q != full line %q", base.Text, ll1.fullText())
	}
	// both layers must use the exact same string so their shaping (and thus the
	// baseline of every glyph) matches and there is no ghosting
	if active.Text != base.Text {
		t.Errorf("active text %q must equal base text %q to avoid ghosting", active.Text, base.Text)
	}
	if base.Position().Y != active.Position().Y || base.Position().X != active.Position().X {
		t.Errorf("base %v and active %v not overlaid", base.Position(), active.Position())
	}
	if ll1.activeClip == nil {
		t.Errorf("expected a clipping overlay for a multi-word line")
	}
}
