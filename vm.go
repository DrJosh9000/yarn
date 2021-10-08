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
	"log"
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

	// ErrStackUnderflow indicates the program tried to pop or peek when the
	// stack was empty.
	ErrStackUnderflow = errors.New("stack underflow")

	// ErrWrongType indicates the program needed a value of one type, but got
	// something else instead.
	ErrWrongType = errors.New("wrong type")
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
	// command to the client game.
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

	// Handlers / callbacks
	Handler DialogueHandler
	Vars    VariableStorage
	FuncMap map[string]interface{} // works a bit like text/template.FuncMap

	// Debugging options
	TraceLog bool
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
	if err := vm.Handler.NodeStart(name); err != nil {
		return fmt.Errorf("handler.NodeStart: %w", err)
	}

	// Find all lines in the node and pass them to PrepareForLines.
	var ids []string
	for _, inst := range node.Instructions {
		switch inst.Opcode {
		case yarnpb.Instruction_RUN_LINE, yarnpb.Instruction_ADD_OPTION:
			ids = append(ids, inst.Operands[0].GetStringValue())
		}
	}
	if err := vm.Handler.PrepareForLines(ids); err != nil {
		return fmt.Errorf("handler.PrepareForLines: %w", err)
	}
	return nil
}

// SetSelectedOption sets the option selected by the player. Call this once the
// player has chosen an option.
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
	vm.state.options = nil
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
		pc := vm.state.pc
		inst := vm.state.node.Instructions[pc]
		if vm.TraceLog {
			log.Printf("stack %q; options %v", vm.state.stack, vm.state.options)
			log.Printf("% 15s %06d %s", vm.state.node.Name, pc, FormatInstruction(inst))
		}
		if err := vm.execute(inst); err != nil {
			return fmt.Errorf("executing %v at %d: %w", inst, pc, err)
		}
		vm.state.pc++
		if proglen := len(vm.state.node.Instructions); vm.state.pc >= proglen {
			if err := vm.Handler.NodeComplete(vm.state.node.Name); err != nil {
				return fmt.Errorf("handler.NodeComplete: %w", err)
			}
			vm.execState = stopped
			if err := vm.Handler.DialogueComplete(); err != nil {
				return fmt.Errorf("handler.DialogueComplete: %w", err)
			}
		}
	}
	return nil
}

func (vm *VirtualMachine) execJumpTo(operands []*yarnpb.Operand) error {
	// Jumps to a named position in the node.
	// opA = string: label name
	k := operands[0].GetStringValue()
	pc, ok := vm.state.node.Labels[k]
	if !ok {
		return fmt.Errorf("unknown label %q in node %q", k, vm.state.node.Name)
	}
	vm.state.pc = int(pc) - 1
	return nil
}

func (vm *VirtualMachine) execJump([]*yarnpb.Operand) error {
	// Peeks a string from stack, and jumps to that named position in
	// the node.
	// No operands.
	k, err := vm.state.peekString()
	if err != nil {
		return err
	}
	pc, ok := vm.state.node.Labels[k]
	if !ok {
		return fmt.Errorf("unknown label %q in node %q", k, vm.state.node.Name)
	}
	vm.state.pc = int(pc) - 1
	return nil
}

func (vm *VirtualMachine) execRunLine(operands []*yarnpb.Operand) error {
	// Delivers a string ID to the client.
	// opA = string: string ID
	line := Line{
		ID: operands[0].GetStringValue(),
	}
	if len(operands) > 1 {
		// Second operand gives number of values on stack to include as
		// substitutions.
		// NB: the bytecode only has floats, no ints.
		n, err := operandToInt(operands[1])
		if err != nil {
			return fmt.Errorf("operandToInt(opB): %w", err)
		}
		ss, err := vm.state.popNStrings(n)
		if err != nil {
			return fmt.Errorf("popNStrings(%d): %w", n, err)
		}
		line.Substitutions = ss
	}
	vm.execState = deliveringContent
	if err := vm.Handler.Line(line); err != nil {
		return fmt.Errorf("handler.Line: %w", err)
	}
	if vm.execState == deliveringContent {
		vm.execState = waitingForContinue
	}
	return nil
}

func (vm *VirtualMachine) execRunCommand(operands []*yarnpb.Operand) error {
	// Delivers a command to the client.
	// opA = string: command text
	cmd := operands[0].GetStringValue()
	if len(operands) > 1 {
		// Second operand gives number of values on stack to interpolate
		// into the command as substitutions.
		// NB: the bytecode only has floats, no ints.
		n, err := operandToInt(operands[1])
		if err != nil {
			return fmt.Errorf("operandToInt(opB): %w", err)
		}
		ss, err := vm.state.popNStrings(n)
		if err != nil {
			return fmt.Errorf("popNStrings(%d): %w", n, err)
		}
		for i, s := range ss {
			cmd = strings.Replace(cmd, fmt.Sprintf("{%d}", i), s, -1)
		}
	}
	vm.execState = deliveringContent
	if err := vm.Handler.Command(cmd); err != nil {
		return fmt.Errorf("handler.Command: %w", err)
	}
	if vm.execState == deliveringContent {
		vm.execState = waitingForContinue
	}
	return nil
}

func (vm *VirtualMachine) execAddOption(operands []*yarnpb.Operand) error {
	// Adds an entry to the option list (see ShowOptions).
	// - opA = string: string ID for option to add
	// - opB = string: destination to go to if this option is selected
	// - opC = number: number of expressions on the stack to insert
	//   into the line
	// - opD = bool: whether the option has a condition on it (in which
	//   case a value should be popped off the stack and used to signal
	//   the game that the option should be not available)
	line := Line{
		ID: operands[0].GetStringValue(),
	}
	if len(operands) > 2 {
		n, err := operandToInt(operands[2])
		if err != nil {
			return fmt.Errorf("operandToInt(opC): %w", err)
		}
		ss, err := vm.state.popNStrings(n)
		if err != nil {
			return fmt.Errorf("popNStrings(%d): %w", n, err)
		}
		line.Substitutions = ss
	}
	avail := true
	if len(operands) > 3 && operands[3].GetBoolValue() {
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
		DestinationNode: operands[1].GetStringValue(),
		IsAvailable:     avail,
	})
	return nil
}

func (vm *VirtualMachine) execShowOptions([]*yarnpb.Operand) error {
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
	if err := vm.Handler.Options(vm.state.options); err != nil {
		return fmt.Errorf("handler.Options: %w", err)
	}
	if vm.execState == waitingForContinue {
		// The handler called SetSelectedOption!
		vm.execState = running
	}
	return nil
}

func (vm *VirtualMachine) execPushString(operands []*yarnpb.Operand) error {
	// Pushes a string onto the stack.
	// opA = string: the string to push to the stack.
	vm.state.push(operands[0].GetStringValue())
	return nil
}

func (vm *VirtualMachine) execPushFloat(operands []*yarnpb.Operand) error {
	// Pushes a floating point number onto the stack.
	// opA = float: number to push to stack
	vm.state.push(operands[0].GetFloatValue())
	return nil
}

func (vm *VirtualMachine) execPushBool(operands []*yarnpb.Operand) error {
	// Pushes a boolean onto the stack.
	// opA = bool: the bool to push to stack
	vm.state.push(operands[0].GetBoolValue())
	return nil
}

func (vm *VirtualMachine) execPushNull([]*yarnpb.Operand) error {
	// Pushes a null value onto the stack.
	// No operands.
	vm.state.push(nil)
	return nil
}

func (vm *VirtualMachine) execJumpIfFalse(operands []*yarnpb.Operand) error {
	// Jumps to the named position in the the node, if the top of the
	// stack is not null, zero or false.
	// opA = string: label name
	x, err := vm.state.peek()
	if err != nil {
		return fmt.Errorf("peek: %w", err)
	}
	b, err := convertToBool(x)
	if err != nil {
		return fmt.Errorf("convertToBool: %w", err)
	}
	if b {
		// Value is true, so don't jump
		return nil
	}
	k := operands[0].GetStringValue()
	pc, ok := vm.state.node.Labels[k]
	if !ok {
		return fmt.Errorf("unknown label %q", k)
	}
	vm.state.pc = int(pc) - 1
	return nil
}

func (vm *VirtualMachine) execPop([]*yarnpb.Operand) error {
	// Discards top of stack.
	// No operands.
	if _, err := vm.state.pop(); err != nil {
		return fmt.Errorf("pop: %w", err)
	}
	return nil
}

func (vm *VirtualMachine) execCallFunc(operands []*yarnpb.Operand) error {
	// Calls a function in the client. Pops as many arguments as the
	// client indicates the function receives, and the result (if any)
	// is pushed to the stack.
	// opA = string: name of the function

	// TODO: typecheck FuncMap during preprocessing
	// TODO: a lot of this is very forgiving...
	k := operands[0].GetStringValue()
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
		return fmt.Errorf("pop: %w", err)
	}
	got, err := convertToInt(gotx)
	if err != nil {
		return fmt.Errorf("convertToInt: %w", err)
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
			return fmt.Errorf("pop: %w", err)
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
	return nil
}

func (vm *VirtualMachine) execPushVariable(operands []*yarnpb.Operand) error {
	// Pushes the contents of a variable onto the stack.
	// opA = name of variable
	k := operands[0].GetStringValue()
	v, ok := vm.Vars.GetValue(k)
	if ok {
		vm.state.push(v)
		return nil
	}
	// Is it provided as an initial value?
	w, ok := vm.Program.InitialValues[k]
	if !ok {
		return fmt.Errorf("no variable %q in storage or initial values", k)
	}
	switch x := w.Value.(type) {
	case *yarnpb.Operand_BoolValue:
		vm.state.push(x.BoolValue)
	case *yarnpb.Operand_FloatValue:
		vm.state.push(x.FloatValue)
	case *yarnpb.Operand_StringValue:
		vm.state.push(x.StringValue)
	}
	return nil
}

func (vm *VirtualMachine) execStoreVariable(operands []*yarnpb.Operand) error {
	// Stores the contents of the top of the stack in the named
	// variable.
	// opA = name of variable
	k := operands[0].GetStringValue()
	v, err := vm.state.peek()
	if err != nil {
		return fmt.Errorf("peek: %w", err)
	}
	vm.Vars.SetValue(k, v)
	return nil
}

func (vm *VirtualMachine) execStop([]*yarnpb.Operand) error {
	// Stops execution of the program.
	// No operands.
	if err := vm.Handler.NodeComplete(vm.state.node.Name); err != nil {
		return fmt.Errorf("handler.NodeComplete: %w", err)
	}
	if err := vm.Handler.DialogueComplete(); err != nil {
		return fmt.Errorf("handler.DialogueComplete: %w", err)
	}
	vm.execState = stopped
	return nil
}

func (vm *VirtualMachine) execRunNode([]*yarnpb.Operand) error {
	// Pops a string off the top of the stack, and runs the node with
	// that name.
	// No operands.
	node, err := vm.state.popString()
	if err != nil {
		return fmt.Errorf("popString: %w", err)
	}
	if err := vm.Handler.NodeComplete(vm.state.node.Name); err != nil {
		return fmt.Errorf("handler.NodeComplete: %w", err)
	}
	if err := vm.SetNode(node); err != nil {
		return fmt.Errorf("SetNode: %w", err)
	}
	vm.state.pc--
	return nil
}

var dispatchTable = []func(*VirtualMachine, []*yarnpb.Operand) error{
	yarnpb.Instruction_JUMP_TO:        (*VirtualMachine).execJumpTo,
	yarnpb.Instruction_JUMP:           (*VirtualMachine).execJump,
	yarnpb.Instruction_RUN_LINE:       (*VirtualMachine).execRunLine,
	yarnpb.Instruction_RUN_COMMAND:    (*VirtualMachine).execRunCommand,
	yarnpb.Instruction_ADD_OPTION:     (*VirtualMachine).execAddOption,
	yarnpb.Instruction_SHOW_OPTIONS:   (*VirtualMachine).execShowOptions,
	yarnpb.Instruction_PUSH_STRING:    (*VirtualMachine).execPushString,
	yarnpb.Instruction_PUSH_FLOAT:     (*VirtualMachine).execPushFloat,
	yarnpb.Instruction_PUSH_BOOL:      (*VirtualMachine).execPushBool,
	yarnpb.Instruction_PUSH_NULL:      (*VirtualMachine).execPushNull,
	yarnpb.Instruction_JUMP_IF_FALSE:  (*VirtualMachine).execJumpIfFalse,
	yarnpb.Instruction_POP:            (*VirtualMachine).execPop,
	yarnpb.Instruction_CALL_FUNC:      (*VirtualMachine).execCallFunc,
	yarnpb.Instruction_PUSH_VARIABLE:  (*VirtualMachine).execPushVariable,
	yarnpb.Instruction_STORE_VARIABLE: (*VirtualMachine).execStoreVariable,
	yarnpb.Instruction_STOP:           (*VirtualMachine).execStop,
	yarnpb.Instruction_RUN_NODE:       (*VirtualMachine).execRunNode,
}

func (vm *VirtualMachine) execute(inst *yarnpb.Instruction) error {
	if inst.Opcode < 0 || int(inst.Opcode) >= len(dispatchTable) {
		return fmt.Errorf("invalid opcode %v", inst.Opcode)
	}
	exec := dispatchTable[inst.Opcode]
	if exec == nil {
		return fmt.Errorf("invalid opcode %v", inst.Opcode)
	}
	return exec(vm, inst.Operands)
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
	// pop = (peek and then chuck out the top)
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
		return false, fmt.Errorf("%w from stack [%T != bool]", ErrWrongType, x)
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
		return "", fmt.Errorf("%w from stack [%T != string]", ErrWrongType, x)
	}
	return t, nil
}

// Reading N strings from the stack is common enough that I made a dedicated
// helper method for it.
func (s *state) popNStrings(n int) ([]string, error) {
	if n < 0 {
		return nil, fmt.Errorf("popping %d items", n)
	}
	if n == 0 {
		return nil, nil
	}
	if n > len(s.stack) {
		return nil, fmt.Errorf("%w [%d > %d]", ErrStackUnderflow, n, len(s.stack))
	}
	rem := len(s.stack) - n
	ss := make([]string, n)
	for i, x := range s.stack[rem:] {
		t, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("%w from stack [%T != string]", ErrWrongType, x)
		}
		ss[i] = t
	}
	s.stack = s.stack[:rem]
	return ss, nil
}

// peek returns the top vaue from the stack only.
func (s *state) peek() (interface{}, error) {
	if len(s.stack) == 0 {
		return nil, ErrStackUnderflow
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
		return "", fmt.Errorf("%w from stack [%T != string]", ErrWrongType, x)
	}
	return t, nil
}
