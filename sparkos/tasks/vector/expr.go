package vector

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrParse = errors.New("parse error")
	ErrEval  = errors.New("eval error")
)

type evalMode uint8

const (
	modeFloat evalMode = iota
	modeExact
)

type env struct {
	mode evalMode
	prec int

	vars  map[string]Value
	funcs map[string]userFunc
}

type userFunc struct {
	param string
	body  node
}

func newEnv() *env {
	return &env{
		mode: modeFloat,
		prec: 12,
		vars: map[string]Value{
			"pi": NumberValue(Float(math.Pi)),
			"e":  NumberValue(Float(math.E)),
		},
		funcs: make(map[string]userFunc),
	}
}

type node interface {
	Eval(e *env) (Value, error)
	Simplify() node
	Deriv(varName string) node
}

type nodeNumber struct{ v Number }

func (n nodeNumber) Eval(_ *env) (Value, error) { return NumberValue(n.v), nil }

func (n nodeNumber) Simplify() node { return n }

func (n nodeNumber) Deriv(_ string) node { return nodeNumber{v: RatNumber(RatInt(0))} }

type nodeIdent struct{ name string }

func (n nodeIdent) Eval(e *env) (Value, error) {
	v, ok := e.vars[n.name]
	if !ok {
		return Value{}, fmt.Errorf("%w: unknown variable %q", ErrEval, n.name)
	}
	return v, nil
}

func (n nodeIdent) Simplify() node { return n }

func (n nodeIdent) Deriv(varName string) node {
	if n.name == varName {
		return nodeNumber{v: RatNumber(RatInt(1))}
	}
	return nodeNumber{v: RatNumber(RatInt(0))}
}

type nodeUnary struct {
	op byte
	x  node
}

func (n nodeUnary) Eval(e *env) (Value, error) {
	v, err := n.x.Eval(e)
	if err != nil {
		return Value{}, err
	}
	if v.kind == valueExpr {
		return ExprValue(nodeUnary{op: n.op, x: v.expr}.Simplify()), nil
	}

	switch n.op {
	case '+':
		return v, nil
	case '-':
		return NumberValue(negNumber(v.num)), nil
	default:
		return Value{}, fmt.Errorf("%w: unary %q", ErrEval, n.op)
	}
}

func (n nodeUnary) Simplify() node {
	x := n.x.Simplify()
	if num, ok := x.(nodeNumber); ok {
		switch n.op {
		case '+':
			return num
		case '-':
			return nodeNumber{v: negNumber(num.v)}
		}
	}
	if u, ok := x.(nodeUnary); ok && n.op == '+' {
		return u
	}
	return nodeUnary{op: n.op, x: x}
}

func (n nodeUnary) Deriv(varName string) node {
	switch n.op {
	case '+':
		return n.x.Deriv(varName)
	case '-':
		return nodeUnary{op: '-', x: n.x.Deriv(varName)}.Simplify()
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

type nodeBinary struct {
	op    byte
	left  node
	right node
}

func (n nodeBinary) Eval(e *env) (Value, error) {
	a, err := n.left.Eval(e)
	if err != nil {
		return Value{}, err
	}
	b, err := n.right.Eval(e)
	if err != nil {
		return Value{}, err
	}

	if a.kind == valueExpr || b.kind == valueExpr {
		return ExprValue(nodeBinary{op: n.op, left: a.ToNode(), right: b.ToNode()}.Simplify()), nil
	}

	out, err := evalBinaryNumber(e, n.op, a.num, b.num)
	if err != nil {
		return Value{}, err
	}
	return NumberValue(out), nil
}

func evalBinaryNumber(e *env, op byte, a, b Number) (Number, error) {
	switch op {
	case '+':
		return addNumber(e, a, b)
	case '-':
		return subNumber(e, a, b)
	case '*':
		return mulNumber(e, a, b)
	case '/':
		return divNumber(e, a, b)
	case '^':
		return powNumber(e, a, b)
	default:
		return Number{}, fmt.Errorf("%w: binary %q", ErrEval, op)
	}
}

func (n nodeBinary) Simplify() node {
	left := n.left.Simplify()
	right := n.right.Simplify()

	ln, lok := left.(nodeNumber)
	rn, rok := right.(nodeNumber)
	if lok && rok {
		out, err := evalBinaryNumber(newEnv(), n.op, ln.v, rn.v)
		if err == nil {
			return nodeNumber{v: out}
		}
	}

	switch n.op {
	case '+':
		if isZeroNode(left) {
			return right
		}
		if isZeroNode(right) {
			return left
		}
	case '-':
		if isZeroNode(right) {
			return left
		}
	case '*':
		if isZeroNode(left) || isZeroNode(right) {
			return nodeNumber{v: RatNumber(RatInt(0))}
		}
		if isOneNode(left) {
			return right
		}
		if isOneNode(right) {
			return left
		}
	case '/':
		if isZeroNode(left) {
			return nodeNumber{v: RatNumber(RatInt(0))}
		}
		if isOneNode(right) {
			return left
		}
	case '^':
		if isOneNode(right) {
			return left
		}
		if isZeroNode(right) {
			return nodeNumber{v: RatNumber(RatInt(1))}
		}
	}

	return nodeBinary{op: n.op, left: left, right: right}
}

func isZeroNode(n node) bool {
	num, ok := n.(nodeNumber)
	if !ok {
		return false
	}
	if num.v.kind == numberRat {
		return num.v.r.num == 0
	}
	return num.v.f == 0
}

func isOneNode(n node) bool {
	num, ok := n.(nodeNumber)
	if !ok {
		return false
	}
	if num.v.kind == numberRat {
		return num.v.r.num == 1 && num.v.r.den == 1
	}
	return num.v.f == 1
}

func (n nodeBinary) Deriv(varName string) node {
	switch n.op {
	case '+':
		return nodeBinary{op: '+', left: n.left.Deriv(varName), right: n.right.Deriv(varName)}.Simplify()
	case '-':
		return nodeBinary{op: '-', left: n.left.Deriv(varName), right: n.right.Deriv(varName)}.Simplify()
	case '*':
		// (uv)' = u'v + uv'
		return nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '*',
				left:  n.left.Deriv(varName),
				right: n.right,
			},
			right: nodeBinary{
				op:    '*',
				left:  n.left,
				right: n.right.Deriv(varName),
			},
		}.Simplify()
	case '/':
		// (u/v)' = (u'v - uv') / v^2
		num := nodeBinary{
			op: '-',
			left: nodeBinary{
				op:    '*',
				left:  n.left.Deriv(varName),
				right: n.right,
			},
			right: nodeBinary{
				op:    '*',
				left:  n.left,
				right: n.right.Deriv(varName),
			},
		}
		den := nodeBinary{op: '^', left: n.right, right: nodeNumber{v: RatNumber(RatInt(2))}}
		return nodeBinary{op: '/', left: num, right: den}.Simplify()
	case '^':
		// Handle f(x)^c where c is constant number.
		if cn, ok := n.right.(nodeNumber); ok && cn.v.kind == numberRat && cn.v.r.den == 1 {
			c := cn.v.r.num
			if c == 0 {
				return nodeNumber{v: RatNumber(RatInt(0))}
			}
			// (u^c)' = c*u^(c-1)*u'
			return nodeBinary{
				op: '*',
				left: nodeBinary{
					op:    '*',
					left:  nodeNumber{v: RatNumber(RatInt(c))},
					right: nodeBinary{op: '^', left: n.left, right: nodeNumber{v: RatNumber(RatInt(c - 1))}},
				},
				right: n.left.Deriv(varName),
			}.Simplify()
		}
		// Fallback to numeric zero.
		return nodeNumber{v: RatNumber(RatInt(0))}
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

type nodeCall struct {
	name string
	args []node
}

func (n nodeCall) Eval(e *env) (Value, error) {
	if n.name == "simp" && len(n.args) == 1 {
		return ExprValue(n.args[0].Simplify()), nil
	}
	if n.name == "diff" && len(n.args) == 2 {
		varName, ok := callArgIdent(n.args[1])
		if !ok {
			return Value{}, fmt.Errorf("%w: diff expects second arg as identifier", ErrEval)
		}
		return ExprValue(n.args[0].Deriv(varName).Simplify()), nil
	}

	args := make([]Value, 0, len(n.args))
	for _, a := range n.args {
		v, err := a.Eval(e)
		if err != nil {
			return Value{}, err
		}
		args = append(args, v)
	}

	if fn, ok := e.funcs[n.name]; ok {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("%w: %s expects 1 argument", ErrEval, n.name)
		}
		prev, hadPrev := e.vars[fn.param]
		e.vars[fn.param] = args[0]
		out, err := fn.body.Eval(e)
		if hadPrev {
			e.vars[fn.param] = prev
		} else {
			delete(e.vars, fn.param)
		}
		return out, err
	}

	out, ok, err := builtinCall(e, n.name, args)
	if !ok {
		return Value{}, fmt.Errorf("%w: unknown function %q", ErrEval, n.name)
	}
	if err != nil {
		return Value{}, err
	}
	return NumberValue(out), nil
}

func (n nodeCall) Simplify() node {
	args := make([]node, 0, len(n.args))
	for _, a := range n.args {
		args = append(args, a.Simplify())
	}
	return nodeCall{name: n.name, args: args}
}

func (n nodeCall) Deriv(varName string) node {
	// Very small set: sin/cos/exp/ln with 1 arg.
	if len(n.args) != 1 {
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
	u := n.args[0]
	du := u.Deriv(varName)
	switch n.name {
	case "sin":
		return nodeBinary{op: '*', left: nodeCall{name: "cos", args: []node{u}}, right: du}.Simplify()
	case "cos":
		return nodeBinary{op: '*', left: nodeUnary{op: '-', x: nodeCall{name: "sin", args: []node{u}}}, right: du}.Simplify()
	case "exp":
		return nodeBinary{op: '*', left: nodeCall{name: "exp", args: []node{u}}, right: du}.Simplify()
	case "ln":
		return nodeBinary{op: '*', left: nodeBinary{op: '/', left: nodeNumber{v: RatNumber(RatInt(1))}, right: u}, right: du}.Simplify()
	default:
		return nodeNumber{v: RatNumber(RatInt(0))}
	}
}

func callArgIdent(n node) (string, bool) {
	id, ok := n.(nodeIdent)
	if ok {
		return id.name, true
	}
	return "", false
}

func builtinCall(e *env, name string, args []Value) (Number, bool, error) {
	// If any arg is symbolic, keep it symbolic by returning "unknown" and letting caller wrap.
	for _, a := range args {
		if a.kind == valueExpr {
			return Number{}, false, nil
		}
	}
	nums := make([]Number, 0, len(args))
	for _, a := range args {
		nums = append(nums, a.num)
	}

	switch name {
	case "sin":
		return unaryFloat(e, nums, math.Sin)
	case "cos":
		return unaryFloat(e, nums, math.Cos)
	case "tan":
		return unaryFloat(e, nums, math.Tan)
	case "asin":
		return unaryFloat(e, nums, math.Asin)
	case "acos":
		return unaryFloat(e, nums, math.Acos)
	case "atan":
		return unaryFloat(e, nums, math.Atan)
	case "sqrt":
		return unaryFloat(e, nums, math.Sqrt)
	case "abs":
		if len(nums) != 1 {
			return Number{}, true, fmt.Errorf("%w: abs expects 1 argument", ErrEval)
		}
		if nums[0].kind == numberRat {
			r := nums[0].r
			if r.num < 0 {
				r.num = -r.num
			}
			return RatNumber(r), true, nil
		}
		return Float(math.Abs(nums[0].f)), true, nil
	case "exp":
		return unaryFloat(e, nums, math.Exp)
	case "ln":
		return unaryFloat(e, nums, math.Log)
	case "min":
		if len(nums) == 0 {
			return Number{}, true, fmt.Errorf("%w: min expects >= 1 argument", ErrEval)
		}
		m := nums[0].Float64()
		for _, v := range nums[1:] {
			f := v.Float64()
			if f < m {
				m = f
			}
		}
		return Float(m), true, nil
	case "max":
		if len(nums) == 0 {
			return Number{}, true, fmt.Errorf("%w: max expects >= 1 argument", ErrEval)
		}
		m := nums[0].Float64()
		for _, v := range nums[1:] {
			f := v.Float64()
			if f > m {
				m = f
			}
		}
		return Float(m), true, nil
	default:
		return Number{}, false, nil
	}
}

func unaryFloat(e *env, args []Number, fn func(float64) float64) (Number, bool, error) {
	if len(args) != 1 {
		return Number{}, true, fmt.Errorf("%w: expects 1 argument", ErrEval)
	}
	return Float(fn(args[0].Float64())), true, nil
}

func negNumber(n Number) Number {
	if n.kind == numberRat {
		r := n.r
		r.num = -r.num
		return RatNumber(r)
	}
	return Float(-n.f)
}

func addNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Add(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() + b.Float64()), nil
}

func subNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Sub(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() - b.Float64()), nil
}

func mulNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Mul(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() * b.Float64()), nil
}

func divNumber(e *env, a, b Number) (Number, error) {
	if b.Float64() == 0 {
		return Number{}, fmt.Errorf("%w: division by zero", ErrEval)
	}
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat {
		r, err := a.r.Div(b.r)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(a.Float64() / b.Float64()), nil
}

func powNumber(e *env, a, b Number) (Number, error) {
	if e.mode == modeExact && a.kind == numberRat && b.kind == numberRat && b.r.den == 1 {
		r, err := a.r.PowInt(b.r.num)
		if err == nil {
			return RatNumber(r), nil
		}
	}
	return Float(math.Pow(a.Float64(), b.Float64())), nil
}
