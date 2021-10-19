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
	"unicode"

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
	return t.render(row.Text, line.Substitutions)
}

func (t *StringTable) render(text string, substs []string) (string, error) {
	pl := new(parsedLine)
	if err := lineParser.ParseString("", text, pl); err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := pl.assemble(&sb, substs); err != nil {
		return "", err
	}
	return sb.String(), nil
}

var (
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

	lineParser = participle.MustBuild(
		&parsedLine{},
		participle.Lexer(lineLexer),
		participle.Elide("Whitespace"),
	)
)

type parsedLine struct {
	Fragments []*fragment `@@*`
}

func (p *parsedLine) assemble(sb *strings.Builder, substs []string) error {
	for _, f := range p.Fragments {
		if err := f.assemble(sb, substs); err != nil {
			return err
		}
	}
	return nil
}

type parsedFunc struct {
	Name  string       `@Ident`
	Input *fragment    `"\"" @@ "\""`
	Opts  []*parsedOpt `@@+`
}

func (f *parsedFunc) assemble(sb *strings.Builder, substs []string) error {
	// input is a fragment that needs assembling
	var inb strings.Builder
	if err := f.Input.assemble(&inb, substs); err != nil {
		return err
	}
	in := inb.String()

	// function name determines lookup key
	switch f.Name {
	case "select":
		// input chooses which value to interpolate
		// (input == lookup key)
		return f.findAndAssemble(sb, substs, in, in)

	case "plural":
		// TODO: use CLDR
		if in == "1" {
			return f.findAndAssemble(sb, substs, in, "one")
		}
		return f.findAndAssemble(sb, substs, in, "other")

	case "ordinal":
		// TODO: use CLDR
		switch in {
		case "1":
			return f.findAndAssemble(sb, substs, in, "one")
		case "2":
			return f.findAndAssemble(sb, substs, in, "two")
		case "3":
			return f.findAndAssemble(sb, substs, in, "few")
		default:
			return f.findAndAssemble(sb, substs, in, "other")
		}

	default:
		return fmt.Errorf("unknown format function %q", f.Name)
	}
}

func (f *parsedFunc) findAndAssemble(sb *strings.Builder, substs []string, input, key string) error {
	for _, opt := range f.Opts {
		if opt.Key == key {
			return opt.assemble(sb, substs, input)
		}
	}
	return fmt.Errorf("key %q not found in %#v", key, f.Opts)
}

type fragment struct {
	Escaped string       `@Escaped`
	Func    *parsedFunc  `| "[" @@ "]"`
	Subst   *parsedSubst `| "{" @@ "}"`
	Text    string       `| @Char`
}

func (s *fragment) assemble(sb *strings.Builder, substs []string) error {
	if s == nil {
		return nil
	}
	switch {
	case s.Escaped != "":
		sb.WriteString(s.Escaped[1:])
	case s.Func != nil:
		return s.Func.assemble(sb, substs)
	case s.Subst != nil:
		return s.Subst.assemble(sb, substs)
	default:
		sb.WriteString(s.Text)
	}
	return nil
}

type parsedSubst struct {
	Index string `@Index`
}

func (s *parsedSubst) assemble(sb *strings.Builder, substs []string) error {
	n, err := strconv.Atoi(s.Index)
	if err != nil {
		return err
	}
	sb.WriteString(substs[n])
	return nil
}

type parsedOpt struct {
	Key   string      `@Ident "="`
	Value []*fragment `"\"" @@* "\""`
}

func (o *parsedOpt) assemble(sb *strings.Builder, substs []string, input string) error {
	for _, v := range o.Value {
		if v.Text == "%" {
			sb.WriteString(input)
			continue
		}
		if err := v.assemble(sb, substs); err != nil {
			return err
		}
	}
	return nil
}

func (t *StringTable) renderByHand(text string, substs []string) (string, error) {
	// Do substitutions and format functions in one big stateful loop.
	var sb strings.Builder

	// parser states
	const (
		raw = iota
		subst
		fmtf
		fmtfstr
		fmtfsub
		eatspace
	)
	escape := false // true means next rune is escaped
	stack, state := []int{}, raw
	push := func(st int) { stack, state = append(stack, state), st }
	pop := func() { stack, state = stack[:len(stack)-1], stack[len(stack)-1] }
	var substIdx int
	var fb strings.Builder

	for _, r := range text {
	loopStart:
		switch state {
		case raw:
			if escape {
				switch r {
				case '[', ']', '{', '}':
					// escape sequences in raw state are \[, \], \{, and \}
				default:
					// this char is not part of an escape sequence, or the
					// sequence is otherwise passed through
					sb.WriteRune('\\')
				}
				sb.WriteRune(r)
				escape = false
				break
			}
			switch r {
			case '\\':
				escape = true
			case '[':
				push(fmtf)
				push(eatspace)
			case '{':
				substIdx = 0
				push(subst)
			default:
				sb.WriteRune(r)
			}

		case subst:
			switch {
			case unicode.IsDigit(r):
				substIdx *= 10
				substIdx += int(r - '0')
			case r == '}':
				if substIdx < 0 || substIdx >= len(substs) {
					return "", fmt.Errorf("substitution index %d out of range [0, %d)", substIdx, len(substs))
				}
				sb.WriteString(substs[substIdx])
				pop()
			default:
				return "", fmt.Errorf("invalid rune %c in substitution", r)
			}

		case fmtf:
			switch r {
			case '"':
				// a string within the format function
				fb.WriteRune(r)
				push(fmtfstr)
			case ']':
				// parse and evaluate the function
				ff := new(fmtFunc)
				if err := funcParser.ParseString("", fb.String(), ff); err != nil {
					return "", fmt.Errorf("parsing format function %q: %w", fb.String(), err)
				}
				fr, err := ff.render(substs, t.langCode)
				if err != nil {
					return "", err
				}
				sb.WriteString(fr)
				pop()
			default:
				fb.WriteRune(r)
			}

		case fmtfstr:
			// a string within function
			if escape {
				switch r {
				case '{', '}':
					// escape sequences in this state are \{, \}
				default:
					// this char is not part of an escape sequence, or the
					// sequence is passed through
					fb.WriteRune('\\')
				}
				fb.WriteRune(r)
				escape = false
				break
			}
			switch r {
			case '\\':
				escape = true
			case '{':
				// substitution within string within function
				substIdx = 0
				push(fmtfsub)
			case '"':
				// end of string
				fb.WriteRune(r)
				pop()
			default:
				fb.WriteRune(r)
			}

		case fmtfsub:
			// substitution within a string within a function
			switch {
			case unicode.IsDigit(r):
				substIdx *= 10
				substIdx += int(r - '0')
			case r == '}':
				if substIdx < 0 || substIdx >= len(substs) {
					return "", fmt.Errorf("substitution index %d out of range [0, %d)", substIdx, len(substs))
				}
				fb.WriteString(substs[substIdx])
				pop()
			default:
				return "", fmt.Errorf("invalid rune %c in substitution", r)
			}

		case eatspace:
			if !unicode.IsSpace(r) {
				pop()
				// Reprocess r from the start of the loop body.
				goto loopStart
			}

		default:
			return "", fmt.Errorf("unknown parser state %d", state)
		}
	}
	if len(stack) != 0 || state != raw {
		return "", fmt.Errorf("parser ended line in bad state %d [stack:%v]", state, stack)
	}
	if escape {
		// trailing backslash is not an escape sequence
		sb.WriteRune('\\')
	}
	return sb.String(), nil
}

type fmtFunc struct {
	Name  string `parser:"@Ident"`
	Input string `parser:"@String"`
	Opts  []struct {
		K string `parser:"@Ident '='"`
		V string `parser:"@String"`
	} `parser:"@@+"`
}

var funcParser = participle.MustBuild(&fmtFunc{})

func (f *fmtFunc) render(substs []string, langCode string) (string, error) {
	in, err := strconv.Unquote(f.Input)
	if err != nil {
		return "", err
	}

	switch f.Name {
	case "select":
		// input chooses which value to interpolate
		for _, kv := range f.Opts {
			if kv.K == in {
				out := strings.ReplaceAll(kv.V, "%", in)
				return strconv.Unquote(out)
			}
		}
		return "", fmt.Errorf("key %q not found in %v", in, f.Opts)

	case "plural":
		// input is itself a number to feed through CLDR to find the
		// pluralisation case
		n, err := strconv.Atoi(in)
		if err != nil {
			return "", fmt.Errorf("format function input not an integer: %w", err)
		}
		// TODO: use CLDR
		switch n {
		case 1:
			for _, kv := range f.Opts {
				if kv.K == "one" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}
		default:
			for _, kv := range f.Opts {
				if kv.K == "other" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}
		}

	case "ordinal":
		// input is itself a number to feed through CLDR to find the
		// ordinal case
		// TODO
		n, err := strconv.Atoi(in)
		if err != nil {
			return "", fmt.Errorf("format function input not an integer: %w", err)
		}
		// TODO: use CLDR
		switch n {
		case 1:
			for _, kv := range f.Opts {
				if kv.K == "one" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}

		case 2:
			for _, kv := range f.Opts {
				if kv.K == "two" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}

		case 3:
			for _, kv := range f.Opts {
				if kv.K == "few" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}

		default:
			for _, kv := range f.Opts {
				if kv.K == "other" {
					out := strings.ReplaceAll(kv.V, "%", in)
					return strconv.Unquote(out)
				}
			}
		}
	}
	return "", fmt.Errorf("unknown format function %q", f.Name)
}
