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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

const traceOutput = false

func TestAllTestPlans(t *testing.T) {
	testplans, err := filepath.Glob("testdata/*.testplan")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	for _, tpn := range testplans {
		t.Run(tpn, func(t *testing.T) {
			tpf, err := os.Open(tpn)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer tpf.Close()
			testplan, err := ReadTestPlan(tpf)
			if err != nil {
				t.Fatalf("ReadTestPlan: %v", err)
			}

			base := strings.TrimSuffix(filepath.Base(tpn), ".testplan")

			yarnc, err := ioutil.ReadFile("testdata/" + base + ".yarn.yarnc")
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var prog yarnpb.Program
			if err := proto.Unmarshal(yarnc, &prog); err != nil {
				t.Fatalf("proto.Unmarshal: %v", err)
			}

			csv, err := os.Open("testdata/" + base + ".yarn.csv")
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer csv.Close()
			st, err := ReadStringTable(csv)
			if err != nil {
				t.Fatalf("ReadStringTable: %v", err)
			}

			if traceOutput {
				log.Print(FormatProgram(&prog))
			}

			vm := &VirtualMachine{
				Program: &prog,
				Handler: testplan,
				Vars:    make(MapVariableStorage),
				FuncMap: FuncMap{
					// Used by various
					"assert": func(x bool) error {
						if !x {
							return errors.New("assertion failed")
						}
						return nil
					},
					// Used by Functions.yarn
					// TODO: use ints like the real YarnSpinner
					"add_three_operands": func(x, y, z float32) float32 {
						return x + y + z
					},
				},
				TraceLog: traceOutput,
			}
			testplan.StringTable = st
			testplan.VirtualMachine = vm

			if err := vm.Run("Start"); err != nil {
				t.Errorf("vm.Run() = %v", err)
			}
			if err := testplan.Complete(); err != nil {
				t.Errorf("testplan incomplete: %v", err)
			}
		})
	}
}
