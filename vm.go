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

// Package yarn implements the Yarn Spinner virtual machine and dialogue system.
// For the original implementation, see https://yarnspinner.dev and
// https://github.com/YarnSpinnerTool/YarnSpinner.
package yarn // import "github.com/DrJosh9000/yarn"

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
)

// Various sentinel errors returned by the virtual machine.
const (
	// ErrNilDialogueHandler indicates that Handler hasn't been set.
	ErrNilDialogueHandler = virtualMachineError("nil dialogue handler")

	// ErrNilVariableStorage indicates that Vars hasn't been set.
	ErrNilVariableStorage = virtualMachineError("nil variable storage")

	// ErrMissingProgram indicates that Program hasn't been set.
	ErrMissingProgram = virtualMachineError("missing or empty program")

	// ErrNoOptions indicates the program is invalid - it tried to show options
	// but none had been added.
	ErrNoOptions = virtualMachineError("no options were added")

	// ErrStackUnderflow indicates the program tried to pop or peek when the
	// stack was empty.
	ErrStackUnderflow = virtualMachineError("stack underflow")

	// ErrWrongType indicates the program needed a stack value, operand, or
	// function of one type, but got something else instead.
	ErrWrongType = virtualMachineError("wrong type")

	// ErrNotConvertible indicates the program tried to convert a stack value
	// or operand to a different type, but it was not convertible to that type.
	ErrNotConvertible = virtualMachineError("not convertible")

	// ErrNodeNotFound is returned where Run or SetNode is passed the name of a
	// node that is not in the program.
	ErrNodeNotFound = virtualMachineError("node not found")

	// ErrLabelNotFound indicates the program tries to jump to a label that
	// isn't in the label table for the current node.
	ErrLabelNotFound = virtualMachineError("label not found")

	// ErrNilOperand indicates the a malformed program containing an instruction
	// that requires a usable operand but the operand was nil.
	ErrNilOperand = virtualMachineError("nil operand")

	// ErrFunctionNotFound indicates the program tried to call a function but
	// that function is not in the FuncMap.
	ErrFunctionNotFound = virtualMachineError("function not found")

	// ErrFunctionArgMismatch indicates the program tried to call a function but
	// had the wrong number or types of args to pass to it.
	ErrFunctionArgMismatch = virtualMachineError("arg mismatch")
)

// Stop stops the virtual machine without error. It is used by the STOP
// instruction, but can also be returned by your handler to stop the VM in the
// same way. However a stop happens, NodeComplete and DialogueComplete are still
// called.
const Stop = virtualMachineError("stop")

var (
	// Used to typecheck the second return arg of functions in FuncMap.
	errorType = reflect.TypeOf((*error)(nil)).Elem()

	// Used to switch on function argument type to pick a conversion.
	boolType    = reflect.TypeOf(true)
	float32Type = reflect.TypeOf(float32(0))
	float64Type = reflect.TypeOf(float64(0))
	intType     = reflect.TypeOf(int(0))
	stringType  = reflect.TypeOf("")
)

// Used to implement the sentinel errors as consts instead of vars.
type virtualMachineError string

func (e virtualMachineError) Error() string { return string(e) }

// VirtualMachine implements the Yarn Spinner virtual machine.
type VirtualMachine struct {
	// Program is the program to execute.
	Program *yarnpb.Program

	// Handler receives content (lines, options, etc) and other events.
	Handler DialogueHandler

	// Vars stores variables used and provided by the dialogue.
	Vars VariableStorage

	// FuncMap is used to provide user-defined functions.
	FuncMap FuncMap

	// TraceLogf, if not nil, is called before each instruction to log the
	// current stack, options, and the instruction about to be executed.
	TraceLogf func(string, ...interface{})

	state state
}

// SetNode sets the VM to begin a node. If a node is already selected,
// NodeComplete will be called for that node. Then NodeStart and PrepareForLines
// will be called (for the newly selected node). Passing the current node is one
// way to reset to the start of the node.
func (vm *VirtualMachine) SetNode(name string) error {
	if vm.Program == nil {
		return ErrMissingProgram
	}
	node, found := vm.Program.Nodes[name]
	if !found {
		return ErrNodeNotFound
	}

	// Designate the current node complete.
	if vm.state.node != nil {
		if err := vm.Handler.NodeComplete(vm.state.node.Name); err != nil {
			return fmt.Errorf("handler.NodeComplete: %w", err)
		}
	}

	// Reset the state and start at this node.
	vm.state = state{
		node: node,
	}

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

// Run executes the program, starting at a particular node.
func (vm *VirtualMachine) Run(startNode string) error {
	if vm.Handler == nil {
		return ErrNilDialogueHandler
	}
	if vm.Vars == nil {
		return ErrNilVariableStorage
	}
	// Provide default funcs, merge provided funcmap to allow overrides.
	vm.FuncMap = vm.defaultFuncMap().merge(vm.FuncMap)
	// Set start node
	if err := vm.SetNode(startNode); err != nil {
		return err
	}
	// Run! This is the instruction loop.
instructionLoop:
	for vm.state.pc < len(vm.state.node.Instructions) {
		inst := vm.state.node.Instructions[vm.state.pc]
		if vm.TraceLogf != nil {
			vm.TraceLogf("stack %v; options %v", vm.state.stack, vm.state.options)
			vm.TraceLogf("% 15s %06d %s", vm.state.node.Name, vm.state.pc, FormatInstruction(inst))
		}
		switch err := vm.execute(inst); {
		case errors.Is(err, Stop): // machine has stopped
			break instructionLoop
		case err != nil: // something else
			return fmt.Errorf("%s %06d %s: %w", vm.state.node.Name, vm.state.pc, FormatInstruction(inst), err)
		}
	}
	if err := vm.Handler.NodeComplete(vm.state.node.Name); err != nil && !errors.Is(err, Stop) {
		return fmt.Errorf("handler.NodeComplete: %w", err)
	}
	if err := vm.Handler.DialogueComplete(); err != nil && !errors.Is(err, Stop) {
		return fmt.Errorf("handler.DialogueComplete: %w", err)
	}
	return nil
}

// defaultFuncMap provides the default func map for this VM along with all built-in functions.
func (vm *VirtualMachine) defaultFuncMap() FuncMap {
	result := defaultFuncMap()
	result.merge(map[string]interface{}{
		"visited": func(nodeName string) bool {
			_, ok := vm.Vars.GetValue(fmt.Sprintf("$Yarn.Internal.Visiting.%s", nodeName))
			return ok
		},
		"visited_count": func(nodeName string) int {
			if count, ok := vm.Vars.GetValue(fmt.Sprintf("$Yarn.Internal.Visiting.%s", nodeName)); ok {
				return int(count.(float32))
			}
			return 0
		},
	})
	return result
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

func (vm *VirtualMachine) execJumpTo(operands []*yarnpb.Operand) error {
	// Jumps to a named position in the node.
	// opA = string: label name
	k := operands[0].GetStringValue()
	pc, ok := vm.state.node.Labels[k]
	if !ok {
		return fmt.Errorf("%q %w in node %q", k, ErrLabelNotFound, vm.state.node.Name)
	}
	vm.state.pc = int(pc)
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
		return fmt.Errorf("%q %w in node %q", k, ErrLabelNotFound, vm.state.node.Name)
	}
	vm.state.pc = int(pc)
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
	if err := vm.Handler.Line(line); err != nil {
		return fmt.Errorf("handler.Line: %w", err)
	}
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execRunCommand(operands []*yarnpb.Operand) error {
	// Delivers a command to the client.
	// opA = string: command text
	cmd := operands[0].GetStringValue()
	if len(operands) > 1 {
		// Second operand gives number of values on stack to interpolate
		// into the command as substitutions.
		n, err := operandToInt(operands[1])
		if err != nil {
			return fmt.Errorf("operandToInt(opB): %w", err)
		}
		ss, err := vm.state.popNStrings(n)
		if err != nil {
			return fmt.Errorf("popNStrings(%d): %w", n, err)
		}
		for i, s := range ss {
			cmd = strings.ReplaceAll(cmd, fmt.Sprintf("{%d}", i), s)
		}
	}
	// To allow the command to overwrite PC, increment it first
	vm.state.pc++
	if err := vm.Handler.Command(cmd); err != nil {
		return fmt.Errorf("handler.Command: %w", err)
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
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execShowOptions([]*yarnpb.Operand) error {
	// Presents the current list of options to the client, then clears
	// the list. The most recently selected option will be on the top
	// of the stack when execution resumes.
	// No operands.
	if len(vm.state.options) == 0 {
		// NOTE: jon implements this as a machine stop instead of an exception
		vm.Handler.DialogueComplete()
		return ErrNoOptions
	}
	index, err := vm.Handler.Options(vm.state.options)
	if err != nil {
		return fmt.Errorf("handler.Options: %w", err)
	}
	if optslen := len(vm.state.options); index < 0 || index >= optslen {
		return fmt.Errorf("selected option %d out of bounds [0, %d)", index, optslen)
	}
	vm.state.push(vm.state.options[index].DestinationNode)
	vm.state.options = nil
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execPushString(operands []*yarnpb.Operand) error {
	// Pushes a string onto the stack.
	// opA = string: the string to push to the stack.
	vm.state.push(operands[0].GetStringValue())
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execPushFloat(operands []*yarnpb.Operand) error {
	// Pushes a floating point number onto the stack.
	// opA = float: number to push to stack
	vm.state.push(operands[0].GetFloatValue())
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execPushBool(operands []*yarnpb.Operand) error {
	// Pushes a boolean onto the stack.
	// opA = bool: the bool to push to stack
	vm.state.push(operands[0].GetBoolValue())
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execPushNull([]*yarnpb.Operand) error {
	// Pushes a null value onto the stack.
	// No operands.
	vm.state.push(nil)
	vm.state.pc++
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
	b, err := ConvertToBool(x)
	if err != nil {
		return fmt.Errorf("convertToBool: %w", err)
	}
	if b {
		// Value is true, so don't jump
		vm.state.pc++
		return nil
	}
	k := operands[0].GetStringValue()
	pc, ok := vm.state.node.Labels[k]
	if !ok {
		return fmt.Errorf("%q %w in node %q", k, ErrLabelNotFound, vm.state.node.Name)
	}
	vm.state.pc = int(pc)
	return nil
}

func (vm *VirtualMachine) execPop([]*yarnpb.Operand) error {
	// Discards top of stack.
	// No operands.
	if _, err := vm.state.pop(); err != nil {
		return fmt.Errorf("pop: %w", err)
	}
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execCallFunc(operands []*yarnpb.Operand) error {
	// Calls a function in the client. Pops as many arguments as the
	// client indicates the function receives, and the result (if any)
	// is pushed to the stack.
	// opA = string: name of the function

	// TODO: typecheck FuncMap during preprocessing
	// TODO: a lot of this is very forgiving...
	funcname := operands[0].GetStringValue()
	function, found := vm.FuncMap[funcname]
	if !found {
		return fmt.Errorf("%q %w", funcname, ErrFunctionNotFound)
	}
	functype := reflect.TypeOf(function)
	if functype.Kind() != reflect.Func {
		return fmt.Errorf("%w: function for %q not actually a function [type %T]", ErrWrongType, funcname, function)
	}
	// Compiler puts number of args on top of stack
	gotx, err := vm.state.pop()
	if err != nil {
		return fmt.Errorf("pop: %w", err)
	}
	gotArgc, err := ConvertToInt(gotx)
	if err != nil {
		return fmt.Errorf("convertToInt: %w", err)
	}
	// Check that we have enough args to call the func
	switch wantArgc := functype.NumIn(); {
	case functype.IsVariadic() && gotArgc < wantArgc-1:
		// The last (variadic) arg is free to be empty. But we don't even have
		// that many...
		return fmt.Errorf("%w: insufficient args provided by program [got %d < want %d]", ErrFunctionArgMismatch, gotArgc, wantArgc-1)
	case !functype.IsVariadic() && gotArgc != wantArgc:
		// Gotta match exactly.
		return fmt.Errorf("%w: wrong number of args provided by program [got %d, want %d]", ErrFunctionArgMismatch, gotArgc, wantArgc)
	}

	// Also check that function returns between 0 and 2 args; if there are two,
	// the second is only allowed to be type error.
	switch functype.NumOut() {
	case 0, 1:
		// ok
	case 2:
		if functype.Out(1) != errorType {
			return fmt.Errorf("%w: wrong type for second return arg [got %s, want error]", ErrFunctionArgMismatch, functype.Out(1).Name())
		}
	default:
		return fmt.Errorf("%w: unsupported number of return args [got %d, want in {0,1,2}]", ErrFunctionArgMismatch, functype.NumOut())
	}

	arg := gotArgc
	params := make([]reflect.Value, arg)
	for arg > 0 {
		arg--
		param, err := vm.state.pop()
		if err != nil {
			return fmt.Errorf("pop: %w", err)
		}
		var argtype reflect.Type
		if functype.IsVariadic() && arg >= functype.NumIn()-1 {
			// last arg is reported by reflect as a slice type
			argtype = functype.In(functype.NumIn() - 1).Elem()
		} else {
			// Not variadic, or arg comes before the final variadic arg
			argtype = functype.In(arg)
		}
		if param == nil {
			// substitute nil param with a zero value, because nil Value can't
			// be used.
			params[arg] = reflect.Zero(argtype)
			continue
		}

		// typecheck paramtype against argtype
		if paramtype := reflect.TypeOf(param); !paramtype.AssignableTo(argtype) {
			// attempt conversion to the type expected by the function
			switch argtype {
			// no case for interface{} because everything is assignable to interface{}
			case stringType:
				param = ConvertToString(param)
			case float32Type:
				p, err := ConvertToFloat32(param)
				if err != nil {
					return err
				}
				param = p
			case float64Type:
				p, err := ConvertToFloat64(param)
				if err != nil {
					return err
				}
				param = p
			case intType:
				p, err := ConvertToInt(param)
				if err != nil {
					return err
				}
				param = p
			case boolType:
				p, err := ConvertToBool(param)
				if err != nil {
					return err
				}
				param = p
			default:
				return fmt.Errorf("%w: value %v [type %T] not assignable or convertible to argument %d of %q [type %v]", ErrFunctionArgMismatch, param, param, arg, funcname, argtype)
			}
		}
		params[arg] = reflect.ValueOf(param)
	}

	// Because the func could overwrite PC, increment first
	vm.state.pc++

	result := reflect.ValueOf(function).Call(params)

	// Error?
	if last := functype.NumOut() - 1; last >= 0 && functype.Out(last) == errorType && !result[last].IsNil() {
		return result[last].Interface().(error)
	}

	// A return value?
	if len(result) > 0 && functype.Out(0) != errorType {
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
		vm.state.pc++
		return nil
	}
	// Is it provided as an initial value?
	w, ok := vm.Program.InitialValues[k]
	if !ok {
		// Neither a known nor initial value.
		// Yarn Spinner pushes null.
		vm.state.push(nil)
		vm.state.pc++
		return nil
	}
	switch x := w.Value.(type) {
	case *yarnpb.Operand_BoolValue:
		vm.state.push(x.BoolValue)
	case *yarnpb.Operand_FloatValue:
		vm.state.push(x.FloatValue)
	case *yarnpb.Operand_StringValue:
		vm.state.push(x.StringValue)
	}
	vm.state.pc++
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
	vm.state.pc++
	return nil
}

func (vm *VirtualMachine) execStop([]*yarnpb.Operand) error {
	// Stops execution of the program.
	// No operands.
	return Stop
}

func (vm *VirtualMachine) execRunNode([]*yarnpb.Operand) error {
	// Pops a string off the top of the stack, and runs the node with
	// that name.
	// No operands.
	node, err := vm.state.popString()
	if err != nil {
		return fmt.Errorf("popString: %w", err)
	}
	if err := vm.SetNode(node); err != nil {
		return fmt.Errorf("SetNode: %w", err)
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
		ss[i] = ConvertToString(x)
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
