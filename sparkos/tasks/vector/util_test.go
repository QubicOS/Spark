package vector

import "testing"

func TestControl_SizeNumericExactTime(t *testing.T) {
	e := newEnv()

	ex := ExprValue(nodeBinary{
		op:    '+',
		left:  nodeNumber{v: RatNumber(RatInt(1))},
		right: nodeBinary{op: '/', left: nodeNumber{v: RatNumber(RatInt(1))}, right: nodeNumber{v: RatNumber(RatInt(2))}},
	})

	sz, ok, err := builtinCallControl(e, "size", []Value{ex})
	if !ok || err != nil {
		t.Fatalf("size ok=%v err=%v", ok, err)
	}
	if !sz.IsNumber() || sz.num.Float64() <= 0 {
		t.Fatalf("size=%v", sz.num.Float64())
	}

	ev, ok, err := builtinCallControl(e, "exact", []Value{ex})
	if !ok || err != nil {
		t.Fatalf("exact ok=%v err=%v", ok, err)
	}
	if !ev.IsNumber() || ev.num.String(12) != "3/2" {
		t.Fatalf("exact=%v", ev.num.String(12))
	}

	nv, ok, err := builtinCallControl(e, "numeric", []Value{ex})
	if !ok || err != nil {
		t.Fatalf("numeric ok=%v err=%v", ok, err)
	}
	if !nv.IsNumber() || nv.num.Float64() != 1.5 {
		t.Fatalf("numeric=%v", nv.num.Float64())
	}

	tv, ok, err := builtinCallControl(e, "time", []Value{ex})
	if !ok || err != nil {
		t.Fatalf("time ok=%v err=%v", ok, err)
	}
	if !tv.IsNumber() || tv.num.Float64() != 1.5 {
		t.Fatalf("time=%v", tv.num.Float64())
	}
	dt, ok := e.vars["_time_ms"]
	if !ok || !dt.IsNumber() {
		t.Fatalf("_time_ms=%v", dt.kind)
	}
}
