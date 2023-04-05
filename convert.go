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
	"strconv"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
)

// ConvertToBool attempts conversion of the standard Yarn Spinner VM types
// (bool, number, string, null) to bool.
func ConvertToBool(x interface{}) (bool, error) {
	if x == nil {
		return false, nil
	}
	switch x := x.(type) {
	case bool:
		return x, nil
	case float32:
		return !math.IsNaN(float64(x)) && x != 0, nil
	case float64:
		return !math.IsNaN(x) && x != 0, nil
	case int:
		return x != 0, nil
	case string:
		return x != "", nil
	default:
		return false, fmt.Errorf("%T %w to bool", x, ErrNotConvertible)
	}
}

// ConvertToInt attempts conversion of the standard Yarn Spinner VM types to
// (bool, number, string, null) to int.
func ConvertToInt(x interface{}) (int, error) {
	if x == nil {
		return 0, nil
	}
	switch t := x.(type) {
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
	case float32:
		return int(t), nil
	case float64:
		return int(t), nil
	case int:
		return t, nil
	case string:
		return strconv.Atoi(t)
	default:
		if t == nil {
			return 0, nil
		}
		return 0, fmt.Errorf("%T %w to int", x, ErrNotConvertible)
	}
}

// ConvertToFloat32 attempts conversion of the standard Yarn Spinner VM types
// (bool, number, string, null) to a float32.
func ConvertToFloat32(x interface{}) (float32, error) {
	if x == nil {
		return 0, nil
	}
	switch t := x.(type) {
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
	case float32:
		return t, nil
	case float64:
		return float32(t), nil
	case int:
		return float32(t), nil
	case string:
		y, err := strconv.ParseFloat(t, 32)
		if err != nil {
			return 0, err
		}
		return float32(y), nil
	default:
		if t == nil {
			return 0, nil
		}
		return 0, fmt.Errorf("%T %w to float32", x, ErrNotConvertible)
	}
}

// ConvertToFloat64 attempts conversion of the standard Yarn Spinner VM types
// (bool, number, string, null) to a float64.
func ConvertToFloat64(x interface{}) (float64, error) {
	if x == nil {
		return 0, nil
	}
	switch t := x.(type) {
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
	case float32:
		return float64(t), nil
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	case string:
		return strconv.ParseFloat(t, 64)
	default:
		if t == nil {
			return 0, nil
		}
		return 0, fmt.Errorf("%T %w to float64", x, ErrNotConvertible)
	}
}

// ConvertToString converts a value to a string, in a way that matches what Yarn
// Spinner does. nil becomes "null", and booleans are title-cased.
func ConvertToString(x interface{}) string {
	if x == nil {
		return "null"
	}
	if x, ok := x.(bool); ok {
		if x {
			return "True"
		}
		return "False"
	}
	return fmt.Sprint(x)
}

// operandToInt is a helper for turning a number value into an int.
func operandToInt(op *yarnpb.Operand) (int, error) {
	if op == nil {
		return 0, ErrNilOperand
	}
	f, ok := op.Value.(*yarnpb.Operand_FloatValue)
	if !ok {
		return 0, fmt.Errorf("%w for operand [%T != Operand_FloatValue]", ErrWrongType, op.Value)
	}
	return int(f.FloatValue), nil
}
