package vector

import (
	"fmt"
)

func builtinCallPlane(_ *env, name string, args []Value) (Value, bool, error) {
	if name != "plane" {
		return Value{}, false, nil
	}

	// plane(n, d) -> z(x,y) where n is vec3 and n.x*x + n.y*y + n.z*z + d = 0.
	if len(args) == 2 && args[0].kind == valueArray && len(args[0].arr) == 3 && args[1].IsNumber() {
		nx := args[0].arr[0]
		ny := args[0].arr[1]
		nz := args[0].arr[2]
		d := args[1].num.Float64()
		if nz == 0 {
			return Value{}, true, fmt.Errorf("%w: plane: n.z must be non-zero (not a function z(x,y))", ErrEval)
		}
		// z = -(nx*x + ny*y + d)/nz
		num := nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '+',
				left:  nodeBinary{op: '*', left: nodeNumber{v: Float(nx)}, right: nodeIdent{name: "x"}},
				right: nodeBinary{op: '*', left: nodeNumber{v: Float(ny)}, right: nodeIdent{name: "y"}},
			},
			right: nodeNumber{v: Float(d)},
		}
		ex := nodeBinary{
			op: '/',
			left: nodeUnary{
				op: '-',
				x:  num,
			},
			right: nodeNumber{v: Float(nz)},
		}.Simplify()
		return ExprValue(ex), true, nil
	}

	// plane(p0, p1, p2) -> z(x,y) through 3 points (each vec3).
	if len(args) == 3 &&
		args[0].kind == valueArray && len(args[0].arr) == 3 &&
		args[1].kind == valueArray && len(args[1].arr) == 3 &&
		args[2].kind == valueArray && len(args[2].arr) == 3 {
		p0 := args[0].arr
		p1 := args[1].arr
		p2 := args[2].arr
		u := [3]float64{p1[0] - p0[0], p1[1] - p0[1], p1[2] - p0[2]}
		v := [3]float64{p2[0] - p0[0], p2[1] - p0[1], p2[2] - p0[2]}
		nx := u[1]*v[2] - u[2]*v[1]
		ny := u[2]*v[0] - u[0]*v[2]
		nz := u[0]*v[1] - u[1]*v[0]
		if nz == 0 {
			return Value{}, true, fmt.Errorf("%w: plane: points form vertical plane (not a function z(x,y))", ErrEval)
		}
		d := -(nx*p0[0] + ny*p0[1] + nz*p0[2])
		return builtinCallPlane(nil, "plane", []Value{ArrayValue([]float64{nx, ny, nz}), NumberValue(Float(d))})
	}

	return Value{}, true, fmt.Errorf("%w: plane(n, d) or plane(p0, p1, p2)", ErrEval)
}
