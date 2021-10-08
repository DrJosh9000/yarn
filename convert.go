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
	"errors"
	"fmt"
	"strconv"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
)

func convertToBool(x interface{}) (bool, error) {
	if x == nil {
		return false, nil
	}
	switch t := x.(type) {
	case bool:
		return t, nil
	case float64:
		return t != 0, nil
	case int:
		return t != 0, nil
	case string:
		return len(t) > 0, nil
	default:
		if t == nil {
			return false, nil
		}
		return false, fmt.Errorf("cannot convert value of type %T to a bool", x)
	}
}

func convertToInt(x interface{}) (int, error) {
	if x == nil {
		return 0, nil
	}
	switch t := x.(type) {
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
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
		return 0, fmt.Errorf("cannot convert value of type %T to int", x)
	}
}

func operandToInt(op *yarnpb.Operand) (int, error) {
	if op == nil {
		return 0, errors.New("nil operand")
	}
	f, ok := op.Value.(*yarnpb.Operand_FloatValue)
	if !ok {
		return 0, fmt.Errorf("wrong operand type [%T != Operand_FloatValue]", op.Value)
	}
	return int(f.FloatValue), nil
}
