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
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// StringTableRow contains all the information from one row in a string table.
type StringTableRow struct {
	ID, Text, File, Node string
	LineNumber           int
}

// StringTable contains all the information from a string table, keyed by
// string ID.
type StringTable struct {
	langCode string
	table    map[string]StringTableRow
}

// ReadStringTable reads a CSV-formatted string table from the reader. It
// assumes the first row is a header.
func ReadStringTable(r io.Reader, langCode string) (*StringTable, error) {
	st := make(map[string]StringTableRow)
	header := true
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 5
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv read: %v", err)
		}
		if header {
			header = false
			continue
		}
		ln, err := strconv.Atoi(rec[4])
		if err != nil {
			return nil, fmt.Errorf("atoi: %w", err)
		}
		id := rec[0]
		st[id] = StringTableRow{
			ID:         id,
			Text:       rec[1],
			File:       rec[2],
			Node:       rec[3],
			LineNumber: ln,
		}
	}
	return &StringTable{
		langCode: langCode,
		table:    st,
	}, nil
}

// Render looks up the row corresponding to line.ID, interpolates substitutions
// (from line.Substitutions), and applies format functions.
func (t *StringTable) Render(line Line) (string, error) {
	row, found := t.table[line.ID]
	if !found {
		return "", fmt.Errorf("no string %q in string table", line.ID)
	}
	// Parse the line according to the grammar below.
	filename := fmt.Sprintf("%s:%d", row.File, row.LineNumber)
	pl := new(parsedString)
	if err := lineParser.ParseString(filename, row.Text, pl); err != nil {
		return "", err
	}

	// Apply substitutions and format functions into a string builder.
	var sb strings.Builder
	if err := pl.render(&sb, line.Substitutions); err != nil {
		return "", err
	}
	return sb.String(), nil
}

var (
	// This lexer is a bit more general than needed since it allows things like
	// nested functions, but... hey I get nested functions for ~free!
	lineLexer = lexer.MustStateful(lexer.Rules{
		"Root": {
			{Name: "Escaped", Pattern: `\\[{}\[\]"]`, Action: nil},
			{Name: "Func", Pattern: `\[`, Action: lexer.Push("Func")},
			{Name: "Subst", Pattern: `{`, Action: lexer.Push("Subst")},
			{Name: "Char", Pattern: `%|[^{\[%"]+`, Action: nil},
		},
		"Func": {
			{Name: "Whitespace", Pattern: `\s+`, Action: nil},
			{Name: "Ident", Pattern: `\w+`, Action: nil},
			{Name: "Equals", Pattern: `=`, Action: nil},
			{Name: "String", Pattern: `"`, Action: lexer.Push("String")},
			{Name: "FuncEnd", Pattern: `\]`, Action: lexer.Pop()},
		},
		"Subst": {
			{Name: "Index", Pattern: `\d+`, Action: nil},
			{Name: "SubstEnd", Pattern: `}`, Action: lexer.Pop()},
		},
		"String": {
			lexer.Include("Root"),
			{Name: "StringEnd", Pattern: `"`, Action: lexer.Pop()},
		},
	})

	// a line is a kind of string, just missing the quotes...
	lineParser = participle.MustBuild(
		&parsedString{},
		participle.Lexer(lineLexer),
		participle.Elide("Whitespace"),
	)
)

type parsedString struct {
	Fragments []*fragment `parser:"@@*"`
}

func (p *parsedString) render(sb *strings.Builder, substs []string) error {
	for _, f := range p.Fragments {
		if err := f.render(sb, substs); err != nil {
			return err
		}
	}
	return nil
}

type fragment struct {
	Escaped string       `parser:"@Escaped"`
	Func    *parsedFunc  `parser:"| '[' @@ ']'"`
	Subst   *parsedSubst `parser:"| '{' @@ '}'"`
	Text    string       `parser:"| @Char"`
}

func (s *fragment) render(sb *strings.Builder, substs []string) error {
	if s == nil {
		return nil
	}
	switch {
	case s.Escaped != "":
		sb.WriteString(s.Escaped[1:])
	case s.Func != nil:
		return s.Func.render(sb, substs)
	case s.Subst != nil:
		return s.Subst.render(sb, substs)
	default:
		sb.WriteString(s.Text)
	}
	return nil
}

type parsedFunc struct {
	Name  string        `parser:"@Ident"`
	Input *parsedString `parser:"\"\\\"\" @@ \"\\\"\""`
	Opts  []*parsedOpt  `parser:"@@+"`
}

func (f *parsedFunc) render(sb *strings.Builder, substs []string) error {
	// input is a fragment that needs assembling
	var inb strings.Builder
	if err := f.Input.render(&inb, substs); err != nil {
		return err
	}
	in := inb.String()

	// function name determines lookup key
	switch f.Name {
	case "select":
		// input chooses which value to interpolate
		// (input == lookup key)
		return f.findAndRender(sb, substs, in, in)

	case "plural":
		// TODO: use CLDR
		if in == "1" {
			return f.findAndRender(sb, substs, in, "one")
		}
		return f.findAndRender(sb, substs, in, "other")

	case "ordinal":
		// TODO: use CLDR
		switch in {
		case "1":
			return f.findAndRender(sb, substs, in, "one")
		case "2":
			return f.findAndRender(sb, substs, in, "two")
		case "3":
			return f.findAndRender(sb, substs, in, "few")
		default:
			return f.findAndRender(sb, substs, in, "other")
		}

	default:
		return fmt.Errorf("unknown format function %q", f.Name)
	}
}

// findAndRender searches f.Opts for the option matching the key, and then
// renders that option to sb.
func (f *parsedFunc) findAndRender(sb *strings.Builder, substs []string, input, key string) error {
	for _, opt := range f.Opts {
		if opt.Key == key {
			return opt.render(sb, substs, input)
		}
	}
	return fmt.Errorf("key %q not found in %#v", key, f.Opts)
}

type parsedSubst struct {
	Index string `parser:"@Index"`
}

func (s *parsedSubst) render(sb *strings.Builder, substs []string) error {
	n, err := strconv.Atoi(s.Index)
	if err != nil {
		return err
	}
	if n < 0 || n >= len(substs) {
		return fmt.Errorf("substitution index %d out of range [0, %d)", n, len(substs))
	}
	sb.WriteString(substs[n])
	return nil
}

type parsedOpt struct {
	Key   string        `parser:"@Ident '='"`
	Value *parsedString `parser:"\"\\\"\" @@ \"\\\"\""`
}

func (o *parsedOpt) render(sb *strings.Builder, substs []string, input string) error {
	// Options have an additional token that needs to be processed specially
	// (%), so don't just call o.Value.render.
	for _, v := range o.Value.Fragments {
		if v.Text == "%" {
			sb.WriteString(input)
			continue
		}
		if err := v.render(sb, substs); err != nil {
			return err
		}
	}
	return nil
}
