// Copyright 2016 Josh Deprez
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

// Package yarn implements the YarnSpinner VM (see github.com/YarnSpinnerTool/YarnSpinner).
package yarn // import "github.com/DrJosh9000/yarn"

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
)

// BUG: This package hasn't been used or tested yet, and is incomplete.

var (
	ErrNoNodeSelected           = errors.New("no node selected to run")
	ErrWaitingOnOptionSelection = errors.New("waiting on option selection")
	ErrNilDialogueHandler       = errors.New("nil dialogue handler")
	ErrNilVariableStorage       = errors.New("nil variable storage")
	ErrMissingProgram           = errors.New("missing or empty program")
	ErrNoOptions                = errors.New("no options were added")
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// ExecState is the highest-level machine state.
type ExecState int

const (
	Stopped = ExecState(iota)
	WaitingOnOptionSelection
	Suspended
	Running
)

type state struct {
	node    *yarnpb.Node // current node
	pc      int          // program counter
	stack   []interface{}
	options []Option
}

// push pushes a value onto the state's stack.
func (s *state) push(x interface{}) { s.stack = append(s.stack, x) }

// pop removes a value from the stack and returns it.
func (s *state) pop() (interface{}, error) {
	x, err := s.peek()
	if err != nil {
		return nil, err
	}
	s.stack = s.stack[:len(s.stack)-1]
	return x, nil
}

// Reading N strings from the stack is common enough that I made a dedicated
// helper method for it.
func (s *state) popNStrings(n int) ([]string, error) {
	if n < 1 {
		return nil, fmt.Errorf("too few items requested [%d < 1]", n)
	}
	if n > len(s.stack) {
		return nil, fmt.Errorf("stack underflow [%d > %d]", n, len(s.stack))
	}
	rem := len(s.stack) - n
	ss := make([]string, n)
	for i, x := range s.stack[rem:] {
		t, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("wrong type from stack [%T != string]", x)
		}
		ss[i] = t
	}
	s.stack = s.stack[:rem]
	return ss, nil
}

// peek returns the top vaue from the stack only.
func (s *state) peek() (interface{}, error) {
	if len(s.stack) == 0 {
		return nil, errors.New("stack underflow")
	}
	return s.stack[len(s.stack)-1], nil
}

// Peek returns the string form the top of the stack.
func (s *state) peekString() (string, error) {
	x, err := s.peek()
	if err != nil {
		return "", err
	}
	t, ok := x.(string)
	if !ok {
		return "", fmt.Errorf("wrong type at top of stack [%T != string]", x)
	}
	return t, nil
}

// VirtualMachine implements the virtual machine.
type VirtualMachine struct {
	stateMu   sync.RWMutex
	execState ExecState
	state     state

	// Program to execute
	Program *yarnpb.Program

	Handler DialogueHandler
	Vars    VariableStorage
	FuncMap map[string]interface{} // works a bit like text/template.FuncMap
}

// SetNode sets the VM to begin a node.
func (vm *VirtualMachine) SetNode(name string) error {
	if vm.Program == nil || len(vm.Program.Nodes) == 0 {
		return ErrMissingProgram
	}
	node, found := vm.Program.Nodes[name]
	if !found {
		return fmt.Errorf("node %q not found", name)
	}
	// Reset the state and start at this node.
	vm.stateMu.Lock()
	vm.state = state{
		node: node,
	}
	vm.stateMu.Unlock()
	vm.Handler.NodeStart(name)
	return nil
}

// SetSelectedOption sets the option selected by the player. Call this once the
// player has chosen an option. The machine will be returned to Suspended state.
func (vm *VirtualMachine) SetSelectedOption(index int) error {
	vm.stateMu.Lock()
	defer vm.stateMu.Unlock()
	if vm.execState != WaitingOnOptionSelection {
		return fmt.Errorf("not waiting for an option selection [m.execState = %d]", vm.execState)
	}
	if optslen := len(vm.state.options); index < 0 || index >= optslen {
		return fmt.Errorf("selected option %d out of bounds [0, %d)", index, optslen)
	}
	vm.state.push(vm.state.options[index].DestinationNode)
	vm.state.options = vm.state.options[:0]
	vm.execState = Suspended
	return nil
}

// Stop stops the virtual machine.
func (vm *VirtualMachine) Stop() {
	vm.stateMu.Lock()
	vm.execState = Stopped
	vm.stateMu.Unlock()
}

func (vm *VirtualMachine) Continue() error {
	if vm.state.node == nil {
		return ErrNoNodeSelected
	}
	if vm.execState == WaitingOnOptionSelection {
		return ErrWaitingOnOptionSelection
	}
	if vm.Handler == nil {
		return ErrNilDialogueHandler
	}
	if vm.Vars == nil {
		return ErrNilVariableStorage
	}
	vm.execState = Running
	for vm.execState == Running {
		if err := vm.Execute(vm.state.node.Instructions[vm.state.pc]); err != nil {
			return err
		}
		vm.state.pc++
		if proglen := len(vm.state.node.Instructions); vm.state.pc >= proglen {
			vm.Handler.NodeComplete(vm.state.node.Name)
			vm.execState = Stopped
			vm.Handler.DialogueComplete()
		}
	}
	return nil
}

// Execute executes a single instruction.
func (vm *VirtualMachine) Execute(inst *yarnpb.Instruction) error {
	switch inst.Opcode {

	case yarnpb.Instruction_JUMP_TO:
		k := inst.Operands[0].GetStringValue()
		pc, ok := vm.state.node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.state.pc = int(pc) - 1

	case yarnpb.Instruction_JUMP:
		k, err := vm.state.peekString()
		if err != nil {
			return err
		}
		pc, ok := vm.state.node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.state.pc = int(pc) - 1

	case yarnpb.Instruction_RUN_LINE:
		line := Line{
			ID: inst.Operands[0].GetStringValue(),
		}
		if len(inst.Operands) > 1 {
			// Second operand gives number of values on stack to include as
			// substitutions.
			// NB: the bytecode only has floats, no ints.
			ss, err := vm.state.popNStrings(int(inst.Operands[1].GetFloatValue()))
			if err != nil {
				return err
			}
			line.Substitutions = ss
		}
		if vm.Handler.Line(line) == PauseExecution {
			vm.execState = Suspended
		}

	case yarnpb.Instruction_RUN_COMMAND:
		cmd := inst.Operands[0].GetStringValue()
		if len(inst.Operands) > 1 {
			// Second operand gives number of values on stack to interpolate
			// into the command as substitutions.
			// NB: the bytecode only has floats, no ints.
			ss, err := vm.state.popNStrings(int(inst.Operands[1].GetFloatValue()))
			if err != nil {
				return err
			}
			for i, s := range ss {
				cmd = strings.Replace(cmd, fmt.Sprintf("{%d}", i), s, -1)
			}
		}
		if vm.Handler.Command(cmd) == PauseExecution {
			vm.execState = Suspended
		}

	case yarnpb.Instruction_ADD_OPTION:
		line := Line{
			ID: inst.Operands[0].GetStringValue(),
		}
		if len(inst.Operands) > 2 {
			ss, err := vm.state.popNStrings(int(inst.Operands[2].GetFloatValue()))
			if err != nil {
				return err
			}
			line.Substitutions = ss
		}
		vm.state.options = append(vm.state.options, Option{
			ID:              len(vm.state.options),
			Line:            line,
			DestinationNode: inst.Operands[1].GetStringValue(),
		})

	case yarnpb.Instruction_SHOW_OPTIONS:
		if len(vm.state.options) == 0 {
			// NOTE: jon implements this as a machine stop instead of an exception
			return ErrNoOptions
		}
		vm.execState = WaitingOnOptionSelection
		vm.Handler.Options(vm.state.options)

	case yarnpb.Instruction_PUSH_STRING:
		vm.state.push(inst.Operands[0].GetStringValue())

	case yarnpb.Instruction_PUSH_FLOAT:
		vm.state.push(inst.Operands[0].GetFloatValue())

	case yarnpb.Instruction_PUSH_BOOL:
		vm.state.push(inst.Operands[0].GetBoolValue())

	case yarnpb.Instruction_PUSH_NULL:
		vm.state.push(nil)

	case yarnpb.Instruction_JUMP_IF_FALSE:
		x, err := vm.state.peek()
		if err != nil {
			return err
		}
		b, err := convertToBool(x)
		if err != nil {
			return err
		}
		if b {
			// Value is true, so don't jump
			return nil
		}
		k := inst.Operands[0].GetStringValue()
		pc, ok := vm.state.node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.state.pc = int(pc) - 1

	case yarnpb.Instruction_POP:
		if _, err := vm.state.pop(); err != nil {
			return err
		}

	case yarnpb.Instruction_CALL_FUNC:
		// TODO: typecheck FuncMap during preprocessing
		// TODO: a lot of this is very forgiving...
		k := inst.Operands[0].GetStringValue()
		f, found := vm.FuncMap[k]
		if !found {
			return fmt.Errorf("function %q not found", k)
		}
		ft := reflect.TypeOf(f)
		if ft.Kind() != reflect.Func {
			return fmt.Errorf("function %q not actually a function [type %T]", k, f)
		}
		// Compiler puts number of args on top of stack
		gotx, err := vm.state.pop()
		if err != nil {
			return err
		}
		got, err := convertToInt(gotx)
		if err != nil {
			return err
		}
		// Check that we have enough args to call the func
		switch want := ft.NumIn(); {
		case ft.IsVariadic() && got < want-1:
			// The last (variadic) arg is free to be empty.
			return fmt.Errorf("insufficient args [%d < %d]", got, want)
		case got != want:
			return fmt.Errorf("wrong number of args [%d != %d]", got, want)
		}

		// Also check that f either returns {0,1,2} values; the second is
		// only allowed to be type error.
		switch {
		case ft.NumOut() == 2 && ft.Out(1) == errorType:
			// ok
		case ft.NumOut() < 2:
			// ok
		default:
			// TODO: elaborate in message
			return errors.New("wrong number or type of return args")
		}

		params := make([]reflect.Value, got)
		for got >= 0 {
			got--
			p, err := vm.state.pop()
			if err != nil {
				return err
			}
			params[got] = reflect.ValueOf(p)
		}

		result := reflect.ValueOf(f).Call(params)
		if len(result) == 2 && !result[1].IsNil() {
			return result[1].Interface().(error)
		}
		if len(result) > 0 {
			vm.state.push(result[0].Interface())
		}

	case yarnpb.Instruction_PUSH_VARIABLE:
		k := inst.Operands[0].GetStringValue()
		v, ok := vm.Vars.GetValue(k)
		if !ok {
			return fmt.Errorf("no variable called %q", k)
		}
		vm.state.push(v)

	case yarnpb.Instruction_STORE_VARIABLE:
		k := inst.Operands[0].GetStringValue()
		v, err := vm.state.peek()
		if err != nil {
			return err
		}
		x, err := convertToFloat(v)
		if err != nil {
			return err
		}
		vm.Vars.SetValue(k, x)

	case yarnpb.Instruction_STOP:
		vm.Handler.NodeComplete(vm.state.node.Name)
		vm.Handler.DialogueComplete()
		vm.execState = Stopped

	case yarnpb.Instruction_RUN_NODE:
		node := ""
		if len(inst.Operands) == 0 || inst.Operands[0].GetStringValue() == "" {
			// Use the stack, Luke.
			t, err := vm.state.peekString()
			if err != nil {
				return err
			}
			node = t
		} else {
			node = inst.Operands[0].GetStringValue()
		}
		pause := vm.Handler.NodeComplete(vm.state.node.Name)
		if err := vm.SetNode(node); err != nil {
			return err
		}
		if pause == PauseExecution {
			vm.execState = Suspended
		}

	default:
		return fmt.Errorf("invalid opcode %v", inst.Opcode)
	}
	return nil
}
