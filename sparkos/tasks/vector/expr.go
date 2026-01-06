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

type env struct {
	vars  map[string]float64
	funcs map[string]userFunc
}

type userFunc struct {
	param string
	body  node
}

func newEnv() *env {
	return &env{
		vars: map[string]float64{
			"pi": math.Pi,
			"e":  math.E,
		},
		funcs: make(map[string]userFunc),
	}
}

type node interface {
	Eval(e *env) (float64, error)
}

type nodeNumber struct{ v float64 }

func (n nodeNumber) Eval(_ *env) (float64, error) { return n.v, nil }

type nodeIdent struct{ name string }

func (n nodeIdent) Eval(e *env) (float64, error) {
	v, ok := e.vars[n.name]
	if !ok {
		return 0, fmt.Errorf("%w: unknown variable %q", ErrEval, n.name)
	}
	return v, nil
}

type nodeUnary struct {
	op byte
	x  node
}

func (n nodeUnary) Eval(e *env) (float64, error) {
	v, err := n.x.Eval(e)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case '+':
		return v, nil
	case '-':
		return -v, nil
	default:
		return 0, fmt.Errorf("%w: unary %q", ErrEval, n.op)
	}
}

type nodeBinary struct {
	op    byte
	left  node
	right node
}

func (n nodeBinary) Eval(e *env) (float64, error) {
	a, err := n.left.Eval(e)
	if err != nil {
		return 0, err
	}
	b, err := n.right.Eval(e)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case '+':
		return a + b, nil
	case '-':
		return a - b, nil
	case '*':
		return a * b, nil
	case '/':
		if b == 0 {
			return 0, fmt.Errorf("%w: division by zero", ErrEval)
		}
		return a / b, nil
	case '^':
		return math.Pow(a, b), nil
	default:
		return 0, fmt.Errorf("%w: binary %q", ErrEval, n.op)
	}
}

type nodeCall struct {
	name string
	args []node
}

func (n nodeCall) Eval(e *env) (float64, error) {
	args := make([]float64, 0, len(n.args))
	for _, a := range n.args {
		v, err := a.Eval(e)
		if err != nil {
			return 0, err
		}
		args = append(args, v)
	}

	if fn, ok := e.funcs[n.name]; ok {
		if len(args) != 1 {
			return 0, fmt.Errorf("%w: %s expects 1 argument", ErrEval, n.name)
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

	out, err, ok := builtinCall(n.name, args)
	if !ok {
		return 0, fmt.Errorf("%w: unknown function %q", ErrEval, n.name)
	}
	if err != nil {
		return 0, err
	}
	return out, nil
}

func builtinCall(name string, args []float64) (float64, error, bool) {
	switch name {
	case "sin":
		return unaryBuiltin(args, math.Sin)
	case "cos":
		return unaryBuiltin(args, math.Cos)
	case "tan":
		return unaryBuiltin(args, math.Tan)
	case "asin":
		return unaryBuiltin(args, math.Asin)
	case "acos":
		return unaryBuiltin(args, math.Acos)
	case "atan":
		return unaryBuiltin(args, math.Atan)
	case "sqrt":
		return unaryBuiltin(args, math.Sqrt)
	case "abs":
		return unaryBuiltin(args, math.Abs)
	case "exp":
		return unaryBuiltin(args, math.Exp)
	case "ln":
		return unaryBuiltin(args, math.Log)
	case "log":
		if len(args) == 1 {
			return math.Log10(args[0]), nil, true
		}
		if len(args) == 2 {
			if args[1] <= 0 || args[1] == 1 {
				return 0, fmt.Errorf("%w: log base must be > 0 and != 1", ErrEval), true
			}
			return math.Log(args[0]) / math.Log(args[1]), nil, true
		}
		return 0, fmt.Errorf("%w: log expects 1 or 2 arguments", ErrEval), true
	case "floor":
		return unaryBuiltin(args, math.Floor)
	case "ceil":
		return unaryBuiltin(args, math.Ceil)
	case "round":
		if len(args) != 1 {
			return 0, fmt.Errorf("%w: round expects 1 argument", ErrEval), true
		}
		return math.Round(args[0]), nil, true
	case "min":
		if len(args) == 0 {
			return 0, fmt.Errorf("%w: min expects >= 1 argument", ErrEval), true
		}
		m := args[0]
		for _, v := range args[1:] {
			if v < m {
				m = v
			}
		}
		return m, nil, true
	case "max":
		if len(args) == 0 {
			return 0, fmt.Errorf("%w: max expects >= 1 argument", ErrEval), true
		}
		m := args[0]
		for _, v := range args[1:] {
			if v > m {
				m = v
			}
		}
		return m, nil, true
	default:
		return 0, nil, false
	}
}

func unaryBuiltin(args []float64, fn func(float64) float64) (float64, error, bool) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%w: expects 1 argument", ErrEval), true
	}
	return fn(args[0]), nil, true
}
