// Copyright 2023 Josh Deprez
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
	"sync/atomic"
)

// ErrAlreadyStopped is returned when the AsyncAdapter cannot
// stop the virtual machine, because it is already stopped.
const ErrAlreadyStopped = virtualMachineError("VM already stopped or stopping")

var _ DialogueHandler = &AsyncAdapter{}

// VMState enumerates the different states that AsyncAdapter can be in.
type VMState int32

const (
	// No event has been delivered (since the last call to Go / GoWithOption);
	// the VM is executing.
	VMStateRunning = iota

	// An event other than Options was delivered, and VM execution is blocked.
	VMStatePaused

	// Options event was delivered, and VM execution is blocked.
	VMStatePausedOptions

	// Execution has not begun, or has ended (e.g. by calling Abort, or any
	// other error).
	VMStateStopped
)

func (s VMState) String() string {
	switch s {
	case VMStateRunning:
		return "Running"
	case VMStatePaused:
		return "Paused"
	case VMStatePausedOptions:
		return "PausedOptions"
	case VMStateStopped:
		return "Stopped"
	}
	return fmt.Sprintf("(invalid VMState %d)", s)
}

// VMStateMismatchErr is returned when AsyncAdapter is told to do something
// (either by the user calling Go, GoWithChoice, or Abort, or the VM calling a
// DialogueHandler method) but this requires AsyncAdapter to be in a different
// state than the state it is in.
type VMStateMismatchErr struct {
	// The VM was in state Got, but we wanted it to be in state Want in order
	// to change it to state Next.
	Got, Want, Next VMState
}

func (e VMStateMismatchErr) Error() string {
	return fmt.Sprintf("VM is %v, so cannot transition from %v to %v", e.Got, e.Want, e.Next)
}

// AsyncAdapter is a DialogueHandler that exposes an interface that is similar
// to the mainline YarnSpinner VM dialogue handler. Instead of manually blocking
// inside the DialogueHandler callbacks, AsyncAdapter does this for you, until
// you call Go, GoWithChoice, or Abort (as appropriate).
type AsyncAdapter struct {
	state   atomic.Int32
	handler AsyncDialogueHandler
	msgCh   chan asyncMsg
}

// NewAsyncAdapter returns a new AsyncAdapter.
func NewAsyncAdapter(h AsyncDialogueHandler) *AsyncAdapter {
	return &AsyncAdapter{
		handler: h,
		// The user might call Go from within their handler's Line method
		// (or however many other ways to try to continue the VM immediately).
		// If msgCh was unbuffered, calling Go would wait forever trying to send
		// on the channel, because AsyncAdapter only receives on msgCh after
		// their method returns.
		msgCh: make(chan asyncMsg, 1),
	}
}

// State returns the current state.
func (a *AsyncAdapter) State() VMState {
	return VMState(a.state.Load())
}

func (a *AsyncAdapter) stateTransition(old, new int32) error {
	if !a.state.CompareAndSwap(old, new) {
		// This races (between CAS and a.State, something else could switch the
		// state around). While I try to make the error maximally useful for
		// debugging ... YOLO?
		return VMStateMismatchErr{
			Got:  a.State(),
			Want: VMState(old),
			Next: VMState(new),
		}
	}
	return nil
}

// Go will continue the VM after it has delivered any event (other than
// Options). If the VM is not paused following any event other than Options, an
// error will be returned.
func (a *AsyncAdapter) Go() error {
	if err := a.stateTransition(VMStatePaused, VMStateRunning); err != nil {
		return err
	}
	a.msgCh <- goMsg{}
	return nil
}

// GoWithChoice will continue the VM after it has delivered an Options event.
// Pass the ID of the chosen option. If the VM is not paused following an
// Options event, an error will be returned.
func (a *AsyncAdapter) GoWithChoice(id int) error {
	if err := a.stateTransition(VMStatePausedOptions, VMStateRunning); err != nil {
		return err
	}
	a.msgCh <- choiceMsg{id}
	return nil
}

// Abort stops the VM with the given error as soon as possible (either within
// the current event, or on the next event). If a nil error is passed, Abort
// will replace it with Stop (so that NodeComplete and DialogueComplete still
// fire). If the VM is already stopped (either through Abort, or after the
// DialogueComplete event) an error will be returned.
func (a *AsyncAdapter) Abort(err error) error {
	if old := a.state.Swap(VMStateStopped); old == VMStateStopped {
		return ErrAlreadyStopped
	}
	if err == nil {
		err = Stop
	}
	a.msgCh <- abortMsg{err}
	return nil
}

// waitForGo waits for Go or Abort to be called.
func (a *AsyncAdapter) waitForGo() error {
	switch msg := (<-a.msgCh).(type) {
	case goMsg:
		return nil
	case choiceMsg:
		// This is incredibly unlikely, but I check it anyway.
		return errors.New("AsyncAdapter.GoWithChoice called, but last event was not Options")
	case abortMsg:
		return msg.err
	default:
		return fmt.Errorf("invalid message type %T received", msg)
	}
}

// waitForChoice waits for GoWithChoice or Abort to be called.
func (a *AsyncAdapter) waitForChoice() (int, error) {
	switch msg := (<-a.msgCh).(type) {
	case goMsg:
		// This is incredibly unlikely, but I check it anyway.
		return -1, errors.New("AsyncAdapter.Go called, but last event was Options")
	case choiceMsg:
		return msg.choice, nil
	case abortMsg:
		return -1, msg.err
	default:
		return -1, fmt.Errorf("invalid message type %T received", msg)
	}
}

// --- DialogueHandler implementation --- \\

// NodeStart is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) NodeStart(nodeName string) error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.NodeStart(nodeName)
	return a.waitForGo()
}

// PrepareForLines is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) PrepareForLines(lineIDs []string) error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.PrepareForLines(lineIDs)
	return a.waitForGo()
}

// Line is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) Line(line Line) error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.Line(line)
	return a.waitForGo()
}

// Options is called by the VM and blocks until GoWithChoice or Abort is called.
func (a *AsyncAdapter) Options(options []Option) (int, error) {
	if err := a.stateTransition(VMStateRunning, VMStatePausedOptions); err != nil {
		return -1, err
	}
	a.handler.Options(options)
	return a.waitForChoice()
}

// Command is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) Command(command string) error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.Command(command)
	return a.waitForGo()
}

// NodeComplete is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) NodeComplete(nodeName string) error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.NodeComplete(nodeName)
	return a.waitForGo()

}

// DialogueComplete is called by the VM and blocks until Go or Abort is called.
func (a *AsyncAdapter) DialogueComplete() error {
	if err := a.stateTransition(VMStateRunning, VMStatePaused); err != nil {
		return err
	}
	a.handler.DialogueComplete()
	return a.waitForGo()
}

// --- AsyncAdapter messages --- \\

// AsyncAdapter works by waiting on a channel. The three message types are below.
type asyncMsg interface {
	asyncMsgTag()
}

// Sent on the channel when Go is called.
type goMsg struct{}

func (goMsg) asyncMsgTag() {}

// Sent on the channel when GoWithChoice is called.
type choiceMsg struct {
	choice int
}

func (choiceMsg) asyncMsgTag() {}

// Sent on the channel when Abort is called.
type abortMsg struct {
	err error
}

func (abortMsg) asyncMsgTag() {}
