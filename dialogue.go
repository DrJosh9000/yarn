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

// Line represents a line of dialogue.
type Line struct {
	// The string ID for the line.
	ID string
	// Values that should be interpolated into the user-facing text.
	Substitutions []string
}

// Option represents one option (among others) that the player could
// choose.
type Option struct {
	// A number identifying this option. If this option is selected, pass
	// this number back to the dialogue system.
	ID int

	// The line that should be presented for this option.
	Line Line

	// Name of the node to run if this option is selected.
	DestinationNode string
}

// MapVariableStorage implements VariableStorage, in memory, using a map.
type MapVariableStorage map[string]interface{}

// Clear empties the storage of all values.
func (m MapVariableStorage) Clear() {
	for name := range m {
		delete(m, name)
	}
}

// Get fetches a value from the map.
func (m MapVariableStorage) GetValue(name string) (value interface{}, found bool) {
	value, found = m[name]
	return value, found
}

// Set sets a value in the map.
func (m MapVariableStorage) SetValue(name string, value interface{}) {
	m[name] = value
}

// HandlerExecutionType values control what the dialogue system does when
// control is passed to a handler.
type HandlerExecutionType int

const (
	// The dialogue system should suspend execution during the handler.
	PauseExecution = HandlerExecutionType(iota)

	// The dialogue system should continue executing.
	ContinueExecution
)

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
	Clear()
	GetValue(name string) (value interface{}, ok bool)
	SetValue(name string, value interface{})
}

// DialogueHandler receives events from the VM.
type DialogueHandler interface {
	// Line is called when the dialogue system runs a line of dialogue.
	Line(Line) HandlerExecutionType

	// Options is called to deliver a set of options to the game. The player
	// should choose one of the options. The dialogue system must always wait
	// for an option to be chosen before continuing.
	Options([]Option)

	// Command is called when the dialogue system runs a command.
	Command(string) HandlerExecutionType

	// NodeStart is called when a node has begun executing. It is passed the
	// name of the node.
	NodeStart(string) HandlerExecutionType

	// NodeComplete is called when a node has completed execution. It is passed
	// the name of the node.
	NodeComplete(string) HandlerExecutionType

	// DialogueComplete is called when the dialogue as a whole is complete.
	DialogueComplete()
}
