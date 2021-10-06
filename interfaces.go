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

// Function represents a generic callable function from the VM.
type Function interface {
	Invoke(params ...interface{}) (interface{}, error)
	ParamCount() int
	Returns() bool
}

// Library provides a collection of functions callable from the VM.
type Library interface {
	Function(name string) (Function, error)
}

// VariableStorage stores numeric variables.
type VariableStorage interface {
	Set(name string, value float64)
	Get(name string) (value float64, ok bool)
	Clear()
}

// Delegate receives events from the VM.
type Delegate interface {
	// Handle a line of dialogue
	Line(line string) error
	// Handle a comment
	Command(command string) error
	// User picks an option
	Options(options []string, pickedOption func(option int) error) error
	// This node is complete
	NodeComplete(nextNode string)
	// Dialogue is complete (usually program has stopped)
	DialogueComplete()
}
