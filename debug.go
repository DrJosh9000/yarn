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
	"io"
	"strconv"
	"strings"

	yarnpb "github.com/kalexmills/yarn/bytecode"
)

// FormatInstruction prints an instruction in a format convenient for
// debugging. The output is intended for human consumption only and may change
// between incremental versions of this package.
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

// FormatProgram prints a program in a format convenient for debugging to the
// io.Writer. The output is intended for human consumption only and may change
// between incremental versions of this package.
func FormatProgram(w io.Writer, prog *yarnpb.Program) error {
	// Make all the labels line up, even across nodes
	labelWidth := 0
	for _, node := range prog.Nodes {
		for l := range node.Labels {
			if len(l) > labelWidth {
				labelWidth = len(l)
			}
		}
	}
	labelFmt := "% " + strconv.Itoa(labelWidth) + "s: "
	noLabel := strings.Repeat(" ", labelWidth+2)

	// Now print the program into the string builder
	for name, node := range prog.Nodes {
		// Quick reverse label table
		labels := make(map[int]string)
		for l, a := range node.Labels {
			labels[int(a)] = l
		}

		if _, err := fmt.Fprintf(w, "%s--- %s tags:%v---\n", noLabel, name, node.Tags); err != nil {
			return err
		}
		if node.SourceTextStringID != "" {
			if _, err := fmt.Fprintf(w, "%sSourceTextStringID: %q\n", noLabel, node.SourceTextStringID); err != nil {
				return err
			}
		}
		for n, inst := range node.Instructions {
			if l := labels[n]; l != "" {
				if _, err := fmt.Fprintf(w, labelFmt, l); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprint(w, noLabel); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%06d %s\n", n, FormatInstruction(inst)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// FormatProgramString prints the whole program into a string.
func FormatProgramString(prog *yarnpb.Program) string {
	var sb strings.Builder
	FormatProgram(&sb, prog)
	return sb.String()
}
