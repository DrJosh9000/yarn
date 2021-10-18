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
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// TestStep is a step in a test plan.
type TestStep struct {
	Type     string
	Contents string
}

func (s TestStep) String() string { return s.Type + ": " + s.Contents }

// TestPlan is a helper for .testplan files.
type TestPlan struct {
	Steps []TestStep
	Step  int

	DialogueCompleted bool

	StringTable    *StringTable
	VirtualMachine *VirtualMachine
}

// ReadTestPlan reads a testplan file into a TestPlan.
func ReadTestPlan(r io.Reader) (*TestPlan, error) {
	var tp TestPlan
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		txt := strings.TrimSpace(sc.Text())
		if txt == "" || strings.HasPrefix(txt, "#") {
			// Skip blanks and comments
			continue
		}
		if strings.HasPrefix(txt, "stop") {
			// Superfluous stop at end of file
			break
		}
		tok := strings.SplitN(txt, ":", 2)
		if len(tok) < 2 {
			return nil, fmt.Errorf("malformed step %q", txt)
		}
		tp.Steps = append(tp.Steps, TestStep{
			Type:     strings.TrimSpace(tok[0]),
			Contents: strings.TrimSpace(tok[1]),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &tp, nil
}

// Complete checks if the test plan was completed.
func (p *TestPlan) Complete() error {
	if p.Step != len(p.Steps) {
		return fmt.Errorf("on step %d %v", p.Step, p.Steps[p.Step])
	}
	if !p.DialogueCompleted {
		return errors.New("did not receive DialogueCompleted")
	}
	return nil
}

// Line checks that the line matches the one expected by the plan.
func (p *TestPlan) Line(line Line) error {
	if p.Step >= len(p.Steps) {
		return errors.New("next step after end")
	}
	step := p.Steps[p.Step]
	if step.Type != "line" {
		return fmt.Errorf("testplan got line, want %q", step.Type)
	}
	p.Step++
	text, err := p.StringTable.Render(line)
	if err != nil {
		return err
	}
	if text != step.Contents {
		return fmt.Errorf("testplan got line %q, want %q", text, step.Contents)
	}
	return nil
}

// Options checks that the options match those expected by the plan, then
// selects the option specified in the plan.
func (p *TestPlan) Options(opts []Option) (int, error) {
	for _, opt := range opts {
		if p.Step >= len(p.Steps) {
			return 0, errors.New("next testplan step after end")
		}
		step := p.Steps[p.Step]
		if step.Type != "option" {
			return 0, fmt.Errorf("testplan got option, want %q", step.Type)
		}
		p.Step++
		text, err := p.StringTable.Render(opt.Line)
		if err != nil {
			return 0, err
		}
		if text != step.Contents {
			return 0, fmt.Errorf("testplan got option line %q, want %q", text, step.Contents)
		}
	}
	// Next step should be a select
	if p.Step >= len(p.Steps) {
		return 0, errors.New("next testplan step after end")
	}
	step := p.Steps[p.Step]
	if step.Type != "select" {
		return 0, fmt.Errorf("testplan got select, want %q", step.Type)
	}
	p.Step++
	n, err := strconv.Atoi(step.Contents)
	if err != nil {
		return 0, fmt.Errorf("converting testplan step to int: %w", err)
	}
	return n - 1, nil
}

// Command handles the command... somehow.
func (p *TestPlan) Command(command string) error {
	// Handle "commands" that should be compiled differently ?
	if strings.HasPrefix(command, "jump ") {
		// v2 should compile this as RUN_NODE, I guess?
		return p.VirtualMachine.SetNode(strings.TrimPrefix(command, "jump "))
	}

	if p.Step >= len(p.Steps) {
		return errors.New("next testplan step after end")
	}
	step := p.Steps[p.Step]
	if step.Type != "command" {
		return fmt.Errorf("testplan got command, want %q", step.Type)
	}
	p.Step++
	// TODO: check the command
	return nil
}

// DialogueComplete records the event in p.DialogueCompleted.
func (p *TestPlan) DialogueComplete() error {
	p.DialogueCompleted = true
	return nil
}

// NodeStart does nothing and returns nil.
func (p *TestPlan) NodeStart(string) error { return nil }

// NodeComplete does nothing and returns nil.
func (p *TestPlan) NodeComplete(string) error { return nil }

// PrepareForLines does nothing and returns nil.
func (p *TestPlan) PrepareForLines([]string) error { return nil }
