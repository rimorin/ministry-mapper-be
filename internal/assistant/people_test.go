package assistant

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNameMask(t *testing.T) {
	people := newPseudonyms("cong1")
	tok := people.token
	cases := []struct {
		name  string
		text  string
		names []string
		want  string
	}{
		{"multi-word name in any case", "which map did bro tan get?", []string{"Bro Tan"}, "which map did " + tok("Bro Tan") + " get?"},
		{"extra spaces between words", "did Bro   Tan come", []string{"Bro Tan"}, "did " + tok("Bro Tan") + " come"},
		{"single word matched only as written", "Who will come? Did Will come?", []string{"Will"}, "Who will come? Did " + tok("Will") + " come?"},
		{"whole words only", "which homes hit the limit? Did Lim come?", []string{"Lim"}, "which homes hit the limit? Did " + tok("Lim") + " come?"},
		{"longest name first", "Bro Tan Ah Kow came", []string{"Bro Tan", "Bro Tan Ah Kow"}, tok("Bro Tan Ah Kow") + " came"},
		{"name without ASCII letters", "陈伟来了吗", []string{"陈伟来"}, tok("陈伟来") + "了吗"},
		{"too short to match safely", "Al came", []string{"Al"}, "Al came"},
		{"part of a new name the admin typed", "Give it to Bro QA Test", []string{"Test"}, "Give it to Bro QA Test"},
		{"first word of a longer name", "Did Tan Ah Kow come?", []string{"Tan"}, "Did Tan Ah Kow come?"},
		{"honorific is not part of the name", "what did Sis Ong do? Mr. Ong too", []string{"Ong"}, "what did Sis " + tok("Ong") + " do? Mr. " + tok("Ong") + " too"},
		{"names in a list", "Did Lim, Ong and Tan come", []string{"Lim", "Ong", "Tan"}, "Did " + tok("Lim") + ", " + tok("Ong") + " and " + tok("Tan") + " come"},
		{"known multi-word name inside a longer one", "give it to Bro Tan Ah Kow", []string{"Bro Tan"}, "give it to Bro Tan Ah Kow"},
		{"regex characters in a name", "did J. (Joe) Ong come", []string{"J. (Joe) Ong"}, "did " + tok("J. (Joe) Ong") + " come"},
	}
	for _, c := range cases {
		if got := newNameMask(c.names).apply(c.text, people); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if strings.Contains(newNameMask([]string{"Bro Tan"}).apply("did bro tan come", people), "tan") {
		t.Error("the name must not survive masking")
	}
}

func TestClipKeepsWholeCharacters(t *testing.T) {
	long := strings.Repeat("த", 600) // 3 bytes each, so byte 1500 falls mid-character
	got := clip(long)
	if !utf8.ValidString(got) || len(got) > 1500 || len(got) < 1497 {
		t.Fatalf("clip gave %d bytes, valid=%v", len(got), utf8.ValidString(got))
	}
	if clip("short") != "short" {
		t.Error("short text must stay as it is")
	}
}
