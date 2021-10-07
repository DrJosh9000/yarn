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

// Various sentinel errors.
var (
	// ErrNoNodeSelected indicates the VM tried to run but SetNode hadn't been
	// called.
	ErrNoNodeSelected = errors.New("no node selected to run")

	// ErrWaitingOnOptionSelection indicates the VM delivered options to the
	// handler, but an option hasn't been selected (with SetSelectedOption).
	ErrWaitingOnOptionSelection = errors.New("waiting on option selection")

	// ErrNilDialogueHandler indicates that Handler hasn't been set.
	ErrNilDialogueHandler = errors.New("nil dialogue handler")

	// ErrNilVariableStorage indicates that Vars hasn't been set.
	ErrNilVariableStorage = errors.New("nil variable storage")

	// ErrMissingProgram indicates that Program hasn't been set.
	ErrMissingProgram = errors.New("missing or empty program")

	// ErrNoOptions indicates the program is invalid - it tried to show options
	// but none had been added.
	ErrNoOptions = errors.New("no options were added")
)

// Used to check the second return arg of functions in FuncMap.
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// execState is the highest-level machine state.
type execState int

const (
	// The Virtual Machine is not running a node.
	stopped = execState(iota)

	// The Virtual Machine is waiting on option selection.
	waitingOnOptionSelection

	//The VirtualMachine has finished delivering content to the
	// client game, and is waiting for Continue to be called.
	waitingForContinue

	// The VirtualMachine is delivering a line, options, or a
	// commmand to the client game.
	deliveringContent

	// The VirtualMachine is in the middle of executing code.
	running
)

// VirtualMachine implements the Yarn Spinner virtual machine.
type VirtualMachine struct {
	stateMu   sync.RWMutex
	execState execState
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

	// Find all lines in the node and pass them to PrepareForLines.
	var ids []string
	for _, inst := range node.Instructions {
		switch inst.Opcode {
		case yarnpb.Instruction_RUN_LINE, yarnpb.Instruction_ADD_OPTION:
			ids = append(ids, inst.Operands[0].GetStringValue())
		}
	}
	vm.Handler.PrepareForLines(ids)
	return nil
}

// SetSelectedOption sets the option selected by the player. Call this once the
// player has chosen an option. The machine will be returned to Suspended state.
func (vm *VirtualMachine) SetSelectedOption(index int) error {
	vm.stateMu.Lock()
	defer vm.stateMu.Unlock()
	if vm.execState != waitingOnOptionSelection {
		return fmt.Errorf("not waiting for an option selection [m.execState = %d]", vm.execState)
	}
	if optslen := len(vm.state.options); index < 0 || index >= optslen {
		return fmt.Errorf("selected option %d out of bounds [0, %d)", index, optslen)
	}
	vm.state.push(vm.state.options[index].DestinationNode)
	vm.state.options = vm.state.options[:0]
	vm.execState = waitingForContinue
	return nil
}

// Stop stops the virtual machine.
func (vm *VirtualMachine) Stop() {
	vm.stateMu.Lock()
	vm.execState = stopped
	vm.stateMu.Unlock()
}

// Continue continues executing the VM.
func (vm *VirtualMachine) Continue() error {
	if vm.state.node == nil {
		return ErrNoNodeSelected
	}
	if vm.execState == waitingOnOptionSelection {
		return ErrWaitingOnOptionSelection
	}
	if vm.Handler == nil {
		return ErrNilDialogueHandler
	}
	if vm.Vars == nil {
		return ErrNilVariableStorage
	}
	if vm.execState == deliveringContent {
		vm.execState = running
		return nil
	}
	vm.execState = running
	for vm.execState == running {
		if err := vm.execute(vm.state.node.Instructions[vm.state.pc]); err != nil {
			return err
		}
		vm.state.pc++
		if proglen := len(vm.state.node.Instructions); vm.state.pc >= proglen {
			vm.Handler.NodeComplete(vm.state.node.Name)
			vm.execState = stopped
			vm.Handler.DialogueComplete()
		}
	}
	return nil
}

func (vm *VirtualMachine) execute(inst *yarnpb.Instruction) error {
	switch inst.Opcode {

	case yarnpb.Instruction_JUMP_TO:
		// Jumps to a named position in the node.
		// opA = string: label name
		k := inst.Operands[0].GetStringValue()
		pc, ok := vm.state.node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.state.pc = int(pc) - 1

	case yarnpb.Instruction_JUMP:
		// Peeks a string from stack, and jumps to that named position in
		// the node.
		// No operands.
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
		// Delivers a string ID to the client.
		// opA = string: string ID
		line := Line{
			ID: inst.Operands[0].GetStringValue(),
		}
		if len(inst.Operands) > 1 {
			// Second operand gives number of values on stack to include as
			// substitutions.
			// NB: the bytecode only has floats, no ints.
			n, err := operandToInt(inst.Operands[1])
			if err != nil {
				return err
			}
			ss, err := vm.state.popNStrings(n)
			if err != nil {
				return err
			}
			line.Substitutions = ss
		}
		vm.execState = deliveringContent
		vm.Handler.Line(line)
		if vm.execState == deliveringContent {
			vm.execState = waitingForContinue
		}

	case yarnpb.Instruction_RUN_COMMAND:
		// Delivers a command to the client.
		// opA = string: command text
		cmd := inst.Operands[0].GetStringValue()
		if len(inst.Operands) > 1 {
			// Second operand gives number of values on stack to interpolate
			// into the command as substitutions.
			// NB: the bytecode only has floats, no ints.
			n, err := operandToInt(inst.Operands[1])
			if err != nil {
				return err
			}
			ss, err := vm.state.popNStrings(n)
			if err != nil {
				return err
			}
			for i, s := range ss {
				cmd = strings.Replace(cmd, fmt.Sprintf("{%d}", i), s, -1)
			}
		}
		vm.execState = deliveringContent
		vm.Handler.Command(cmd)
		if vm.execState == deliveringContent {
			vm.execState = waitingForContinue
		}

	case yarnpb.Instruction_ADD_OPTION:
		// Adds an entry to the option list (see ShowOptions).
		// - opA = string: string ID for option to add
		// - opB = string: destination to go to if this option is selected
		// - opC = number: number of expressions on the stack to insert
		//   into the line
		// - opD = bool: whether the option has a condition on it (in which
		//   case a value should be popped off the stack and used to signal
		//   the game that the option should be not available)
		line := Line{
			ID: inst.Operands[0].GetStringValue(),
		}
		if len(inst.Operands) > 2 {
			n, err := operandToInt(inst.Operands[2])
			if err != nil {
				return err
			}
			ss, err := vm.state.popNStrings(n)
			if err != nil {
				return err
			}
			line.Substitutions = ss
		}
		avail := true
		if len(inst.Operands) > 3 && inst.Operands[3].GetBoolValue() {
			// Condition must be on the stack as a bool.
			cp, err := vm.state.popBool()
			if err != nil {
				return err
			}
			avail = cp
		}
		vm.state.options = append(vm.state.options, Option{
			ID:              len(vm.state.options),
			Line:            line,
			DestinationNode: inst.Operands[1].GetStringValue(),
			IsAvailable:     avail,
		})

	case yarnpb.Instruction_SHOW_OPTIONS:
		// Presents the current list of options to the client, then clears
		// the list. The most recently selected option will be on the top
		// of the stack when execution resumes.
		// No operands.
		if len(vm.state.options) == 0 {
			// NOTE: jon implements this as a machine stop instead of an exception
			vm.execState = stopped
			vm.Handler.DialogueComplete()
			return ErrNoOptions
		}
		vm.execState = waitingOnOptionSelection
		vm.Handler.Options(vm.state.options)
		if vm.execState == waitingForContinue {
			// The handler called SetSelectedOption!
			vm.execState = running
		}

	case yarnpb.Instruction_PUSH_STRING:
		// Pushes a string onto the stack.
		// opA = string: the string to push to the stack.
		vm.state.push(inst.Operands[0].GetStringValue())

	case yarnpb.Instruction_PUSH_FLOAT:
		// Pushes a floating point number onto the stack.
		// opA = float: number to push to stack
		vm.state.push(inst.Operands[0].GetFloatValue())

	case yarnpb.Instruction_PUSH_BOOL:
		// Pushes a boolean onto the stack.
		// opA = bool: the bool to push to stack
		vm.state.push(inst.Operands[0].GetBoolValue())

	case yarnpb.Instruction_PUSH_NULL:
		// Pushes a null value onto the stack.
		// No operands.
		vm.state.push(nil)

	case yarnpb.Instruction_JUMP_IF_FALSE:
		// Jumps to the named position in the the node, if the top of the
		// stack is not null, zero or false.
		// opA = string: label name
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
		// Discards top of stack.
		// No operands.
		if _, err := vm.state.pop(); err != nil {
			return err
		}

	case yarnpb.Instruction_CALL_FUNC:
		// Calls a function in the client. Pops as many arguments as the
		// client indicates the function receives, and the result (if any)
		// is pushed to the stack.
		// opA = string: name of the function

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
		// Pushes the contents of a variable onto the stack.
		// opA = name of variable
		k := inst.Operands[0].GetStringValue()
		v, ok := vm.Vars.GetValue(k)
		if ok {
			vm.state.push(v)
			return nil
		}
		// Is it provided as an initial value?
		w, ok := vm.Program.InitialValues[k]
		if !ok {
			return fmt.Errorf("no variable called %q", k)
		}
		switch x := w.Value.(type) {
		case *yarnpb.Operand_BoolValue:
			vm.state.push(x.BoolValue)
		case *yarnpb.Operand_FloatValue:
			vm.state.push(x.FloatValue)
		case *yarnpb.Operand_StringValue:
			vm.state.push(x.StringValue)
		}

	case yarnpb.Instruction_STORE_VARIABLE:
		// Stores the contents of the top of the stack in the named
		// variable.
		// opA = name of variable
		k := inst.Operands[0].GetStringValue()
		v, err := vm.state.peek()
		if err != nil {
			return err
		}
		vm.Vars.SetValue(k, v)

	case yarnpb.Instruction_STOP:
		// Stops execution of the program.
		// No operands.
		vm.Handler.NodeComplete(vm.state.node.Name)
		vm.Handler.DialogueComplete()
		vm.execState = stopped

	case yarnpb.Instruction_RUN_NODE:
		// Pops a string off the top of the stack, and runs the node with
		// that name.
		// No operands.
		node, err := vm.state.popString()
		if err != nil {
			return err
		}
		vm.Handler.NodeComplete(vm.state.node.Name)
		if err := vm.SetNode(node); err != nil {
			return err
		}
		vm.state.pc--

	default:
		return fmt.Errorf("invalid opcode %v", inst.Opcode)
	}
	return nil
}

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

func (s *state) popBool() (bool, error) {
	x, err := s.pop()
	if err != nil {
		return false, err
	}
	b, ok := x.(bool)
	if !ok {
		return false, fmt.Errorf("wrong type popped from stack [%T != bool]", x)
	}
	return b, nil
}

func (s *state) popString() (string, error) {
	x, err := s.pop()
	if err != nil {
		return "", err
	}
	t, ok := x.(string)
	if !ok {
		return "", fmt.Errorf("wrong type popped from stack [%T != string]", x)
	}
	return t, nil
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
