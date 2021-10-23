// Copyright 2021 Josh Deprez
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package yarn

import (
	"fmt"
)

// FuncMap maps function names to implementations. Each function must return 0,
// 1, or 2 values, and if 2 are returned, the latter must be type `error`.
// It is similar to the text/template FuncMap.
type FuncMap map[string]interface{}

// merge merges fm into m and returns m.
func (m FuncMap) merge(fm FuncMap) FuncMap {
	for n, f := range fm {
		m[n] = f
	}
	return m
}

// defaultFuncMap returns a FuncMap with the standard Yarn Spinner operators.
func defaultFuncMap() FuncMap {
	return FuncMap{
		"None":                 func(x interface{}) interface{} { return x },
		"EqualTo":              func(x, y interface{}) bool { return x == y },
		"GreaterThan":          func(x, y float32) bool { return x > y },
		"GreaterThanOrEqualTo": func(x, y float32) bool { return x >= y },
		"LessThan":             func(x, y float32) bool { return x < y },
		"LessThanOrEqualTo":    func(x, y float32) bool { return x <= y },
		"NotEqualTo":           func(x, y interface{}) bool { return x != y },
		// numbers are truthy, hence boolean operators are generic
		"Or":         funcOr,
		"And":        funcAnd,
		"Xor":        funcXor,
		"Not":        funcNot,
		"UnaryMinus": func(x float32) float32 { return -x },
		"Add":        funcAdd,
		"Minus":      func(x, y float32) float32 { return x - y },
		"Multiply":   func(x, y float32) float32 { return x * y },
		"Divide":     func(x, y float32) float32 { return x / y },
		"Modulo":     func(x, y float32) float32 { return float32(int(x) % int(y)) },
	}
}

func funcOr(x, y interface{}) (bool, error) {
	xt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("first arg: %w", err)
	}
	yt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("second arg: %w", err)
	}
	return xt || yt, nil
}

func funcAnd(x, y interface{}) (bool, error) {
	xt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("first arg: %w", err)
	}
	yt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("second arg: %w", err)
	}
	return xt && yt, nil
}

func funcXor(x, y interface{}) (bool, error) {
	xt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("first arg: %w", err)
	}
	yt, err := convertToBool(x)
	if err != nil {
		return false, fmt.Errorf("second arg: %w", err)
	}
	return xt != yt, nil
}

func funcNot(x interface{}) (bool, error) {
	t, err := convertToBool(x)
	return !t, err
}

func funcAdd(x, y interface{}) (interface{}, error) {
	if x == nil {
		return y, nil
	}
	if y == nil {
		return x, nil
	}
	switch xt := x.(type) {
	case string:
		return xt + convertToString(y), nil
	case float32:
		switch yt := y.(type) {
		case float32:
			return xt + yt, nil
		case string:
			return convertToString(x) + yt, nil
		default:
			return nil, fmt.Errorf("mismatching types [first arg float32, second arg %T]", y)
		}
	}
	return false, fmt.Errorf("unsupported type [%T ∉ {nil,float32,string}]", x)
}
