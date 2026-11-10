package assistant

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
	"strings"

	"ministry-mapper/internal/llm"
)

// unitConstants are the numbers a calculation may use without the data holding
// them: percentages and units of time.
const unitConstants = "1 2 7 10 12 24 30 31 52 60 100 365"

const maxExpressionLength = 200

// calculate works out figures the data does not hold, such as a percentage or an
// estimate. Its result is a function result, so the grounding check accepts it;
// its operands must already be in what the model was shown, so it cannot turn an
// invented number into a grounded one.
var calculate = Tool{
	Name: "calculate",
	Description: "Work out a figure from numbers already in the function results: a difference, total, " +
		"average, percentage, rate or estimate. Use it for every number you derive, and quote its result. " +
		"Write the arithmetic with + - * / and brackets, such as (137 / 412) * 100.",
	Params: object(map[string]any{
		"expression": map[string]any{"type": "string",
			"description": "The arithmetic, using only numbers from the function results and units such as 100 or 7."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Expression string `json:"expression"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		expr := strings.TrimSpace(args.Expression)
		if expr == "" || len(expr) > maxExpressionLength {
			return Result{}, badArgs("expression must be 1 to %d characters", maxExpressionLength)
		}
		if err := llm.GroundedNumbers(s.seen+" "+unitConstants, expr); err != nil {
			return Result{}, badArgs("the expression %v; use only numbers from the function results", err)
		}
		value, err := evaluate(expr)
		if err != nil {
			return Result{}, badArgs("%v", err)
		}
		return Result{
			Columns: []string{"expression", "result", "rounded"},
			Rows: []map[string]any{{
				"expression": expr, "result": math.Round(value*100) / 100, "rounded": int64(math.Round(value)),
			}},
		}, nil
	},
}

// evaluate computes plain arithmetic: numbers, + - * /, and brackets. Anything
// else, such as a name or a function call, is refused.
func evaluate(expr string) (float64, error) {
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, errors.New("write the expression with numbers, + - * / and brackets only")
	}
	value, err := eval(node)
	if err == nil && (math.IsInf(value, 0) || math.IsNaN(value)) {
		err = errors.New("the expression has no finite result")
	}
	return value, err
}

func eval(node ast.Expr) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		if n.Kind != token.INT && n.Kind != token.FLOAT {
			return 0, fmt.Errorf("%s is not a number", n.Value)
		}
		return strconv.ParseFloat(n.Value, 64)
	case *ast.ParenExpr:
		return eval(n.X)
	case *ast.UnaryExpr:
		x, err := eval(n.X)
		switch {
		case err != nil:
			return 0, err
		case n.Op == token.SUB:
			return -x, nil
		case n.Op == token.ADD:
			return x, nil
		}
	case *ast.BinaryExpr:
		x, err := eval(n.X)
		if err != nil {
			return 0, err
		}
		y, err := eval(n.Y)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.ADD:
			return x + y, nil
		case token.SUB:
			return x - y, nil
		case token.MUL:
			return x * y, nil
		case token.QUO:
			if y == 0 {
				return 0, errors.New("the expression divides by zero")
			}
			return x / y, nil
		}
	}
	return 0, errors.New("write the expression with numbers, + - * / and brackets only")
}
