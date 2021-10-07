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
	"io/ioutil"
	"testing"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
	"google.golang.org/protobuf/proto"
)

type call struct {
	method string
	arg    interface{}
}

func (c call) String() string {
	return fmt.Sprintf("%s(%v)", c.method, c.arg)
}

type fakeHandler struct {
	calls []call
}

func (f *fakeHandler) Line(line Line) {
	f.calls = append(f.calls, call{"Line", line})
}

func (f *fakeHandler) Options(opts []Option) {
	f.calls = append(f.calls, call{"Options", opts})
}

func (f *fakeHandler) Command(command string) {
	f.calls = append(f.calls, call{"Command", command})
}

func (f *fakeHandler) NodeStart(nodeName string) {
	f.calls = append(f.calls, call{"NodeStart", nodeName})
}

func (f *fakeHandler) NodeComplete(nodeName string) {
	f.calls = append(f.calls, call{"NodeComplete", nodeName})
}

func (f *fakeHandler) DialogueComplete() {
	f.calls = append(f.calls, call{"DialogueComplete", nil})
}

func (f *fakeHandler) PrepareForLines(lineIDs []string) {
	f.calls = append(f.calls, call{"PrepareForLines", lineIDs})
}

func TestVMWithExample(t *testing.T) {
	yarnc, err := ioutil.ReadFile("testdata/Example.yarn.yarnc")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var prog yarnpb.Program
	if err := proto.Unmarshal(yarnc, &prog); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	vm := &VirtualMachine{
		Program: &prog,
		Handler: &fakeHandler{},
		Vars:    make(MapVariableStorage),
	}

	if err := vm.SetNode("Start"); err != nil {
		t.Errorf("vm.SetNode(Start) = %v", err)
	}
	if err := vm.Continue(); err != nil {
		t.Errorf("vm.Continue() = %v", err)
	}
}
