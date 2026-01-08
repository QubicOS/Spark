package vector

import "sort"

// builtinKeywords is used for autocomplete and syntax highlighting.
func builtinKeywords() []string {
	set := make(map[string]struct{}, len(scalarBuiltins)+len(unaryArrayBuiltins)+len(arrayAggBuiltins)+3)
	set["range"] = struct{}{}
	set["simp"] = struct{}{}
	set["diff"] = struct{}{}
	set["expand"] = struct{}{}
	set["series"] = struct{}{}
	set["horner"] = struct{}{}
	set["degree"] = struct{}{}
	set["coeff"] = struct{}{}
	set["collect"] = struct{}{}
	set["factor"] = struct{}{}
	set["gcd"] = struct{}{}
	set["lcm"] = struct{}{}
	set["resultant"] = struct{}{}
	set["newton"] = struct{}{}
	set["bisection"] = struct{}{}
	set["secant"] = struct{}{}
	set["integrate_num"] = struct{}{}
	set["diff_num"] = struct{}{}
	set["interp"] = struct{}{}
	set["solve1"] = struct{}{}
	set["solve2"] = struct{}{}
	set["roots"] = struct{}{}
	set["region"] = struct{}{}
	set["solve"] = struct{}{}
	set["polyfit"] = struct{}{}
	set["polyval"] = struct{}{}
	set["convolve"] = struct{}{}
	set["cov"] = struct{}{}
	set["corr"] = struct{}{}
	set["hist"] = struct{}{}
	set["implicit"] = struct{}{}
	set["contour"] = struct{}{}
	set["vectorfield"] = struct{}{}
	set["polar"] = struct{}{}
	set["rect"] = struct{}{}
	set["plane"] = struct{}{}
	set["param"] = struct{}{}
	set["expr"] = struct{}{}
	set["eval"] = struct{}{}
	set["size"] = struct{}{}
	set["time"] = struct{}{}
	set["numeric"] = struct{}{}
	set["exact"] = struct{}{}
	set["if"] = struct{}{}
	set["where"] = struct{}{}
	set["and"] = struct{}{}
	set["or"] = struct{}{}
	set["not"] = struct{}{}
	set["vec2"] = struct{}{}
	set["vec3"] = struct{}{}
	set["vec4"] = struct{}{}
	set["x"] = struct{}{}
	set["y"] = struct{}{}
	set["z"] = struct{}{}
	set["w"] = struct{}{}
	set["dot"] = struct{}{}
	set["cross"] = struct{}{}
	set["mag"] = struct{}{}
	set["unit"] = struct{}{}
	set["normalize"] = struct{}{}
	set["dist"] = struct{}{}
	set["angle"] = struct{}{}
	set["proj"] = struct{}{}
	set["outer"] = struct{}{}
	set["lerp"] = struct{}{}
	set["zeros"] = struct{}{}
	set["ones"] = struct{}{}
	set["eye"] = struct{}{}
	set["reshape"] = struct{}{}
	set["T"] = struct{}{}
	set["transpose"] = struct{}{}
	set["det"] = struct{}{}
	set["inv"] = struct{}{}
	set["shape"] = struct{}{}
	set["flatten"] = struct{}{}
	set["get"] = struct{}{}
	set["set"] = struct{}{}
	set["row"] = struct{}{}
	set["col"] = struct{}{}
	set["diag"] = struct{}{}
	set["trace"] = struct{}{}
	set["norm"] = struct{}{}
	set["re"] = struct{}{}
	set["im"] = struct{}{}
	set["conj"] = struct{}{}
	set["arg"] = struct{}{}
	set["median"] = struct{}{}
	set["variance"] = struct{}{}
	set["std"] = struct{}{}
	set["qr"] = struct{}{}
	set["svd"] = struct{}{}
	for name := range scalarBuiltins {
		set[name] = struct{}{}
	}
	for name := range unaryArrayBuiltins {
		set[name] = struct{}{}
	}
	for name := range arrayAggBuiltins {
		set[name] = struct{}{}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isBuiltinKeyword(name string) bool {
	if name == "range" || name == "simp" || name == "diff" ||
		name == "expand" || name == "series" || name == "horner" || name == "degree" || name == "coeff" || name == "collect" ||
		name == "factor" || name == "gcd" || name == "lcm" || name == "resultant" ||
		name == "newton" || name == "bisection" || name == "secant" || name == "integrate_num" || name == "diff_num" || name == "interp" ||
		name == "solve1" || name == "solve2" || name == "roots" || name == "region" || name == "solve" ||
		name == "polyfit" || name == "polyval" || name == "convolve" || name == "cov" || name == "corr" || name == "hist" ||
		name == "implicit" || name == "contour" || name == "vectorfield" ||
		name == "polar" || name == "rect" ||
		name == "plane" || name == "param" || name == "expr" || name == "eval" ||
		name == "size" || name == "time" || name == "numeric" || name == "exact" {
		return true
	}
	switch name {
	case "vec2", "vec3", "vec4", "x", "y", "z", "w", "dot", "cross", "mag", "unit", "normalize", "dist", "angle", "proj", "outer", "lerp":
		return true
	case "zeros", "ones", "eye", "reshape", "T", "transpose", "det", "inv", "shape", "flatten",
		"get", "set", "row", "col", "diag", "trace", "norm", "qr", "svd",
		"re", "im", "conj", "arg", "median", "variance", "std",
		"if", "where", "and", "or", "not":
		return true
	}
	if _, ok := scalarBuiltins[name]; ok {
		return true
	}
	if _, ok := unaryArrayBuiltins[name]; ok {
		return true
	}
	if _, ok := arrayAggBuiltins[name]; ok {
		return true
	}
	return false
}
