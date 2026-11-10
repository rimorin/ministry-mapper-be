//go:build testdata

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"ministry-mapper/internal/assistant"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"github.com/openai/openai-go/v3/responses"
)

// reply turns a scripted Responses reply into the SDK type, as the API would,
// sending its text to onText first.
func reply(raw string, onText func(string)) (*responses.Response, error) {
	var r responses.Response
	err := json.Unmarshal([]byte(raw), &r)
	if onText != nil && err == nil {
		onText(r.OutputText())
	}
	return &r, err
}

// answeringChat answers every question at once without calling a function.
type answeringChat struct{}

func (answeringChat) Respond(_ context.Context, _ responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	return reply(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","id":"msg_1",
		"role":"assistant","status":"completed","content":[{"type":"output_text","annotations":[],
		"text":"{\"answer\":\"Nothing to report.\",\"answered\":true}"}]}],
		"usage":{"input_tokens":120,"output_tokens":8}}`, onText)
}

// settingsThenAnswerChat calls one function, then answers from its result.
type settingsThenAnswerChat struct{ calls int }

func (c *settingsThenAnswerChat) Respond(_ context.Context, _ responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	c.calls++
	if c.calls > 1 {
		return reply(`{"id":"resp_2","object":"response","status":"completed","output":[{"type":"message","id":"msg_1",
			"role":"assistant","status":"completed","content":[{"type":"output_text","annotations":[],
			"text":"{\"answer\":\"Homes count after 3 tries.\",\"answered\":true}"}]}]}`, onText)
	}
	return reply(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"function_call",
		"id":"fc_1","call_id":"call_1","name":"congregation_settings","arguments":"{}","status":"completed"}]}`, onText)
}

// askingChat asks the admin for a detail instead of answering.
type askingChat struct{}

func (askingChat) Respond(_ context.Context, _ responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	return reply(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","id":"msg_1",
		"role":"assistant","status":"completed","content":[{"type":"output_text","annotations":[],
		"text":"{\"answer\":\"Which map?\",\"answered\":true,\"needs_detail\":true,\"suggestions\":[]}"}]}]}`, onText)
}

// promptChat records the instructions the model was given, then answers.
type promptChat struct{ prompt string }

func (c *promptChat) Respond(_ context.Context, params responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	raw, _ := json.Marshal(params)
	c.prompt = string(raw)
	return answeringChat{}.Respond(context.Background(), params, onText)
}

// failingChat stands in for a model that is down.
type failingChat struct{}

func (failingChat) Respond(context.Context, responses.ResponseNewParams, func(string)) (*responses.Response, error) {
	return nil, errors.New("model unavailable")
}

// cancelledChat stands in for a request the admin stopped.
type cancelledChat struct{}

func (cancelledChat) Respond(context.Context, responses.ResponseNewParams, func(string)) (*responses.Response, error) {
	return nil, context.Canceled
}

func useChatter(t *testing.T, chat assistant.Responder) {
	t.Helper()
	previous := assistant.NewResponder
	assistant.NewResponder = func() assistant.Responder { return chat }
	t.Cleanup(func() { assistant.NewResponder = previous })
}

func jsonHeaders(token string) map[string]string {
	return map[string]string{"Content-Type": "application/json", "Authorization": token}
}

// assistantTokens signs in the alpha admin, the alpha read-only user and the
// beta admin.
func assistantTokens(t *testing.T) (admin, readOnly, betaAdmin string) {
	t.Helper()
	tokens := make([]string, 3)
	for i, email := range []string{"admin@alpha.test", "readonly@alpha.test", "admin@beta.test"} {
		token, err := generateToken(email)
		if err != nil {
			t.Fatal(err)
		}
		tokens[i] = token
	}
	return tokens[0], tokens[1], tokens[2]
}

func TestAssistantAsk(t *testing.T) {
	adminToken, readOnlyToken, betaAdminToken := assistantTokens(t)

	ask := func(question string) *strings.Reader {
		body, _ := json.Marshal(map[string]any{"congregation": "testcongalpha01", "question": question, "locale": "en"})
		return strings.NewReader(string(body))
	}
	// loggedWithStatus checks a question that got no answer still stays in history.
	loggedWithStatus := func(status string) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			q, err := app.FindFirstRecordByFilter("assistant_questions", "congregation = {:c}", dbx.Params{"c": "testcongalpha01"})
			if err != nil {
				t.Fatal(err)
			}
			if q.GetString("status") != status || q.GetString("conversation") == "" {
				t.Errorf("status = %q, conversation = %q", q.GetString("status"), q.GetString("conversation"))
			}
		}
	}
	questionsLogged := func(want int) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			var n struct {
				N int `db:"n"`
			}
			if err := app.DB().NewQuery("SELECT COUNT(*) AS n FROM assistant_questions").One(&n); err != nil {
				t.Fatal(err)
			}
			if n.N != want {
				t.Fatalf("assistant_questions = %d, want %d", n.N, want)
			}
		}
	}

	enabled := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Who are the admins?"),
			Headers:         map[string]string{"Content-Type": "application/json"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:            "another congregation's administrator is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Who are the admins?"),
			Headers:         jsonHeaders(betaAdminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Congregation access required."`},
			AfterTestFunc:   questionsLogged(0),
		},
		{
			Name:            "an empty question is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("   "),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Question is required."`},
		},
		{
			Name:            "an overlong question is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask(strings.Repeat("a", 501)),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Question is too long."`},
		},
		{
			Name:            "a location out of range is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            strings.NewReader(`{"congregation":"testcongalpha01","question":"Near me?","location":{"lat":200,"lng":103.8}}`),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Location is out of range."`},
		},
		{
			Name:            "a read-only user can ask, and the question is logged",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Anything to report?"),
			Headers:         jsonHeaders(readOnlyToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"answer":"Nothing to report."`, `"grounded":true`, `"steps":[]`, `"questionId":"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				questionsLogged(1)(t, app, res)
				q, err := app.FindFirstRecordByFilter("assistant_questions", "congregation = {:c}", dbx.Params{"c": "testcongalpha01"})
				if err != nil {
					t.Fatal(err)
				}
				if q.GetString("question") != "Anything to report?" || q.GetInt("prompt_tokens") != 120 || !q.GetBool("answered") ||
					q.GetString("answer") != "Nothing to report." {
					t.Errorf("logged = %v", q.FieldsData())
				}
			},
		},
		{
			Name:    "the daily limit is enforced per congregation",
			Method:  http.MethodPost,
			URL:     "/assistant/ask",
			Body:    ask("One more?"),
			Headers: jsonHeaders(adminToken),
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				for i := 0; i < 300; i++ {
					if _, err := app.DB().NewQuery(`INSERT INTO assistant_questions (id, congregation, question, created)
						VALUES (lower(hex(randomblob(8))), 'testcongalpha01', 'q', strftime('%Y-%m-%d %H:%M:%fZ', 'now'))`).Execute(); err != nil {
						t.Fatal(err)
					}
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  429,
			ExpectedContent: []string{`"status":429`},
		},
	}

	t.Run("enabled", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, answeringChat{})
		for _, scenario := range enabled {
			scenario.Test(t)
		}
	})

	t.Run("flag off", func(t *testing.T) {
		useChatter(t, answeringChat{})
		(&tests.ApiScenario{
			Name:            "the assistant is refused when its flag is off",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Who are the admins?"),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"The assistant is not enabled for this congregation."`},
		}).Test(t)
	})

	t.Run("logs a question the assistant asked back about", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, askingChat{})
		(&tests.ApiScenario{
			Name:            "needs_detail is logged, so it is not counted as unanswered",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Let's assign a map"),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"answer":"Which map?"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				q, err := app.FindFirstRecordByFilter("assistant_questions", "congregation = {:c}", dbx.Params{"c": "testcongalpha01"})
				if err != nil {
					t.Fatal(err)
				}
				if !q.GetBool("needs_detail") || !q.GetBool("answered") {
					t.Errorf("logged = %v", q.FieldsData())
				}
			},
		}).Test(t)
	})

	t.Run("streams each check and the answer's text before the response", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, &settingsThenAnswerChat{})
		(&tests.ApiScenario{
			Name:           "step and delta events arrive before the done event",
			Method:         http.MethodPost,
			URL:            "/assistant/ask",
			Body:           ask("How many tries count?"),
			Headers:        jsonHeaders(adminToken),
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`{"type":"step","step":{"tool":"congregation_settings"`,
				`{"type":"delta","text":"Homes count after 3 tries."}`,
				`{"type":"done","response":{"answer":"Homes count after 3 tries."`,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if ct := res.Header.Get("Content-Type"); ct != "application/x-ndjson" {
					t.Errorf("Content-Type = %q", ct)
				}
				questionsLogged(1)(t, app, res)
			},
		}).Test(t)
	})

	t.Run("uses the admin's view only within the congregation", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		for _, c := range []struct {
			name, territory, want, notWant string
		}{
			{"own territory and map", "testterralpha01", "VIEW: The admin has territory T01", ""},
			{"another congregation's territory", "testterrbeta001", "", "VIEW:"},
		} {
			chat := &promptChat{}
			useChatter(t, chat)
			body, _ := json.Marshal(map[string]any{
				"congregation": "testcongalpha01", "question": "How is this territory doing?", "locale": "en",
				"view": map[string]string{"territory": c.territory, "map": "testmapalpha01a"},
			})
			(&tests.ApiScenario{
				Name:            c.name,
				Method:          http.MethodPost,
				URL:             "/assistant/ask",
				Body:            strings.NewReader(string(body)),
				Headers:         jsonHeaders(adminToken),
				TestAppFactory:  setupTestApp,
				ExpectedStatus:  200,
				ExpectedContent: []string{`"type":"done"`},
				AfterTestFunc: func(t testing.TB, _ *tests.TestApp, _ *http.Response) {
					if c.want != "" && !strings.Contains(chat.prompt, c.want) {
						t.Errorf("prompt lacks %q", c.want)
					}
					if c.notWant != "" && strings.Contains(chat.prompt, c.notWant) {
						t.Errorf("prompt has %q: another congregation's territory leaked in", c.notWant)
					}
				},
			}).Test(t)
		}
	})

	t.Run("model failure mid-stream", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, failingChat{})
		(&tests.ApiScenario{
			Name:               "the stream ends with an error event, and the question stays as failed",
			Method:             http.MethodPost,
			URL:                "/assistant/ask",
			Body:               ask("Anything?"),
			Headers:            jsonHeaders(adminToken),
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`{"type":"error","message":"The assistant could not answer. Please try again."}`},
			NotExpectedContent: []string{`"type":"done"`},
			AfterTestFunc:      loggedWithStatus("failed"),
		}).Test(t)
	})

	t.Run("stopped by the admin", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, cancelledChat{})
		(&tests.ApiScenario{
			Name:               "a cancelled request ends quietly, and the question stays as stopped",
			Method:             http.MethodPost,
			URL:                "/assistant/ask",
			Body:               ask("Anything?"),
			Headers:            jsonHeaders(adminToken),
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			NotExpectedContent: []string{`"type":"error"`, `"type":"done"`},
			AfterTestFunc:      loggedWithStatus("stopped"),
		}).Test(t)
	})

	t.Run("no model key", func(t *testing.T) {
		t.Setenv("ASSISTANT_ENABLED", "true")
		useChatter(t, nil)
		(&tests.ApiScenario{
			Name:            "the assistant reports it is not configured",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            ask("Who are the admins?"),
			Headers:         jsonHeaders(adminToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  503,
			ExpectedContent: []string{`"The assistant is not configured."`},
		}).Test(t)
	})
}

func TestAssistantFeedback(t *testing.T) {
	adminToken, readOnlyToken, _ := assistantTokens(t)
	const question = "testquestion001"
	asked := func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		if _, err := app.DB().NewQuery(`INSERT INTO assistant_questions (id, congregation, user, question, created)
			VALUES ({:id}, 'testcongalpha01', 'testuseralpha01', 'Who came out?', strftime('%Y-%m-%d %H:%M:%fZ', 'now'))`).
			Bind(dbx.Params{"id": question}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	body := func(fields map[string]any) *strings.Reader {
		fields["question"] = question
		raw, _ := json.Marshal(fields)
		return strings.NewReader(string(raw))
	}
	withCards := func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
		asked(t, app, e)
		if _, err := app.DB().NewQuery(`UPDATE assistant_questions SET cards = {:cards} WHERE id = {:id}`).
			Bind(dbx.Params{"id": question, "cards": `[{"proposal":{"id":"00000000000000b1","kind":"share_link"},"outcome":""},` +
				`{"proposal":{"id":"00000000000000b2","kind":"role_change"},"outcome":""}]`}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	logged := func(check func(testing.TB, *core.Record)) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			q, err := app.FindRecordById("assistant_questions", question)
			if err != nil {
				t.Fatal(err)
			}
			check(t, q)
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:           "the asker rates the answer",
			Method:         http.MethodPost,
			URL:            "/assistant/feedback",
			Body:           body(map[string]any{"rating": -1}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: asked,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: logged(func(t testing.TB, q *core.Record) {
				if q.GetInt("rating") != -1 {
					t.Errorf("rating = %v", q.Get("rating"))
				}
			}),
		},
		{
			Name:           "each card keeps its own outcome",
			Method:         http.MethodPost,
			URL:            "/assistant/feedback",
			Body:           body(map[string]any{"proposal": "00000000000000b2", "outcome": "done", "person": "Bro Lee"}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: withCards,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: logged(func(t testing.TB, q *core.Record) {
				var cards []assistant.Card
				if err := q.UnmarshalJSONField("cards", &cards); err != nil || len(cards) != 2 {
					t.Fatalf("cards = %s (%v)", q.GetString("cards"), err)
				}
				if cards[0].Outcome != "" || cards[1].Outcome != "done" || cards[1].Person != "Bro Lee" {
					t.Errorf("cards = %+v; only the second changed", cards)
				}
			}),
		},
		{
			Name:   "a link card keeps the link it created",
			Method: http.MethodPost,
			URL:    "/assistant/feedback",
			// A real share link id: 25 characters, mixed case.
			Body:           body(map[string]any{"proposal": "00000000000000b1", "outcome": "done", "record": "54zMZSgiPrir1HDKDpZF5tN4Z", "label": "Blk 100A"}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: withCards,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: logged(func(t testing.TB, q *core.Record) {
				var cards []assistant.Card
				_ = q.UnmarshalJSONField("cards", &cards)
				if cards[0].Record != "54zMZSgiPrir1HDKDpZF5tN4Z" || cards[0].Label != "Blk 100A" {
					t.Errorf("card = %+v", cards[0])
				}
			}),
		},
		{
			Name:            "a malformed link id is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"proposal": "00000000000000b1", "outcome": "done", "record": "../etc"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  withCards,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:            "an unknown card is not found",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"proposal": "00000000000000ff", "outcome": "done"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  withCards,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:           "a thumbs down keeps its reason",
			Method:         http.MethodPost,
			URL:            "/assistant/feedback",
			Body:           body(map[string]any{"rating": -1, "reason": "wrong_figures"}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: asked,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: logged(func(t testing.TB, q *core.Record) {
				if q.GetInt("rating") != -1 || q.GetString("reason") != "wrong_figures" {
					t.Errorf("rating = %v, reason = %q", q.Get("rating"), q.GetString("reason"))
				}
			}),
		},
		{
			Name:            "a reason without a thumbs down is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"rating": 1, "reason": "wrong_figures"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  asked,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:            "an unknown reason is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"rating": -1, "reason": "too slow"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  asked,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:            "someone else cannot rate the question",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"rating": 1}),
			Headers:         jsonHeaders(readOnlyToken),
			BeforeTestFunc:  asked,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:            "a rating outside -1 to 1 is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"rating": 5}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  asked,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:            "an unknown outcome is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"proposal": "00000000000000b1", "outcome": "deleted everything"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  asked,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:            "signing in is required",
			Method:          http.MethodPost,
			URL:             "/assistant/feedback",
			Body:            body(map[string]any{"rating": 1}),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAssistantConversations(t *testing.T) {
	t.Setenv("ASSISTANT_ENABLED", "true")
	useChatter(t, answeringChat{})
	adminToken, readOnlyToken, betaAdminToken := assistantTokens(t)
	body := func(fields map[string]any) *strings.Reader {
		if _, ok := fields["congregation"]; !ok {
			fields["congregation"] = "testcongalpha01"
		}
		raw, _ := json.Marshal(fields)
		return strings.NewReader(string(raw))
	}
	exec := func(t testing.TB, app *tests.TestApp, query string, params dbx.Params) {
		t.Helper()
		if _, err := app.DB().NewQuery(query).Bind(params).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	// The admin's conversation of two questions, and the read-only user's.
	seed := func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		exec(t, app, `INSERT INTO assistant_conversations (id, congregation, user, title, last_question, created)
			VALUES ('testconvadmin01', 'testcongalpha01', 'testuseralpha01', '', '2026-11-05 10:00:00.000Z', '2026-11-05 09:00:00.000Z'),
			       ('testconvreader1', 'testcongalpha01', 'testuseralpha03', '', '2026-11-05 11:00:00.000Z', '2026-11-05 11:00:00.000Z')`, nil)
		exec(t, app, `INSERT INTO assistant_questions (id, congregation, user, conversation, question, answer, answered, created, cards)
			VALUES ('testqconvadmin1', 'testcongalpha01', 'testuseralpha01', 'testconvadmin01', 'Who came out today?', 'Nobody yet.', 1, '2026-11-05 09:00:00.000Z',
			        '[{"proposal":{"id":"00000000000000c1","kind":"share_link","value":"Bro Tan"},"outcome":"done","person":"Bro Tan","record":"testassignalpha01"},'
			        || '{"proposal":{"id":"00000000000000c2","kind":"share_link","value":"Old"},"outcome":"done","record":"testassignexprd01"}]'),
			       ('testqconvadmin2', 'testcongalpha01', 'testuseralpha01', 'testconvadmin01', 'And yesterday?', 'Two people.', 1, '2026-11-05 10:00:00.000Z', '[]'),
			       ('testqconvreadr1', 'testcongalpha01', 'testuseralpha03', 'testconvreader1', 'Reader question', 'x', 1, '2026-11-05 11:00:00.000Z', '[]')`, nil)
		// The second was stopped on a map, after one lookup, and rated down.
		exec(t, app, `UPDATE assistant_questions SET status = 'stopped', rating = -1, reason = 'wrong_figures',
			steps = '[{"tool":"who_worked","rows":[{"name":"Bro Tan"}]}]', view = '{"territoryId":"testterralpha01","territoryCode":"T01"}'
			WHERE id = 'testqconvadmin2'`, nil)
	}
	count := func(t testing.TB, app *tests.TestApp, query string) int {
		t.Helper()
		var n struct {
			N int `db:"n"`
		}
		if err := app.DB().NewQuery(query).One(&n); err != nil {
			t.Fatal(err)
		}
		return n.N
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "a question continues the asker's conversation",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            body(map[string]any{"question": "And today?", "conversation": "testconvadmin01"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  seed,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"conversationId":"testconvadmin01"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_questions WHERE conversation = 'testconvadmin01'`); n != 3 {
					t.Errorf("questions in the conversation = %d, want 3", n)
				}
				c, _ := app.FindRecordById("assistant_conversations", "testconvadmin01")
				if !strings.HasPrefix(c.GetString("last_question"), time.Now().UTC().Format("2006-01-02")) {
					t.Errorf("last_question = %q, want today", c.GetString("last_question"))
				}
			},
		},
		{
			Name:            "a new conversation takes the panel's id",
			Method:          http.MethodPost,
			URL:             "/assistant/ask",
			Body:            body(map[string]any{"question": "Hello?", "conversation": "panelconv00001a"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  seed,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"conversationId":"panelconv00001a"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_questions WHERE conversation = 'panelconv00001a'`); n != 1 {
					t.Errorf("questions in the new conversation = %d, want 1", n)
				}
			},
		},
		{
			Name:               "someone else's conversation is not continued; a new one starts",
			Method:             http.MethodPost,
			URL:                "/assistant/ask",
			Body:               body(map[string]any{"question": "Hello?", "conversation": "testconvreader1"}),
			Headers:            jsonHeaders(adminToken),
			BeforeTestFunc:     seed,
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"conversationId":"`},
			NotExpectedContent: []string{`"conversationId":"testconvreader1"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_conversations WHERE user = 'testuseralpha01'`); n != 2 {
					t.Errorf("admin's conversations = %d, want the seeded one and a new one", n)
				}
			},
		},
		{
			Name:               "the list holds only the asker's conversations, with the first question",
			Method:             http.MethodPost,
			URL:                "/assistant/history",
			Body:               body(map[string]any{"action": "list"}),
			Headers:            jsonHeaders(adminToken),
			BeforeTestFunc:     seed,
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"id":"testconvadmin01"`, `"firstQuestion":"Who came out today?"`, `"questions":2`, `"lastQuestion":"2026-11-05T10:00:00Z"`, `"more":false`},
			NotExpectedContent: []string{"testconvreader1"},
		},
		{
			Name:           "opening a conversation returns its turns in order",
			Method:         http.MethodPost,
			URL:            "/assistant/history",
			Body:           body(map[string]any{"action": "open", "conversation": "testconvadmin01"}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: seed,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{`"question":"Who came out today?","answer":"Nobody yet."`, `"question":"And yesterday?"`,
				`"cards":[{"proposal":{"id":"00000000000000c1"`, `"outcome":"done","person":"Bro Tan"`, `"cards":[]`,
				// The live link can be shared again; the ended one is not offered.
				`"outcome":"done","person":"Bro Tan","record":"testassignalpha01"}`,

				// The lookups, the place open and the rating come back as they were.
				`"steps":[{"tool":"who_worked","rows":[{"name":"Bro Tan"}]}],"status":"stopped","view":{"territoryId":"testterralpha01","territoryCode":"T01"},"rating":-1,"reason":"wrong_figures"`,
				`"steps":[],"status":"","view":null,"rating":0,"reason":""`},
			NotExpectedContent: []string{"testassignexprd01"},
		},
		{
			Name:            "someone else's conversation cannot be opened",
			Method:          http.MethodPost,
			URL:             "/assistant/history",
			Body:            body(map[string]any{"action": "open", "conversation": "testconvreader1"}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  seed,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:           "a conversation can be renamed",
			Method:         http.MethodPost,
			URL:            "/assistant/history",
			Body:           body(map[string]any{"action": "rename", "conversation": "testconvadmin01", "title": "  Saturday plan  "}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: seed,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				c, _ := app.FindRecordById("assistant_conversations", "testconvadmin01")
				if c.GetString("title") != "Saturday plan" {
					t.Errorf("title = %q", c.GetString("title"))
				}
				if !strings.HasPrefix(c.GetString("last_question"), "2026-11-05 10:00:00") {
					t.Errorf("renaming must not count as activity: last_question = %q", c.GetString("last_question"))
				}
			},
		},
		{
			Name:            "a title over the limit is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/history",
			Body:            body(map[string]any{"action": "rename", "conversation": "testconvadmin01", "title": strings.Repeat("a", 121)}),
			Headers:         jsonHeaders(adminToken),
			BeforeTestFunc:  seed,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:           "deleting a conversation deletes its questions",
			Method:         http.MethodPost,
			URL:            "/assistant/history",
			Body:           body(map[string]any{"action": "delete", "conversation": "testconvadmin01"}),
			Headers:        jsonHeaders(adminToken),
			BeforeTestFunc: seed,
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_conversations WHERE id = 'testconvadmin01'`); n != 0 {
					t.Error("the conversation is still there")
				}
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_questions WHERE conversation = 'testconvadmin01'`); n != 0 {
					t.Errorf("%d of its questions are still there", n)
				}
				if n := count(t, app, `SELECT COUNT(*) AS n FROM assistant_questions WHERE id = 'testqconvreadr1'`); n != 1 {
					t.Error("another user's question went too")
				}
			},
		},
		{
			Name:            "history needs a role in the congregation",
			Method:          http.MethodPost,
			URL:             "/assistant/history",
			Body:            body(map[string]any{"action": "list"}),
			Headers:         jsonHeaders(betaAdminToken),
			BeforeTestFunc:  seed,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:            "an unknown action is refused",
			Method:          http.MethodPost,
			URL:             "/assistant/history",
			Body:            body(map[string]any{"action": "export"}),
			Headers:         jsonHeaders(readOnlyToken),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
