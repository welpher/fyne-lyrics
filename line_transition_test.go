package fynelyrics

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestLineTransitionKeepsPerWordProgress(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	starts := []float64{21.88, 25.14}
	l := NewLyricsViewer()
	l.LineStarts = starts
	l.ActiveLyricPosition = ActiveLyricPositionUpperMiddle
	// old line: 3 words, all sung at boundary; new line: only 2 words, and its
	// second word is >1s away so schedulePredictive schedules nothing
	lines := []string{
		"<00:21.88>愿<00:22.41>望<00:24.17>多",
		"<00:25.14>年<00:30.00>少",
	}
	l.SetLyrics(lines, true)
	l.CreateRenderer()
	l.Resize(fyne.NewSize(640, 480))

	// fully sing the old line, then cross into the new line at its start
	l.advanceTo(25.14)

	if l.currentLine != 2 {
		t.Fatalf("currentLine = %d, want 2", l.currentLine)
	}
	ll := l.vbox.Objects[2].(*lyricLine)
	if ll.activeWordIdx != 0 {
		t.Errorf("line 2 activeWordIdx = %d, want 0 (only first word sung at line start)", ll.activeWordIdx)
	}
	if ll.activeWordIdx == len(ll.words)-1 {
		t.Errorf("line 2 fully highlighted (activeWordIdx=%d of %d words); want per-word progress", ll.activeWordIdx, len(ll.words))
	}
}
