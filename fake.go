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

import "errors"

// FakeDialogueHandler implements DialogueHandler with minimal, do-nothing
// methods. This is useful both for testing, and for satisfying the interface
// via embedding, e.g.:
//
//    type MyHandler struct {
//	      FakeDialogueHandler
//    }
//    // MyHandler is only interested in Line and Options.
//    func (m MyHandler) Line(line Line) error { ... }
//    func (m MyHandler) Options(options []Option) (int, error) { ... }
//    // All the other DialogueHandler methods provided by FakeDialogueHandler.
type FakeDialogueHandler struct{}

// NodeStart returns nil.
func (FakeDialogueHandler) NodeStart(string) error { return nil }

// PrepareForLines returns nil.
func (FakeDialogueHandler) PrepareForLines([]string) error { return nil }

// Line returns nil.
func (FakeDialogueHandler) Line(Line) error { return nil }

// Options returns the first option ID, or an error if there are no options.
func (FakeDialogueHandler) Options(options []Option) (int, error) {
	if len(options) == 0 {
		return 0, errors.New("no options delivered")
	}
	return options[0].ID, nil
}

// Command returns nil.
func (FakeDialogueHandler) Command(string) error { return nil }

// NodeComplete returns nil.
func (FakeDialogueHandler) NodeComplete(string) error { return nil }

// DialogueComplete returns nil.
func (FakeDialogueHandler) DialogueComplete() error { return nil }
