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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScanAttribEvents(t *testing.T) {
	input := "[a]Hello A[/a] [b]Hello B[/b] [c][d][/c]No C, [e/]only D[/d]"

	// parse input
	pt, err := lineParser.ParseString("", input)
	if err != nil {
		t.Fatalf("lineParser.ParseString: %v", err)
	}
	// render input
	lr := lineRenderer{}
	if err := lr.renderString(pt); err != nil {
		t.Fatalf("lineRenderer.renderString: %v", err)
	}
	as := lr.attStr()

	attA := &Attribute{
		Start: 0,
		End:   7,
		Name:  "a",
	}
	attB := &Attribute{
		Start: 8,
		End:   15,
		Name:  "b",
	}
	attC := &Attribute{
		Start: 16,
		End:   16,
		Name:  "c",
	}
	attD := &Attribute{
		Start: 16,
		End:   28,
		Name:  "d",
	}
	attE := &Attribute{
		Start: 22,
		End:   22,
		Name:  "e",
	}

	type posAtts struct {
		Pos  int
		Atts []*Attribute
	}
	want := []posAtts{
		{0, []*Attribute{attA}},
		{7, []*Attribute{attA}},
		{8, []*Attribute{attB}},
		{15, []*Attribute{attB}},
		{16, []*Attribute{attC, attD}},
		{22, []*Attribute{attE}},
		{28, []*Attribute{attD}},
	}
	var got []posAtts
	as.ScanAttribEvents(func(pos int, atts []*Attribute) {
		got = append(got, posAtts{
			Pos:  pos,
			Atts: atts,
		})
	})

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("ScanAttribEvents scan order diff:\n%s", diff)
	}
}
