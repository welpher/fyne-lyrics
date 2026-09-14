package main

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynelyrics "github.com/supersonic-app/fyne-lyrics"
)

// lyricData is a public-domain sample: Li Bai's "Quiet Night Thoughts"
// (Tang dynasty) with an original English rendering, in enhanced-LRC
// karaoke format. It intentionally mixes Chinese and English so the demo
// exercises per-word highlighting across font fallback.
var lyricData = `[ti:Quiet Night Thoughts]
[ar:Li Bai]
[al:Public Domain]
[by:]

[00:00.00]<00:00.00>静<00:00.40>夜<00:00.80>思<00:01.20> <00:01.60>-<00:02.00> <00:02.40>Li<00:02.80> <00:03.20>Bai<00:03.60>
[00:04.00]<00:04.00>床<00:04.40>前<00:04.80>明<00:05.20>月<00:05.60>光<00:06.00>
[00:06.40]<00:06.40>疑<00:06.80>是<00:07.20>地<00:07.60>上<00:08.00>霜<00:08.40>
[00:08.80]<00:08.80>举<00:09.20>头<00:09.60>望<00:10.00>明<00:10.40>月<00:10.80>
[00:11.20]<00:11.20>低<00:11.60>头<00:12.00>思<00:12.40>故<00:12.80>乡<00:13.20>
[00:13.60]<00:13.60>Moonlight<00:14.20> <00:14.60>falls<00:15.00> <00:15.40>by<00:15.80> <00:16.20>my<00:16.60> <00:17.00>bed<00:17.40>
[00:17.80]<00:17.80>like<00:18.20> <00:18.60>frost<00:19.00> <00:19.40>upon<00:19.80> <00:20.20>the<00:20.60> <00:21.00>ground<00:21.40>
[00:21.80]<00:21.80>I<00:22.20> <00:22.60>raise<00:23.00> <00:23.40>my<00:23.80> <00:24.20>head<00:24.60> <00:25.00>to<00:25.40> <00:25.80>the<00:26.20> <00:26.60>moon<00:27.00>
[00:27.40]<00:27.40>I<00:27.80> <00:28.20>lower<00:28.60> <00:29.00>it<00:29.40> <00:29.80>and<00:30.20> <00:30.60>miss<00:31.00> <00:31.40>home<00:31.80>
[00:32.20]<00:32.20>回<00:32.60>乡<00:33.00> <00:33.40>-<00:33.80> <00:34.20>go<00:34.60> <00:35.00>home<00:35.40>
`

var lineRe = regexp.MustCompile(`^\[(\d{2}):(\d{2}\.\d{1,3})\](.*)$`)
var metaRe = regexp.MustCompile(`^\[(ti|ar|al|by):`)

func main() {
	a := app.New()
	w := a.NewWindow("Karaoke Demo - Quiet Night Thoughts")

	var texts []string
	var starts []float64
	for _, raw := range strings.Split(lyricData, "\n") {
		raw = strings.TrimRight(raw, "\r")
		if raw == "" || metaRe.MatchString(raw) {
			continue
		}
		m := lineRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		mm, _ := strconv.Atoi(m[1])
		sec, _ := strconv.ParseFloat(m[2], 64)
		starts = append(starts, float64(mm)*60+sec)
		texts = append(texts, m[3])
	}
	if len(texts) != 10 {
		log.Fatalf("expected 10 lyric lines from the sample, got %d", len(texts))
	}

	viewer := fynelyrics.NewLyricsViewer()
	viewer.ActiveLyricPosition = fynelyrics.ActiveLyricPositionUpperMiddle
	viewer.Alignment = fyne.TextAlignCenter
	viewer.LineStarts = starts
	viewer.SetLyrics(texts, true)

	songDur := 5.0
	if len(starts) > 0 {
		songDur = starts[len(starts)-1] + 5
	}
	start := time.Now()
	var lastPushed float64
	viewer.OnLyricTapped = func(lineNum int) {
		if lineNum >= 1 && lineNum <= len(starts) {
			start = time.Now().Add(-time.Duration(starts[lineNum-1] * float64(time.Second)))
			lastPushed = 0
			viewer.SetPlayTime(starts[lineNum-1], true)
		}
	}

	w.SetContent(viewer)
	w.Resize(fyne.NewSize(640, 480))

	const pushInterval = 250 * time.Millisecond
	anim := fyne.NewAnimation(time.Duration(songDur*float64(time.Second)), func(f float32) {
		cur := time.Since(start).Seconds()
		if cur > songDur {
			cur = songDur
		}
		if cur-lastPushed >= pushInterval.Seconds() || cur >= songDur {
			viewer.SetPlayTime(cur, false)
			lastPushed = cur
		}
	})
	anim.Curve = fyne.AnimationLinear
	anim.Start()

	w.ShowAndRun()
}
