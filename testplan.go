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

// TestPlan is a helper for .testplan files.
type TestPlan struct {
	Steps []TestStep
	Step  int
	VM    *VirtualMachine
}

// ReadTestPlane reads a testplan file into a TestPlan.
func ReadTestPlan(r io.Reader) (*TestPlan, error) {
	var tp TestPlan
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		tok := strings.SplitN(sc.Text(), ": ", 2)
		if len(tok) < 2 {
			return nil, fmt.Errorf("malformed testplan step %q", sc.Text())
		}
		tp.Steps = append(tp.Steps, TestStep{
			Type:     tok[0],
			Contents: tok[1],
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
		return fmt.Errorf("testplan incomplete on step %d", p.Step)
	}
	return nil
}

func (p *TestPlan) Line(line Line) error {
	step := p.Steps[p.Step]
	if step.Type != "line" {
		return fmt.Errorf("testplan got line, want %q", step.Type)
	}
	p.Step++
	// TODO: check the line
	return nil
}

func (p *TestPlan) Options(opts []Option) error {
	for range opts {
		step := p.Steps[p.Step]
		if step.Type != "option" {
			return fmt.Errorf("testplan got option, want %q", step.Type)
		}
		p.Step++
		// TODO: check the option
	}
	// Next step should be a select
	step := p.Steps[p.Step]
	if step.Type != "select" {
		return fmt.Errorf("testplan got select, want %q", step.Type)
	}
	p.Step++
	n, err := strconv.Atoi(step.Contents)
	if err != nil {
		return fmt.Errorf("converting testplan step to int: %w", err)
	}
	return p.VM.SetSelectedOption(n - 1)
}

func (p *TestPlan) Command(command string) error {
	// TODO: how are commands handled in real yarnspinner's testplan?
	if false {
		step := p.Steps[p.Step]
		if step.Type != "command" {
			return fmt.Errorf("testplan got command, want %q", step.Type)
		}
		p.Step++
	}
	// TODO: check the command
	return nil
}

func (p *TestPlan) NodeStart(nodeName string) error {
	return nil
}

func (p *TestPlan) NodeComplete(nodeName string) error {
	return nil
}

func (p *TestPlan) DialogueComplete() error {
	return nil
}

func (p *TestPlan) PrepareForLines(lineIDs []string) error {
	return nil
}
