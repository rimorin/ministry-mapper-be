//go:build testdata

package assistant

import (
	"encoding/json"
	"testing"
)

func TestEvaluate(t *testing.T) {
	for expr, want := range map[string]float64{
		"137 / 412 * 100": 137.0 / 412 * 100,
		"(5 - 2) * 7":     21,
		"-3 + 10":         7,
		"9 / 2":           4.5,
	} {
		if got, err := evaluate(expr); err != nil || got != want {
			t.Errorf("evaluate(%q) = %v, %v; want %v", expr, got, err, want)
		}
	}
	for _, bad := range []string{"1 / 0", "os.Exit(1)", "x + 1", "2 ^ 3", "\"1\" + 1", "1 +"} {
		if _, err := evaluate(bad); err == nil {
			t.Errorf("evaluate(%q) must fail", bad)
		}
	}
}

func TestCalculate_UsesOnlyNumbersFromTheData(t *testing.T) {
	s := Scope{seen: `{"rows":[{"visits":412,"nobody_home":137}]}`}

	r, err := calculate.Run(s, json.RawMessage(`{"expression": "137 / 412 * 100"}`))
	if err != nil {
		t.Fatal(err)
	}
	if row := r.Rows[0]; row["result"] != 33.25 || row["rounded"] != int64(33) {
		t.Fatalf("row = %v", row)
	}

	_, err = calculate.Run(s, json.RawMessage(`{"expression": "999 / 412 * 100"}`))
	wantArgError(t, err, "999")
}
