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

// HandlerExecutionType values control what the dialogue system does when
// control is passed to a handler.
type HandlerExecutionType int

const (
	// The dialogue system should suspend execution during the handler.
	PauseExecution = HandlerExecutionType(iota)

	// The dialogue system should continue executing.
	ContinueExecution
)

// DialogueHandler receives events from the VM.
type DialogueHandler interface {
	// Line is called when the dialogue system runs a line of dialogue.
	Line(Line) HandlerExecutionType

	// Options is called to deliver a set of options to the game. The player
	// should choose one of the options. The dialogue system must always wait
	// for an option to be chosen before continuing.
	Options([]Option)

	// Command is called when the dialogue system runs a command.
	Command(command string) HandlerExecutionType

	// NodeStart is called when a node has begun executing. It is passed the
	// name of the node.
	NodeStart(nodeName string) HandlerExecutionType

	// NodeComplete is called when a node has completed execution. It is passed
	// the name of the node.
	NodeComplete(nodeName string) HandlerExecutionType

	// DialogueComplete is called when the dialogue as a whole is complete.
	DialogueComplete()

	// PrepareForLines is called when the dialogue system anticipates that it
	// will deliver some lines. Note that not every line prepared may end up
	// being run.
	PrepareForLines(lineIDs []string)
}
