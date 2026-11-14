//go:build testdata

package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func adminScope(t *testing.T, congregation, user string) Scope {
	t.Helper()
	s := scopeFor(t, newTestApp(t), congregation)
	s.Role, s.UserID = "administrator", user
	return s
}

func propose(t *testing.T, s Scope, tool Tool, args string) Proposal {
	t.Helper()
	r, err := tool.Run(s, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s(%s): %v", tool.Name, args, err)
	}
	if r.Proposal == nil {
		t.Fatalf("%s(%s) prepared no proposal", tool.Name, args)
	}
	fillProposal(r.Proposal, tool.Name)
	return *r.Proposal
}

func wantArgError(t *testing.T, err error, contains string) {
	t.Helper()
	var bad argError
	if !errors.As(err, &bad) || !strings.Contains(bad.msg, contains) {
		t.Fatalf("err = %v, want an argument error containing %q", err, contains)
	}
}

func TestProposeRoleChange(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	token := newPseudonyms(alphaCong).token("Alpha Conductor")

	p := propose(t, s, proposeRoleChange, `{"person": "`+token+`", "role": "read_only"}`)
	if p.Kind != "role_change" || p.From != "conductor" || p.To != "read_only" || len(p.Warnings) != 0 || p.Blocked != "" {
		t.Fatalf("proposal = %+v", p)
	}
	if len(p.Candidates) != 1 || p.Candidates[0].RoleID != "testrolexcng01b" || p.Candidates[0].Email != "conductor@alpha.test" {
		t.Fatalf("candidates = %+v", p.Candidates)
	}

	revoke := propose(t, s, proposeRoleChange, `{"person": "Alpha Conductor", "role": "none"}`)
	if revoke.Kind != "role_revoke" || revoke.To != "" {
		t.Fatalf("removal = %+v", revoke)
	}

	_, err := proposeRoleChange.Run(s, json.RawMessage(`{"person": "`+token+`", "role": "conductor"}`))
	wantArgError(t, err, "already has the role")
	_, err = proposeRoleChange.Run(s, json.RawMessage(`{"person": "Beta Admin", "role": "conductor"}`))
	wantArgError(t, err, "nobody with access matches")
}

func TestProposeRoleChange_LastAdminIsBlockedAndWarned(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeRoleChange, `{"person": "Alpha Admin", "role": "none"}`)
	if p.Blocked != "last_admin" || strings.Join(p.Warnings, ",") != "self,removes_admin" {
		t.Fatalf("proposal = %+v, want blocked as last admin with self and removes_admin warnings", p)
	}
}

func TestProposeRoleChange_SameNamedPeopleEachCarryTheirOwnRole(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	admin, err := s.App.FindRecordById("users", "testuseralpha01")
	if err != nil {
		t.Fatal(err)
	}
	admin.Set("name", "Alpha Conductor")
	if err := s.App.Save(admin); err != nil {
		t.Fatal(err)
	}

	p := propose(t, s, proposeRoleChange, `{"person": "Alpha Conductor", "role": "read_only"}`)
	if len(p.Candidates) != 2 || p.From != "" || len(p.Warnings) != 0 || p.Blocked != "" {
		t.Fatalf("proposal = %+v, want two candidates and nothing decided for the card yet", p)
	}
	byRole := map[string]Candidate{}
	for _, c := range p.Candidates {
		byRole[c.Role] = c
	}
	if c := byRole["conductor"]; len(c.Warnings) != 0 || c.Blocked != "" {
		t.Errorf("conductor candidate = %+v", c)
	}
	if c := byRole["administrator"]; strings.Join(c.Warnings, ",") != "self,removes_admin" || c.Blocked != "last_admin" {
		t.Errorf("administrator candidate = %+v", c)
	}

	// Someone who already has the new role is left out.
	only := propose(t, s, proposeRoleChange, `{"person": "Alpha Conductor", "role": "conductor"}`)
	if len(only.Candidates) != 1 || only.From != "administrator" || only.Blocked != "last_admin" {
		t.Fatalf("proposal = %+v, want only the administrator", only)
	}
}

func TestProposeRoleGrant(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")

	p := propose(t, s, proposeRoleGrant, `{"search": "beta", "role": "conductor"}`)
	names := []string{}
	for _, c := range p.Candidates {
		names = append(names, c.Name)
		if c.RoleID != "" {
			t.Error("a grant candidate has no role yet")
		}
	}
	if p.Kind != "role_grant" || p.To != "conductor" || strings.Join(names, ",") != "Beta Admin,Beta Xcong" {
		t.Fatalf("proposal = %+v", p)
	}

	byEmail := propose(t, s, proposeRoleGrant, `{"search": "xcong@", "role": "read_only"}`)
	if len(byEmail.Candidates) != 1 || byEmail.Candidates[0].Email != "xcong@beta.test" {
		t.Fatalf("by email = %+v", byEmail.Candidates)
	}

	_, err := proposeRoleGrant.Run(s, json.RawMessage(`{"search": "Alpha", "role": "read_only"}`))
	wantArgError(t, err, "already has a role")
	_, err = proposeRoleGrant.Run(s, json.RawMessage(`{"search": "Nobody At All", "role": "read_only"}`))
	wantArgError(t, err, "sign up")
	_, err = proposeRoleGrant.Run(s, json.RawMessage(`{"search": "be", "role": "read_only"}`))
	wantArgError(t, err, "at least 3")
	_, err = proposeRoleGrant.Run(s, json.RawMessage(`{"search": "%%%", "role": "read_only"}`))
	wantArgError(t, err, "no account matches")
}

func TestAsk_ProposalReachesThePanelButNotTheModel(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"propose_role_grant", `{"search":"Beta","role":"read_only"}`}),
		final("The change is ready for you to confirm on the card below.", true),
	}}
	resp, err := Ask(context.Background(), chat, s, Request{Question: "Give the Beta people read-only access"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Proposals) != 1 || len(resp.Proposals[0].Candidates) != 2 {
		t.Fatalf("proposals = %+v", resp.Proposals)
	}
	sent := chat.sent()
	if strings.Contains(sent, "beta.test") || strings.Contains(sent, "testuserbeta") {
		t.Fatalf("a candidate's email or id reached the model: %s", sent)
	}
	if resp.Steps[0].Result != nil {
		t.Error("a proposal is not a table")
	}
}

func TestCards_KeepTheWholeProposalToConfirmLater(t *testing.T) {
	cards := Cards([]Proposal{{
		ID: "p1", Kind: "role_grant", Previous: "before",
		Candidates: []Candidate{{UserID: "u1", Name: "Bro Lee", Email: "lee@example.com"}},
		Items:      []Item{{ID: "h1", Label: "Blk 100A 12", Detail: `{"notes":"dog"}`}},
	}})
	got := cards[0].Proposal
	if got.Previous != "before" || got.Candidates[0].Email != "lee@example.com" || got.Items[0].Detail != `{"notes":"dog"}` {
		t.Fatalf("lost what confirming needs: %+v", got)
	}
	if cards[0].Outcome != "" {
		t.Fatalf("a new card has an outcome: %+v", cards[0])
	}
}
