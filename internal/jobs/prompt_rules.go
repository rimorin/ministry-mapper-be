package jobs

// overviewMinItems is the smallest list worth summarising. Below it the list
// itself is shorter than any summary, so the model is not called.
const overviewMinItems = 3

// plainLanguageRules is appended to every prompt that writes for readers. The
// vocabulary lives here once so the report, notes, messages and instructions
// emails all sound the same.
const plainLanguageRules = `WRITING RULES:
- Write for a busy reader on a phone. Sentences of 15 words or fewer. One fact per sentence.
- Everyday words only. Say "homes", not households, addresses or units. Say "nobody home",
  not not_home. Say "do not call", not DNC.
- Use only what is in the data given. Never invent, infer or add anything. Any place or
  number you name must appear in the data exactly as written.
- No praise, no exhortation, no judgement of how things went.
- Do not name the people who wrote the data.
`
