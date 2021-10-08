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
	"strings"

	yarnpb "github.com/DrJosh9000/yarn/bytecode"
)

// FormatInstruction prints an instruction in a format convenient for
// debugging. The output is intended for human consumption only.
func FormatInstruction(inst *yarnpb.Instruction) string {
	b := new(strings.Builder)
	fmt.Fprint(b, inst.Opcode)
	for _, op := range inst.Operands {
		switch op.Value.(type) {
		case *yarnpb.Operand_BoolValue:
			fmt.Fprintf(b, " %t", op.GetBoolValue())
		case *yarnpb.Operand_FloatValue:
			// Print as an int for instructions that use int operands
			switch inst.Opcode {
			case yarnpb.Instruction_PUSH_FLOAT:
				fmt.Fprintf(b, " %f", op.GetFloatValue())
			default:
				fmt.Fprintf(b, " %d", int(op.GetFloatValue()))
			}
		case *yarnpb.Operand_StringValue:
			fmt.Fprintf(b, " %q", op.GetStringValue())
		}
	}
	return b.String()
}

// FormatProgram prints a program in a format convenient for debugging. The
// output is intended for human consumption only.
func FormatProgram(prog *yarnpb.Program) string {
	b := new(strings.Builder)
	for name, node := range prog.Nodes {
		// Quick reverse label table
		labels := make(map[int]string)
		for l, a := range node.Labels {
			labels[int(a)] = l
		}

		fmt.Fprintf(b, "\n%s:\n", name)
		for n, inst := range node.Instructions {
			if l, ok := labels[n]; ok {
				fmt.Fprintf(b, "% 15s: ", l)
			} else {
				fmt.Fprint(b, strings.Repeat(" ", 17))
			}
			fmt.Fprintf(b, "%06d %s\n", n, FormatInstruction(inst))
		}
	}
	return b.String()
}
