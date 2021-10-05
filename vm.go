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

	"github.com/DrJosh9000/yarn/bytecode"
)

// BUG: This package hasn't been used or tested yet, and is incomplete.

// ExecState is the highest-level machine state.
type ExecState int

const (
	ExecStateStopped = ExecState(iota)
	ExecStateWaitOnOptionSelection
	ExecStateRunning
)

type option struct{ id, node string }

// VMState models a machine state.
type VMState struct {
	node    string
	pc      int
	stack   []interface{}
	options []option
}

// Push pushes a value onto the state's stack.
func (s *VMState) Push(x interface{}) { s.stack = append(s.stack, x) }

// Pop removes a value from the stack and returns it.
func (s *VMState) Pop() (interface{}, error) {
	x, err := s.Peek()
	if err != nil {
		return nil, err
	}
	s.stack = s.stack[:len(s.stack)-1]
	return x, nil
}

// Peek returns the top vaue from the stack only.
func (s *VMState) Peek() (interface{}, error) {
	if len(s.stack) == 0 {
		return nil, errors.New("stack underflow")
	}
	return s.stack[len(s.stack)-1], nil
}

// Clear resets the stack state.
func (s *VMState) Clear() { s.stack = nil }

// VM implements the virtual machine.
type VM struct {
	execState ExecState
	program   *bytecode.Program
	vmState   *VMState

	Delegate
	Library
	VariableStorage
}

// Stop stops the virtual machine.
func (vm *VM) Stop() { vm.execState = ExecStateStopped }

// RunNext executes the next instruction in the current node.
func (vm *VM) RunNext() error {
	switch vm.execState {
	case ExecStateStopped:
		vm.execState = ExecStateRunning
	case ExecStateWaitOnOptionSelection:
		return errors.New("cannot run, waiting on option selection")
	}
	if vm.Delegate == nil {
		return errors.New("delegate is nil")
	}
	if vm.VariableStorage == nil {
		return errors.New("variable storage is nil")
	}
	node, ok := vm.program.Nodes[vm.vmState.node]
	if !ok {
		return fmt.Errorf("illegal state; unknown node of program %q", vm.vmState.node)
	}
	if vm.vmState.pc < 0 || vm.vmState.pc >= len(node.Instructions) {
		return fmt.Errorf("illegal state; pc %d outside program [0, %d)", vm.vmState.pc, len(node.Instructions))
	}
	ins := node.Instructions[vm.vmState.pc]
	if err := vm.Execute(ins, node); err != nil {
		return err
	}
	vm.vmState.pc++
	if vm.vmState.pc >= len(node.Instructions) {
		vm.execState = ExecStateStopped
	}
	return nil
}

func (vm *VM) optionPicked(i int) error {
	if vm.execState != ExecStateWaitOnOptionSelection {
		return fmt.Errorf("machine is not waiting for an option selection [m.execState = %d]", vm.execState)
	}
	if i < 0 || i >= len(vm.vmState.options) {
		return fmt.Errorf("selected option %d out of bounds [0, %d)", i, len(vm.vmState.options))
	}
	vm.vmState.Push(vm.vmState.options[i].node)
	vm.vmState.options = nil
	vm.execState = ExecStateRunning
	return nil
}

// Execute executes a single instruction.
func (vm *VM) Execute(instruction *bytecode.Instruction, node *bytecode.Node) error {
	switch instruction.Opcode {

	case bytecode.Instruction_JUMP_TO:
		k := instruction.Operands[0].GetStringValue()
		if k == "" {
			return fmt.Errorf("wrong type in opA or empty string [%T]", instruction.Operands[0].Value)
		}
		pc, ok := node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.vmState.pc = int(pc)

	case bytecode.Instruction_JUMP:
		o, err := vm.vmState.Peek()
		if err != nil {
			return err
		}
		k, ok := o.(string)
		if !ok {
			return fmt.Errorf("wrong type of value at top of stack [%T != string]", o)
		}
		pc, ok := node.Labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.vmState.pc = int(pc)

	case bytecode.Instruction_RUN_LINE:
		k := instruction.Operands[0].GetStringValue()
		if k == "" {
			return fmt.Errorf("wrong type in opA or empty string [%T]", instruction.Operands[0].Value)
		}
		l, ok := vm.program.stringTable[k]
		if !ok {
			return fmt.Errorf("no string in string table for key %q", k)
		}
		if err := vm.Line(l); err != nil {
			return err
		}

	case bytecode.Instruction_RUN_COMMAND:
		c := instruction.Operands[0].GetStringValue()
		if c == "" {
			return fmt.Errorf("wrong type in opA or empty string [%T]", instruction.Operands[0].Value)
		}
		if err := vm.Command(c); err != nil {
			return err
		}

	case bytecode.Instruction_ADD_OPTION:
		a, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		b, ok := instruction.opB.(string)
		if !ok {
			return fmt.Errorf("wrong type in opB [%T != string]", instruction.opB)
		}
		vm.vmState.options = append(vm.vmState.options, option{id: a, node: b})

	case bytecode.Instruction_SHOW_OPTIONS:
		switch len(vm.vmState.options) {
		case 0:
			// NOTE: jon implements this as a machine stop instead of an exception
			return errors.New("illegal state, no options to show")
		case 1:
			vm.vmState.Push(vm.vmState.options[0].node)
			vm.vmState.options = nil
			return nil
		}
		// TODO: implement shuffling of options depending on configuration.
		ops := make([]string, 0, len(vm.vmState.options))
		for _, op := range vm.vmState.options {
			s, ok := vm.program.stringTable[op.id]
			if !ok {
				return fmt.Errorf("no string in string table for key %q", op.id)
			}
			ops = append(ops, s)
		}
		vm.execState = ExecStateWaitOnOptionSelection
		if err := vm.Options(ops, vm.optionPicked); err != nil {
			return err
		}

	case bytecode.Instruction_PUSH_STRING:
		x, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		s, ok := vm.program.stringTable[x]
		if !ok {
			return fmt.Errorf("no string in string table for key %q", x)
		}
		vm.vmState.Push(s)

	case bytecode.Instruction_PUSH_FLOAT:
		x, ok := instruction.opA.(float64)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != float64]", instruction.opA)
		}
		vm.vmState.Push(x)

	case bytecode.Instruction_PUSH_BOOL:
		x, ok := instruction.opA.(bool)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != bool]", instruction.opA)
		}
		vm.vmState.Push(x)

	case bytecode.Instruction_PUSH_NULL:
		vm.vmState.Push(nil)

	case bytecode.Instruction_JUMP_IF_FALSE:
		x, err := vm.vmState.Peek()
		if err != nil {
			return err
		}
		b, err := convertToBool(x)
		if err != nil {
			return err
		}
		if b {
			return nil
		}
		k, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		pc, ok := node.labels[k]
		if !ok {
			return fmt.Errorf("unknown label %q", k)
		}
		vm.vmState.pc = pc

	case bytecode.Instruction_POP:
		vm.vmState.Pop()

	case bytecode.Instruction_CALL_FUNC:
		k, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		f, err := vm.Library.Function(k)
		if err != nil {
			return err
		}
		c := f.ParamCount()
		if c == -1 {
			// Variadic, so param count is on stack.
			x, err := vm.vmState.Pop()
			if err != nil {
				return err
			}
			y, ok := x.(int)
			if !ok {
				return fmt.Errorf("wrong type popped from stack [%T != int]", x)
			}
			c = y
		}
		params := make([]interface{}, c)
		for c >= 0 {
			c--
			p, err := vm.vmState.Pop()
			if err != nil {
				return err
			}
			params[c] = p
		}
		r, err := f.Invoke(params...)
		if err != nil {
			return err
		}
		if f.Returns() {
			vm.vmState.Push(r)
		}

	case bytecode.Instruction_PUSH_VARIABLE:
		k, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		v, ok := vm.VariableStorage.Get(k)
		if !ok {
			return fmt.Errorf("no variable called %q", k)
		}
		vm.vmState.Push(v)

	case bytecode.Instruction_STORE_VARIABLE:
		k, ok := instruction.opA.(string)
		if !ok {
			return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
		}
		v, err := vm.vmState.Peek()
		if err != nil {
			return err
		}
		x, err := convertToFloat(v)
		if err != nil {
			return err
		}
		vm.VariableStorage.Set(k, x)

	case bytecode.Instruction_STOP:
		vm.execState = ExecStateStopped
		// TODO: report execution stopped?

	case bytecode.Instruction_RUN_NODE:
		node := ""
		if instruction.opA == nil || instruction.opA.(string) == "" {
			// Use the stack, Luke.
			t, err := vm.vmState.Peek()
			if err != nil {
				return err
			}
			n, ok := t.(string)
			if !ok {
				return fmt.Errorf("wrong type at top of stack [%T != string]", t)
			}
			node = n
		} else {
			n, ok := instruction.opA.(string)
			if !ok {
				return fmt.Errorf("wrong type in opA [%T != string]", instruction.opA)
			}
			node = n
		}
		// TODO: completion handler
		vm.vmState.node = node

	default:
		return fmt.Errorf("invalid instruction %v", instruction)
	}
	return nil
}
