package fynelyrics

import "testing"

func TestParseWords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []word
	}{
		{
			name: "tagged",
			in:   "<00:00.00>狂<00:00.36>人<00:00.72>日",
			want: []word{{"狂", 0.0}, {"人", 0.36}, {"日", 0.72}},
		},
		{
			name: "no tags",
			in:   "普通歌词行",
			want: []word{{"普通歌词行", -1}},
		},
		{
			name: "leading untagged",
			in:   "abc <01:05.05>def",
			want: []word{{"abc ", -1}, {"def", 65.05}},
		},
		{
			name: "empty tag text skipped",
			in:   "<00:00.00><00:00.10>人",
			want: []word{{"人", 0.10}},
		},
		{
			name: "minute wrap",
			in:   "<01:02.03>字",
			want: []word{{"字", 62.03}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseWords(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len: got %d want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("seg %d: got %+v want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}
