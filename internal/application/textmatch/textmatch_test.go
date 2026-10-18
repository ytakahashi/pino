package textmatch

import "testing"

func TestQueryMatchesWithSmartcase(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		term string
		text string
		want bool
	}{
		"lowercase ignores case":        {term: "port", text: "ServerPort", want: true},
		"uppercase preserves case":      {term: "Port", text: "ServerPort", want: true},
		"uppercase refuses other case":  {term: "Port", text: "serverport"},
		"a substring is enough":         {term: "serve", text: "server", want: true},
		"unrelated text does not match": {term: "port", text: "server"},
		"Unicode lowercase folds":       {term: "ä", text: "Ärger", want: true},
		"Unicode uppercase keeps case":  {term: "Ä", text: "ärger"},
		"empty term matches text":       {text: "anything", want: true},
		"empty term matches empty text": {want: true},
		"nonempty term needs text":      {term: "port"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := New(tc.term).Match(tc.text); got != tc.want {
				t.Errorf("New(%q).Match(%q) = %t, want %t", tc.term, tc.text, got, tc.want)
			}
		})
	}
}

func TestQueryPreservesTheOriginalTerm(t *testing.T) {
	t.Parallel()

	for _, term := range []string{"", "port", " Port Ä "} {
		t.Run(term, func(t *testing.T) {
			t.Parallel()

			if got := New(term).Text(); got != term {
				t.Errorf("New(%q).Text() = %q, want %q", term, got, term)
			}
		})
	}
}

func TestZeroQueryMatchesEveryText(t *testing.T) {
	t.Parallel()

	var q Query
	if got := q.Text(); got != "" {
		t.Errorf("zero Query.Text() = %q, want empty", got)
	}

	for _, text := range []string{"", "anything"} {
		if !q.Match(text) {
			t.Errorf("zero Query.Match(%q) = false, want true", text)
		}
	}
}
