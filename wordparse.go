package fynelyrics

import (
	"regexp"
	"strconv"
)

type word struct {
	text  string
	start float64
}

var wordTagRe = regexp.MustCompile(`<(\d{2}):(\d{2})\.(\d{2})>`)

func parseWords(line string) []word {
	matches := wordTagRe.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return []word{{text: line, start: -1}}
	}
	var words []word
	if matches[0][0] > 0 {
		words = append(words, word{text: line[:matches[0][0]], start: -1})
	}
	for i, m := range matches {
		mm, _ := strconv.Atoi(line[m[2]:m[3]])
		ss, _ := strconv.Atoi(line[m[4]:m[5]])
		cc, _ := strconv.Atoi(line[m[6]:m[7]])
		start := float64(mm)*60 + float64(ss) + float64(cc)/100
		textStart := m[1]
		textEnd := len(line)
		if i+1 < len(matches) {
			textEnd = matches[i+1][0]
		}
		text := line[textStart:textEnd]
		if text == "" {
			continue
		}
		words = append(words, word{text: text, start: start})
	}
	return words
}
