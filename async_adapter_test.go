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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// syncAdapter converts the AsyncAdapter back into the synchronous interface.
type syncAdapter struct {
	dh DialogueHandler
	aa *AsyncAdapter
	t  *testing.T
}

func (s *syncAdapter) NodeStart(nodeName string) {
	if err := s.dh.NodeStart(nodeName); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.NodeStart(%q) = %v", nodeName, err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func (s *syncAdapter) PrepareForLines(lineIDs []string) {
	if err := s.dh.PrepareForLines(lineIDs); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.PrepareForLines(%q) = %v", lineIDs, err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func (s *syncAdapter) Line(line Line) {
	if err := s.dh.Line(line); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.Line(%v) = %v", line, err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func (s *syncAdapter) Options(options []Option) {
	choice, err := s.dh.Options(options)
	if err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.Options(%v) = %v", options, err)
	}
	if err := s.aa.GoWithChoice(choice); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.GoWithChoice(%d) = %v", choice, err)
	}
}

func (s *syncAdapter) Command(command string) {
	if err := s.dh.Command(command); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.Command(%v) = %v", command, err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func (s *syncAdapter) NodeComplete(nodeName string) {
	if err := s.dh.NodeComplete(nodeName); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.NodeComplete(%q) = %v", nodeName, err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func (s *syncAdapter) DialogueComplete() {
	if err := s.dh.DialogueComplete(); err != nil {
		s.t.Errorf("syncAdapter.DialogueHandler.DialogueComplete() = %v", err)
	}
	if err := s.aa.Go(); err != nil {
		s.t.Errorf("syncAdapter.AsyncAdapter.Go() = %v", err)
	}
}

func TestAllTestPlansAsync(t *testing.T) {
	testplans, err := filepath.Glob("testdata/*.testplan")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	for _, tpn := range testplans {
		t.Run(tpn, func(t *testing.T) {
			testplan, err := LoadTestPlanFile(tpn)
			if err != nil {
				t.Fatalf("LoadTestPlanFile(%q) = error %v", tpn, err)
			}

			// VM -> AsyncAdapter -> syncAdapter -> testplan
			sa := &syncAdapter{
				dh: testplan,
				t:  t,
			}
			sa.aa = NewAsyncAdapter(sa)

			base := strings.TrimSuffix(filepath.Base(tpn), ".testplan")

			yarnc := "testdata/" + base + ".yarnc"
			prog, st, err := LoadFiles(yarnc, "en")
			if err != nil {
				t.Fatalf("LoadFiles(%q, en) = error %v", yarnc, err)
			}

			vm := &VirtualMachine{
				Program: prog,
				Handler: sa.aa,
				Vars:    NewMapVariableStorage(),
				FuncMap: FuncMap{
					// Used by various
					"assert": func(x interface{}) error {
						t, err := ConvertToBool(x)
						if err != nil {
							return err
						}
						if !t {
							return errors.New("assertion failed")
						}
						return nil
					},
					// Used by Functions.yarn
					// TODO: support ints like the real Yarn Spinner
					"add_three_operands": func(x, y, z float32) float32 {
						return x + y + z
					},
					"last_value": func(x ...interface{}) (interface{}, error) {
						if len(x) == 0 {
							return nil, errors.New("no args")
						}
						return x[len(x)-1], nil
					},
					"dummy_number": func() float32 {
						return 1
					},
					"dummy_bool": func() bool {
						return true
					},
					"dummy_string": func() string {
						return "string"
					},
				},
			}
			testplan.StringTable = st
			if traceOutput {
				vm.TraceLogf = t.Logf
			}

			if err := vm.Run("Start"); err != nil {
				t.Errorf("vm.Run(Start) = %v", err)
			}
			if err := testplan.Complete(); err != nil {
				t.Errorf("testplan incomplete: %v", err)
			}
		})
	}
}

type decoupledEvent struct {
	followup func(*AsyncAdapter) error
	desc     string
}

// decoupledAsyncHandler handles each event and then stores a follow-up action
// to run afterwards.
type decoupledAsyncHandler struct {
	FakeAsyncDialogueHandler
	eventCh chan decoupledEvent
}

func (d *decoupledAsyncHandler) Line(line Line) {
	d.eventCh <- decoupledEvent{
		followup: (*AsyncAdapter).Go,
		desc:     fmt.Sprintf("Line(%v)", line),
	}
}

func (d *decoupledAsyncHandler) Options(options []Option) {
	d.eventCh <- decoupledEvent{
		followup: func(aa *AsyncAdapter) error {
			return aa.GoWithChoice(options[0].ID)
		},
		desc: fmt.Sprintf("Options(%v)", options),
	}
}

func (d *decoupledAsyncHandler) Command(command string) {
	d.eventCh <- decoupledEvent{
		followup: (*AsyncAdapter).Go,
		desc:     fmt.Sprintf("Command(%q)", command),
	}
}

func (d *decoupledAsyncHandler) DialogueComplete() {
	close(d.eventCh)
	d.AsyncAdapter.Go()
}

func TestAsyncAdapterWithDecoupledHandler(t *testing.T) {
	yarnc := "testdata/Example.yarnc"
	prog, _, err := LoadFiles(yarnc, "en")
	if err != nil {
		t.Fatalf("LoadFiles(%q, en) = error %v", yarnc, err)
	}

	dh := &decoupledAsyncHandler{
		eventCh: make(chan decoupledEvent),
	}
	aa := NewAsyncAdapter(dh)
	dh.AsyncAdapter = aa

	vm := &VirtualMachine{
		Program: prog,
		Handler: aa,
		Vars:    NewMapVariableStorage(),
	}
	if traceOutput {
		vm.TraceLogf = t.Logf
	}

	go func() {
		if err := vm.Run("Start"); err != nil {
			t.Errorf("vm.Run(Start) = %v", err)
		}
	}()

	for e := range dh.eventCh {
		if err := e.followup(aa); err != nil {
			t.Errorf("followup after %s: %v", e.desc, err)
		}
	}
}

// immediateAsyncHandler calls Go and GoWithChoice within each event.
type immediateAsyncHandler struct {
	FakeAsyncDialogueHandler
	t *testing.T
}

func (i *immediateAsyncHandler) Line(Line) {
	if err := i.AsyncAdapter.Go(); err != nil {
		i.t.Errorf("AsyncAdapter.Go() = %v", err)
	}
}

func (i *immediateAsyncHandler) Options(options []Option) {
	id := options[0].ID
	if err := i.AsyncAdapter.GoWithChoice(id); err != nil {
		i.t.Errorf("AsyncAdapter.GoWithChoice(%d) = %v", id, err)
	}
}

func (i *immediateAsyncHandler) Command(string) {
	if err := i.AsyncAdapter.Go(); err != nil {
		i.t.Errorf("AsyncAdapter.Go() = %v", err)
	}
}

func TestAsyncAdapterWithImmediateHandler(t *testing.T) {
	yarnc := "testdata/Example.yarnc"
	prog, _, err := LoadFiles(yarnc, "en")
	if err != nil {
		t.Fatalf("LoadFiles(%q, en) = error %v", yarnc, err)
	}

	ah := &immediateAsyncHandler{
		t: t,
	}
	aa := NewAsyncAdapter(ah)
	ah.AsyncAdapter = aa

	vm := &VirtualMachine{
		Program: prog,
		Handler: aa,
		Vars:    NewMapVariableStorage(),
	}
	if traceOutput {
		vm.TraceLogf = t.Logf
	}

	if err := vm.Run("Start"); err != nil {
		t.Errorf("vm.Run(Start) = %v", err)
	}
}

// badAsyncHandler calls the wrong continuation methods first, then the right
// ones.
type badAsyncHandler struct {
	FakeAsyncDialogueHandler
	t *testing.T
}

func (b *badAsyncHandler) Line(Line) {
	want := VMStateMismatchErr{
		Got:  VMStatePaused,
		Want: VMStatePausedOptions,
		Next: VMStateRunning,
	}
	if diff := cmp.Diff(b.AsyncAdapter.GoWithChoice(6), want); diff != "" {
		b.t.Errorf("AsyncAdapter.GoWithChoice(6) error diff (-got +want):\n%s", diff)
	}
	// call Go to proceed, otherwise it hangs (it's waiting for Go, duh)
	if err := b.AsyncAdapter.Go(); err != nil {
		b.t.Errorf("AsyncAdapter.Go() = %v", err)
	}
}

func (b *badAsyncHandler) Options(options []Option) {
	want := VMStateMismatchErr{
		Got:  VMStatePausedOptions,
		Want: VMStatePaused,
		Next: VMStateRunning,
	}
	if diff := cmp.Diff(b.AsyncAdapter.Go(), want); diff != "" {
		b.t.Errorf("AsyncAdapter.Go() error diff (-got +want):\n%s", diff)
	}
	// call GoWithChoice to proceed, otherwise it hangs
	choice := options[0].ID
	if err := b.AsyncAdapter.GoWithChoice(choice); err != nil {
		b.t.Errorf("AsyncAdapter.GoWithChoice(%d) = %v", choice, err)
	}
}

func (b *badAsyncHandler) Command(string) {
	want := VMStateMismatchErr{
		Got:  VMStatePaused,
		Want: VMStatePausedOptions,
		Next: VMStateRunning,
	}
	if diff := cmp.Diff(b.AsyncAdapter.GoWithChoice(0), want); diff != "" {
		b.t.Errorf("AsyncAdapter.GoWithChoice(0) error diff (-got +want):\n%s", diff)
	}
	// pass Go, collect $200
	if err := b.AsyncAdapter.Go(); err != nil {
		b.t.Errorf("AsyncAdapter.Go() = %v", err)
	}
}

func TestAsyncAdapterWithBadHandler(t *testing.T) {
	yarnc := "testdata/Example.yarnc"
	prog, _, err := LoadFiles(yarnc, "en")
	if err != nil {
		t.Fatalf("LoadFiles(%q, en) = error %v", yarnc, err)
	}

	bh := &badAsyncHandler{t: t}
	aa := NewAsyncAdapter(bh)
	bh.AsyncAdapter = aa

	vm := &VirtualMachine{
		Program: prog,
		Handler: aa,
		Vars:    NewMapVariableStorage(),
	}
	if traceOutput {
		vm.TraceLogf = t.Logf
	}

	if err := vm.Run("Start"); err != nil {
		t.Errorf("vm.Run(Start) = %v", err)
	}
}

var errDummy = errors.New("abort! abort!")

// abortAsyncHandler calls Abort within each event.
type abortAsyncHandler struct {
	FakeAsyncDialogueHandler
	t *testing.T
}

func (a *abortAsyncHandler) Line(Line) {
	if err := a.AsyncAdapter.Abort(errDummy); err != nil {
		a.t.Errorf("AsyncAdapter.Abort(errDummy) = %v", err)
	}
}

func (a *abortAsyncHandler) Options(options []Option) {
	if err := a.AsyncAdapter.Abort(errDummy); err != nil {
		a.t.Errorf("AsyncAdapter.Abort(errDummy) = %v", err)
	}
}

func (a *abortAsyncHandler) Command(string) {
	if err := a.AsyncAdapter.Abort(errDummy); err != nil {
		a.t.Errorf("AsyncAdapter.Abort(errDummy) = %v", err)
	}
}

func TestAsyncAdapterWithAbortHandler(t *testing.T) {
	yarnc := "testdata/Example.yarnc"
	prog, _, err := LoadFiles(yarnc, "en")
	if err != nil {
		t.Fatalf("LoadFiles(%q, en) = error %v", yarnc, err)
	}

	bh := &abortAsyncHandler{t: t}
	aa := NewAsyncAdapter(bh)
	bh.AsyncAdapter = aa

	vm := &VirtualMachine{
		Program: prog,
		Handler: aa,
		Vars:    NewMapVariableStorage(),
	}
	if traceOutput {
		vm.TraceLogf = t.Logf
	}

	if err := vm.Run("Start"); !errors.Is(err, errDummy) {
		t.Errorf("vm.Run(Start) = %v, want %v", err, errDummy)
	}

	// aborting while stopped should give an error
	if err := aa.Abort(errDummy); !errors.Is(err, ErrAlreadyStopped) {
		t.Errorf("aa.Abort(errDummy) = %v, want %v", err, ErrAlreadyStopped)
	}
}
