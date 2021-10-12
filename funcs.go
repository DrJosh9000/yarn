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

// FuncMap works like text/template.FuncMap. It maps function names to
// implementations. Each implementation must return 0, 1, or 2 args.
type FuncMap map[string]interface{}

// merge merges fm into m and returns m.
func (m FuncMap) merge(fm FuncMap) FuncMap {
	for n, f := range fm {
		m[n] = f
	}
	return m
}

// defaultFuncMap returns a FuncMap with all the basic YarnSpinner operators.
func defaultFuncMap() FuncMap {
	return FuncMap{
		"None":                 func(x interface{}) interface{} { return x },
		"EqualTo":              func(x, y interface{}) bool { return x == y },
		"GreaterThan":          funcGreaterThan,
		"GreaterThanOrEqualTo": funcGreaterThanOrEqualTo,
		"LessThan":             funcLessThan,
		"LessThanOrEqualTo":    funcLessThanOrEqualTo,
		"NotEqualTo":           func(x, y interface{}) bool { return x != y },
		"Or":                   func(x, y bool) bool { return x || y },
		"And":                  func(x, y bool) bool { return x && y },
		"Xor":                  func(x, y bool) bool { return x != y },
		"Not":                  func(x bool) bool { return !x },
		"UnaryMinus":           funcUnaryMinus,
		"Add":                  funcAdd,
		"Minus":                func(x, y float32) float32 { return x - y },
		"Multiply":             func(x, y float32) float32 { return x * y },
		"Divide":               func(x, y float32) float32 { return x / y },
		"Modulo":               func(x, y float32) float32 { return float32(int(x) % int(y)) },
	}
}

func funcGreaterThan(x, y interface{}) (bool, error) {
	switch xt := x.(type) {
	case string:
		yt, ok := y.(string)
		if !ok {
			return false, fmt.Errorf("mismatching types [%T != string]", y)
		}
		return xt > yt, nil
	case float32:
		yt, ok := y.(float32)
		if !ok {
			return false, fmt.Errorf("mismatching types [%T != float32]", y)
		}
		return xt > yt, nil
	}
	return false, fmt.Errorf("unsupported type [%T ∉ {float32,string}]", x)
}

func funcGreaterThanOrEqualTo(x, y interface{}) (bool, error) {
	if x == y {
		return true, nil
	}
	return funcGreaterThan(x, y)
}

func funcLessThan(x, y interface{}) (bool, error) {
	return funcGreaterThan(y, x)
}

func funcLessThanOrEqualTo(x, y interface{}) (bool, error) {
	return funcGreaterThanOrEqualTo(y, x)
}

func funcUnaryMinus(x interface{}) (interface{}, error) {
	xt, ok := x.(float32)
	if !ok {
		return nil, fmt.Errorf("unsupported type [%T != float32]", x)
	}
	return -xt, nil
}

func funcAdd(x, y interface{}) (interface{}, error) {
	switch xt := x.(type) {
	case string:
		yt, ok := y.(string)
		if !ok {
			return false, fmt.Errorf("mismatching types [%T != string]", y)
		}
		return xt + yt, nil
	case float32:
		yt, ok := y.(float32)
		if !ok {
			return false, fmt.Errorf("mismatching types [%T != float32]", y)
		}
		return xt + yt, nil
	}
	return false, fmt.Errorf("unsupported type [%T ∉ {float32,string}]", x)
}
