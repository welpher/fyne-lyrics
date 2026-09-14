package fynelyrics

import (
	"os"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestMain(m *testing.M) {
	test.NewApp()
	os.Exit(m.Run())
}

func TestSetLyricsParsesWordsAndLineStarts(t *testing.T) {
	l := NewLyricsViewer()
	lines := []string{
		"<00:00.00>狂<00:00.36>人",
		"<00:07.29>词：佚名",
	}
	l.SetLyrics(lines, true)
	if len(l.lineWords) != 2 {
		t.Fatalf("lineWords: got %d want 2", len(l.lineWords))
	}
	if got := len(l.lineWords[0]); got != 2 {
		t.Errorf("line0 words: got %d want 2", got)
	}
	if l.lineStarts[0] != 0 || l.lineStarts[1] != 7.29 {
		t.Errorf("lineStarts: got %v want [0, 7.29]", l.lineStarts)
	}
}

func TestSetLyricsUsesLineStartsWhenProvided(t *testing.T) {
	l := NewLyricsViewer()
	l.LineStarts = []float64{1, 2, 3}
	l.SetLyrics([]string{"a", "b", "c"}, true)
	if l.lineStarts[0] != 1 || l.lineStarts[2] != 3 {
		t.Errorf("expected provided lineStarts, got %v", l.lineStarts)
	}
}
