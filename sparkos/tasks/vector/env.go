package vector

import (
	"errors"
	"math"
)

var (
	ErrParse = errors.New("parse error")
	ErrEval  = errors.New("eval error")
	// ErrUnknownVar is returned when evaluating an expression with an undefined variable.
	ErrUnknownVar = errors.New("unknown variable")
)

// evalMode configures how scalar arithmetic is performed.
type evalMode uint8

const (
	modeFloat evalMode = iota
	modeExact
)

// env is the evaluation environment for expressions.
type env struct {
	mode evalMode
	prec int

	vars  map[string]Value
	funcs map[string]userFunc
}

// userFunc is a user-defined function f(x)=...
type userFunc struct {
	param string
	body  node
}

func newEnv() *env {
	return &env{
		mode: modeFloat,
		prec: 12,
		vars: map[string]Value{
			"pi":    NumberValue(Float(math.Pi)),
			"tau":   NumberValue(Float(2 * math.Pi)),
			"e":     NumberValue(Float(math.E)),
			"phi":   NumberValue(Float((1 + math.Sqrt(5)) / 2)),
			"sqrt2": NumberValue(Float(math.Sqrt2)),
			"sqrt3": NumberValue(Float(math.Sqrt(3))),
			"sqrt5": NumberValue(Float(math.Sqrt(5))),
			"ln2":   NumberValue(Float(math.Ln2)),
			"ln10":  NumberValue(Float(math.Ln10)),
			"i":     ComplexValue(0, 1),
		},
		funcs: make(map[string]userFunc),
	}
}

func (e *env) clone() *env {
	if e == nil {
		return newEnv()
	}

	vars := make(map[string]Value, len(e.vars)+4)
	for k, v := range e.vars {
		vars[k] = v
	}

	return &env{
		mode:  e.mode,
		prec:  e.prec,
		vars:  vars,
		funcs: e.funcs,
	}
}
