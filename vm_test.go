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
	"path/filepath"
	"strings"
	"testing"
)

const traceOutput = false

func TestAllTestPlans(t *testing.T) {
	testplans, err := filepath.Glob("testdata/*.testplan")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	for _, tpn := range testplans {
		t.Run(tpn, func(t *testing.T) {
			testplan, err := LoadTestPlanFile(tpn)
			if err != nil {
				t.Fatalf("LoadTestPlanFile(%q) = error %v", tpn, err)
			}

			base := strings.TrimSuffix(filepath.Base(tpn), ".testplan")

			yarnc := "testdata/" + base + ".yarn.yarnc"
			csv := "testdata/" + base + ".yarn.csv"
			prog, st, err := LoadFiles(yarnc, csv, "en")
			if err != nil {
				t.Fatalf("LoadFiles(%q, %q, en) = error %v", yarnc, csv, err)
			}

			vm := &VirtualMachine{
				Program: prog,
				Handler: testplan,
				Vars:    make(MapVariableStorage),
				FuncMap: FuncMap{
					// Used by various
					"assert": func(x interface{}) error {
						t, err := convertToBool(x)
						if err != nil {
							return err
						}
						if !t {
							return errors.New("assertion failed")
						}
						return nil
					},
					// Used by Functions.yarn
					// TODO: use ints like the real YarnSpinner
					"add_three_operands": func(x, y, z float32) float32 {
						return x + y + z
					},
					"last_value": func(x ...interface{}) (interface{}, error) {
						if len(x) == 0 {
							return nil, errors.New("no args")
						}
						return x[len(x)-1], nil
					},
				},
			}
			testplan.StringTable = st
			if traceOutput {
				vm.TraceLogf = t.Logf
			}

			if err := vm.Run("Start"); err != nil {
				t.Errorf("vm.Run(Start) = %v", err)
			}
			if err := testplan.Complete(); err != nil {
				t.Errorf("testplan incomplete: %v", err)
			}
		})
	}
}
