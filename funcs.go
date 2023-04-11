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
	"math"
	"math/big"
	"math/rand"
)

// FuncMap maps function names to implementations.  It is similar to the
// text/template FuncMap.
//
// Each function must return either 0, 1, or 2 values, and if 2 are returned,
// the latter must be type `error`.
//
// If the arguments being passed by the program are not assignable to an
// argument, and the argument has type bool, int, float32, float64, or string,
// then a conversion is attempted by the VM. For example, if the stack has the
// values ("3", true, 2) on top, CALL_FUNC with "Number.Add" (see below) would
// cause Number.Add's implementation to be called with (3.0, 1.0) (the 2 is the
// argument count).
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
		// --- Non-method funcs (old skool) ---
		"None":                 func(x interface{}) interface{} { return x },
		"EqualTo":              func(x, y interface{}) bool { return x == y },
		"NotEqualTo":           func(x, y interface{}) bool { return x != y },
		"GreaterThan":          func(x, y float32) bool { return x > y },
		"GreaterThanOrEqualTo": func(x, y float32) bool { return x >= y },
		"LessThan":             func(x, y float32) bool { return x < y },
		"LessThanOrEqualTo":    func(x, y float32) bool { return x <= y },
		"Or":                   func(x, y bool) bool { return x || y },
		"And":                  func(x, y bool) bool { return x && y },
		"Xor":                  func(x, y bool) bool { return x != y },
		"Not":                  func(x bool) bool { return !x },
		"UnaryMinus":           func(x float32) float32 { return -x },
		// Add can't use implicit conversion, because it does something
		// different depending on the argument types.
		"Add":      funcAdd,
		"Minus":    func(x, y float32) float32 { return x - y },
		"Multiply": func(x, y float32) float32 { return x * y },
		"Divide":   func(x, y float32) float32 { return x / y },
		"Modulo":   func(x, y int) float32 { return float32(x % y) },

		// --- Method funcs (2.0) ---
		"Bool.EqualTo":                func(x, y bool) bool { return x == y },
		"Bool.NotEqualTo":             func(x, y bool) bool { return x != y },
		"Bool.Or":                     func(x, y bool) bool { return x || y },
		"Bool.And":                    func(x, y bool) bool { return x && y },
		"Bool.Xor":                    func(x, y bool) bool { return x != y },
		"Bool.Not":                    func(x bool) bool { return !x },
		"Number.EqualTo":              func(x, y float32) bool { return x == y },
		"Number.NotEqualTo":           func(x, y float32) bool { return x != y },
		"Number.Add":                  func(x, y float32) float32 { return x + y },
		"Number.Minus":                func(x, y float32) float32 { return x - y },
		"Number.Multiply":             func(x, y float32) float32 { return x * y },
		"Number.Divide":               func(x, y float32) float32 { return x / y },
		"Number.Modulo":               func(x, y int) float32 { return float32(x % y) },
		"Number.UnaryMinus":           func(x float32) float32 { return -x },
		"Number.GreaterThan":          func(x, y float32) bool { return x > y },
		"Number.GreaterThanOrEqualTo": func(x, y float32) bool { return x >= y },
		"Number.LessThan":             func(x, y float32) bool { return x < y },
		"Number.LessThanOrEqualTo":    func(x, y float32) bool { return x <= y },
		"String.EqualTo":              func(x, y string) bool { return x == y },
		"String.NotEqualTo":           func(x, y string) bool { return x != y },
		"String.Add":                  func(x, y string) string { return x + y },

		// built-in functions from documentation.
		"random":       func() float32 { return rand.Float32() },
		"random_range": func(x, y int) float32 { return float32(rand.Intn(y-x) + x) },
		"dice":         func(x int) float32 { return float32(rand.Intn(x) + 1) },
		"round":        func(x float32) float32 { return float32(math.Round(float64(x))) },
		"round_places": func(n float32, places uint) float32 {
			f := new(big.Float).SetMode(big.ToNearestEven).SetPrec(places).SetFloat64(float64(n))
			result, _ := f.Float32()
			return result
		},
		"floor":   func(n float32) float32 { return float32(math.Floor(float64(n))) },
		"ceil":    func(n float32) float32 { return float32(math.Ceil(float64(n))) },
		"inc":     func(n float32) float32 { return float32(math.Trunc(float64(n)) + 1) },
		"dec":     func(n float32) float32 { return float32(math.Ceil(float64(n)) - 1) },
		"decimal": func(n float32) float32 { _, f := math.Modf(float64(n)); return float32(f) },
	}
}

func funcAdd(x, y interface{}) (interface{}, error) {
	if x == nil {
		return y, nil
	}
	if y == nil {
		return x, nil
	}
	// Try strings first
	if xt, ok := x.(string); ok {
		return xt + ConvertToString(y), nil
	}
	if yt, ok := y.(string); ok {
		return ConvertToString(x) + yt, nil
	}
	// numeric, probably
	switch xt := x.(type) {
	case bool:
		// upconvert both to numbers
		xtt, err := ConvertToFloat32(x)
		if err != nil {
			return nil, err
		}
		yt, err := ConvertToFloat32(y)
		if err != nil {
			return nil, err
		}
		return xtt + yt, nil
	case float32:
		yt, err := ConvertToFloat32(y)
		if err != nil {
			return nil, err
		}
		return xt + yt, nil
	case float64:
		yt, err := ConvertToFloat64(y)
		if err != nil {
			return nil, err
		}
		return xt + yt, nil
	case int:
		yt, err := ConvertToInt(y)
		if err != nil {
			return nil, err
		}
		return xt + yt, nil
	}
	return false, fmt.Errorf("unsupported type [%T ∉ {nil,bool,float32,float64,int,string}]", x)
}
