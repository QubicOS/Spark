package vector

import (
	"fmt"
	"math"
)

// builtinCallCAS implements symbolic-ish helpers that operate on unevaluated AST nodes.
//
// These functions are handled in nodeCall.Eval before argument evaluation so callers can use
// free variables (e.g. `series(sin(x), x, 0, 8)`).
func builtinCallCAS(e *env, name string, args []node) (Value, bool, error) {
	switch name {
	case "expand":
		if len(args) != 1 {
			return Value{}, true, fmt.Errorf("%w: expand(expr)", ErrEval)
		}
		return ExprValue(expandNode(args[0]).Simplify()), true, nil

	case "degree":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: degree(expr, x)", ErrEval)
		}
		varName, ok := callArgIdent(args[1])
		if !ok {
			return Value{}, true, fmt.Errorf("%w: degree expects second arg as identifier", ErrEval)
		}
		p, err := polyFromExpr(e, args[0], varName)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: degree: %w", ErrEval, err)
		}
		return NumberValue(RatNumber(RatInt(int64(p.degree())))), true, nil

	case "coeff":
		if len(args) != 3 {
			return Value{}, true, fmt.Errorf("%w: coeff(expr, x, n)", ErrEval)
		}
		varName, ok := callArgIdent(args[1])
		if !ok {
			return Value{}, true, fmt.Errorf("%w: coeff expects second arg as identifier", ErrEval)
		}
		nv, err := args[2].Eval(e)
		if err != nil {
			return Value{}, true, err
		}
		n, err := requireInt(nv)
		if err != nil || n < 0 {
			return Value{}, true, fmt.Errorf("%w: coeff expects non-negative integer n", ErrEval)
		}
		p, err := polyFromExpr(e, args[0], varName)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: coeff: %w", ErrEval, err)
		}
		if n >= len(p.coeffs) {
			return NumberValue(Float(0)), true, nil
		}
		return NumberValue(Float(p.coeffs[n])), true, nil

	case "collect":
		if len(args) != 2 {
			return Value{}, true, fmt.Errorf("%w: collect(expr, x)", ErrEval)
		}
		varName, ok := callArgIdent(args[1])
		if !ok {
			return Value{}, true, fmt.Errorf("%w: collect expects second arg as identifier", ErrEval)
		}
		p, err := polyFromExpr(e, args[0], varName)
		if err != nil {
			return Value{}, true, fmt.Errorf("%w: collect: %w", ErrEval, err)
		}
		return ExprValue(polyToExprHorner(p, varName).Simplify()), true, nil

	case "series":
		// series(expr, x, a, n) returns Taylor series around a.
		if len(args) != 4 {
			return Value{}, true, fmt.Errorf("%w: series(expr, x, a, n)", ErrEval)
		}
		varName, ok := callArgIdent(args[1])
		if !ok {
			return Value{}, true, fmt.Errorf("%w: series expects second arg as identifier", ErrEval)
		}
		aV, err := args[2].Eval(e)
		if err != nil {
			return Value{}, true, err
		}
		if !aV.IsNumber() {
			return Value{}, true, fmt.Errorf("%w: series expects numeric a", ErrEval)
		}
		nV, err := args[3].Eval(e)
		if err != nil {
			return Value{}, true, err
		}
		n, err := requireInt(nV)
		if err != nil || n < 0 || n > 64 {
			return Value{}, true, fmt.Errorf("%w: series expects n in 0..64", ErrEval)
		}

		out, err := taylorSeries(e, args[0], varName, aV.num.Float64(), n)
		if err != nil {
			return Value{}, true, err
		}
		return ExprValue(out.Simplify()), true, nil
	}

	return Value{}, false, nil
}

func expandNode(n node) node {
	switch nn := n.(type) {
	case nodeNumber, nodeIdent:
		return nn
	case nodeUnary:
		return nodeUnary{op: nn.op, x: expandNode(nn.x)}.Simplify()
	case nodeCall:
		args := make([]node, 0, len(nn.args))
		for _, a := range nn.args {
			args = append(args, expandNode(a))
		}
		return nodeCall{name: nn.name, args: args}
	case nodeBinary:
		left := expandNode(nn.left)
		right := expandNode(nn.right)
		switch nn.op {
		case '*':
			// Distribute over addition/subtraction.
			if la, ok := left.(nodeBinary); ok && (la.op == '+' || la.op == '-') {
				return expandNode(nodeBinary{op: la.op, left: nodeBinary{op: '*', left: la.left, right: right}, right: nodeBinary{op: '*', left: la.right, right: right}})
			}
			if rb, ok := right.(nodeBinary); ok && (rb.op == '+' || rb.op == '-') {
				return expandNode(nodeBinary{op: rb.op, left: nodeBinary{op: '*', left: left, right: rb.left}, right: nodeBinary{op: '*', left: left, right: rb.right}})
			}
			return nodeBinary{op: '*', left: left, right: right}.Simplify()
		case '^':
			// Expand (a+b)^n for small integer n.
			if rn, ok := right.(nodeNumber); ok {
				exp := rn.v.Float64()
				if exp == math.Trunc(exp) {
					pow := int(exp)
					if pow >= 0 && pow <= 12 {
						if lb, ok := left.(nodeBinary); ok && (lb.op == '+' || lb.op == '-') {
							out := node(nodeNumber{v: RatNumber(RatInt(1))})
							for i := 0; i < pow; i++ {
								out = expandNode(nodeBinary{op: '*', left: out, right: left}.Simplify())
							}
							return out.Simplify()
						}
					}
				}
			}
			return nodeBinary{op: '^', left: left, right: right}
		default:
			return nodeBinary{op: nn.op, left: left, right: right}.Simplify()
		}
	default:
		return n
	}
}

func taylorSeries(e *env, ex node, varName string, a float64, n int) (node, error) {
	prev, hadPrev := e.vars[varName]
	defer func() {
		if hadPrev {
			e.vars[varName] = prev
		} else {
			delete(e.vars, varName)
		}
	}()

	// Compute sum_{k=0..n} f^(k)(a) / k! * (x-a)^k.
	x := nodeIdent{name: varName}
	dx := nodeBinary{op: '-', left: x, right: nodeNumber{v: Float(a)}}.Simplify()
	fk := ex
	out := node(nodeNumber{v: RatNumber(RatInt(0))})
	fact := 1.0
	for k := 0; k <= n; k++ {
		e.vars[varName] = NumberValue(Float(a))
		v, err := fk.Eval(e)
		if err != nil {
			return nil, err
		}
		if !v.IsNumber() {
			return nil, fmt.Errorf("%w: series expects numeric expression", ErrEval)
		}
		ck := v.num.Float64() / fact
		term := node(nodeNumber{v: Float(ck)})
		if k > 0 {
			pow := dx
			for i := 1; i < k; i++ {
				pow = nodeBinary{op: '*', left: pow, right: dx}.Simplify()
			}
			term = nodeBinary{op: '*', left: term, right: pow}.Simplify()
		}
		out = nodeBinary{op: '+', left: out, right: term}.Simplify()

		fk = fk.Deriv(varName).Simplify()
		fact *= float64(k + 1)
		if fact == 0 || math.IsNaN(fact) || math.IsInf(fact, 0) {
			break
		}
	}
	return out.Simplify(), nil
}
