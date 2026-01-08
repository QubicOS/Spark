package vector

import (
	"fmt"
)

func NodeString(n node) string {
	return nodeStringPrec(n, 0)
}

func nodeStringPrec(n node, parentPrec int) string {
	switch nn := n.(type) {
	case nodeNumber:
		return nn.v.String(12)
	case nodeIdent:
		return nn.name
	case nodeUnary:
		prec := 4
		s := string(nn.op) + nodeStringPrec(nn.x, prec)
		if prec < parentPrec {
			return "(" + s + ")"
		}
		return s
	case nodeCompare:
		prec := 0
		left := nodeStringPrec(nn.left, prec)
		right := nodeStringPrec(nn.right, prec)
		s := fmt.Sprintf("%s %s %s", left, tokenText(nn.op), right)
		if prec < parentPrec {
			return "(" + s + ")"
		}
		return s
	case nodeBinary:
		prec := binPrec(nn.op)
		left := nodeStringPrec(nn.left, prec)
		rightPrec := prec
		if nn.op == '^' {
			rightPrec = prec - 1
		}
		right := nodeStringPrec(nn.right, rightPrec)
		s := fmt.Sprintf("%s %c %s", left, nn.op, right)
		if prec < parentPrec {
			return "(" + s + ")"
		}
		return s
	case nodeCall:
		out := nn.name + "("
		for i, a := range nn.args {
			if i > 0 {
				out += ", "
			}
			out += nodeStringPrec(a, 0)
		}
		out += ")"
		return out
	default:
		return "<?>"
	}
}

func binPrec(op byte) int {
	switch op {
	case '+', '-':
		return 1
	case '*', '/':
		return 2
	case '^':
		return 3
	default:
		return 0
	}
}

func nodeHasIdent(n node, name string) bool {
	switch nn := n.(type) {
	case nodeIdent:
		return nn.name == name
	case nodeNumber:
		return false
	case nodeUnary:
		return nodeHasIdent(nn.x, name)
	case nodeCompare:
		return nodeHasIdent(nn.left, name) || nodeHasIdent(nn.right, name)
	case nodeBinary:
		return nodeHasIdent(nn.left, name) || nodeHasIdent(nn.right, name)
	case nodeCall:
		for _, a := range nn.args {
			if nodeHasIdent(a, name) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
