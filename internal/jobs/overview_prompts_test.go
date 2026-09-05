package jobs

import (
	"strings"
	"testing"
)

// The three overview prompts share the report's writing rules and never hand the
// model the name of the person who wrote a note or message.
func TestOverviewPrompts_PlainLanguageAndNoNames(t *testing.T) {
	notes := []notesData{{Publisher: "Sis Tan", Message: "Large dog in the yard.", Date: "02 Aug 2026", Address: "Blk 412 #05-12"}}
	messages := []messagesData{{Publisher: "Bro Lim", Message: "Unit 07-03 is missing from the map.", Date: "02 Aug 2026", MapName: "Blk 412"}}

	cases := map[string]struct{ system, user string }{}
	sys, user := BuildNotesPrompt(notes, "Alpha")
	cases["notes"] = struct{ system, user string }{sys, user}
	sys, user = BuildMessagesPrompt(messages, "Alpha")
	cases["messages"] = struct{ system, user string }{sys, user}
	sys, user = BuildInstructionsPrompt(messages, "T01 - Blk 412")
	cases["instructions"] = struct{ system, user string }{sys, user}

	for name, c := range cases {
		if !strings.Contains(c.system, "Sentences of 15 words or fewer") || !strings.Contains(c.system, "Never invent") {
			t.Errorf("%s: system prompt lacks the shared writing rules", name)
		}
		for _, person := range []string{"Sis Tan", "Bro Lim", "Publisher:", "From:"} {
			if strings.Contains(c.user, person) {
				t.Errorf("%s: user message must not carry the author (%q)", name, person)
			}
		}
	}
	if !strings.Contains(cases["notes"].user, "Large dog in the yard.") || !strings.Contains(cases["notes"].user, "Blk 412 #05-12") {
		t.Error("notes prompt must carry the note text and address")
	}
	if !strings.Contains(cases["messages"].system, `"todo"`) {
		t.Error("messages prompt must ask for the todo list")
	}
}

func TestGenerateOverviews_SkipSmallLists(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "would-fail-if-called")
	two := []messagesData{{Message: "a"}, {Message: "b"}}
	if s := generateMessagesAISummary(two, "Alpha"); s.Available {
		t.Error("two messages must not be summarised")
	}
	if s := generateInstructionsAISummary(two, "Blk 412"); s.Available {
		t.Error("two instructions must not be summarised")
	}
	if s := generateNotesAISummary([]notesData{{Message: "a"}}, "Alpha"); s.Available {
		t.Error("one note must not be summarised")
	}
}
