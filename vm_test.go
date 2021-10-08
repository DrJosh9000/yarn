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
	"io/ioutil"
	"log"
	"os"
	"testing"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

const traceOutput = false

func TestVMExample(t *testing.T) {
	tpf, err := os.Open("testdata/Example.testplan")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	defer tpf.Close()
	testplan, err := ReadTestPlan(tpf)
	if err != nil {
		t.Fatalf("ReadTestPlan: %v", err)
	}

	yarnc, err := ioutil.ReadFile("testdata/Example.yarn.yarnc")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var prog yarnpb.Program
	if err := proto.Unmarshal(yarnc, &prog); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	if traceOutput {
		log.Print(FormatProgram(&prog))
	}

	vm := &VirtualMachine{
		Program:  &prog,
		Handler:  testplan,
		Vars:     make(MapVariableStorage),
		TraceLog: traceOutput,
	}
	testplan.VM = vm

	if err := vm.SetNode("Start"); err != nil {
		t.Errorf("vm.SetNode(Start) = %v", err)
	}
	for {
		if err := vm.Continue(); err != nil {
			t.Errorf("vm.Continue() = %v", err)
			break
		}
		if vm.execState == stopped {
			break
		}
	}
	if err := testplan.Complete(); err != nil {
		t.Errorf("testplan incomplete: %v", err)
	}
}
