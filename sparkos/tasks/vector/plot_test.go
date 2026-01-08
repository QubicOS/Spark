package vector

import "testing"

func TestPlot_ImplicitContourVectorfield(t *testing.T) {
	e := newEnv()

	// Circle: x^2 + y^2 - 1 = 0.
	f := ExprValue(nodeBinary{
		op: '-',
		left: nodeBinary{
			op: '+',
			left: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "x"},
				right: nodeNumber{v: Float(2)},
			},
			right: nodeBinary{
				op:    '^',
				left:  nodeIdent{name: "y"},
				right: nodeNumber{v: Float(2)},
			},
		},
		right: nodeNumber{v: Float(1)},
	})

	v, ok, err := builtinCallPlot(e, "implicit", []Value{
		f,
		NumberValue(Float(-2)), NumberValue(Float(2)),
		NumberValue(Float(-2)), NumberValue(Float(2)),
		NumberValue(Float(32)),
	})
	if !ok || err != nil {
		t.Fatalf("implicit ok=%v err=%v", ok, err)
	}
	if v.kind != valueMatrix || v.cols != 2 || v.rows <= 0 {
		t.Fatalf("implicit kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}

	// Two contour levels for y: y=0 and y=1.
	y := ExprValue(nodeIdent{name: "y"})
	levels := ArrayValue([]float64{0, 1})
	v, ok, err = builtinCallPlot(e, "contour", []Value{
		y,
		levels,
		NumberValue(Float(-2)), NumberValue(Float(2)),
		NumberValue(Float(-2)), NumberValue(Float(2)),
		NumberValue(Float(16)),
	})
	if !ok || err != nil {
		t.Fatalf("contour ok=%v err=%v", ok, err)
	}
	if v.kind != valueMatrix || v.cols != 2 || v.rows <= 0 {
		t.Fatalf("contour kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}

	// Vectorfield: (y, -x).
	fx := ExprValue(nodeIdent{name: "y"})
	fy := ExprValue(nodeUnary{op: '-', x: nodeIdent{name: "x"}})
	v, ok, err = builtinCallPlot(e, "vectorfield", []Value{
		fx, fy,
		NumberValue(Float(-1)), NumberValue(Float(1)),
		NumberValue(Float(-1)), NumberValue(Float(1)),
		NumberValue(Float(8)),
	})
	if !ok || err != nil {
		t.Fatalf("vectorfield ok=%v err=%v", ok, err)
	}
	if v.kind != valueMatrix || v.cols != 2 || v.rows <= 0 {
		t.Fatalf("vectorfield kind=%v rows=%d cols=%d", v.kind, v.rows, v.cols)
	}
}
