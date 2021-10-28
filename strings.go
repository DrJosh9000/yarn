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
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	cldr "github.com/razor-1/localizer-cldr"
	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// StringTable contains all the information from a string table, keyed by
// string ID. This can be constructed either by using ReadStringTable, or
// manually (e.g. if you are not using Yarn Spinner CSV string tables but still
// want to use substitutions, format functions, and markup tags).
type StringTable struct {
	Language language.Tag
	Table    map[string]*StringTableRow
}

// LoadStringTableFile is a convenient function for loading a CSV string table
// given a file path. It assumes the first row is a header. langCode must be a
// valid BCP 47 language tag.
func LoadStringTableFile(stringTablePath, langCode string) (*StringTable, error) {
	csv, err := os.Open(stringTablePath)
	if err != nil {
		return nil, fmt.Errorf("opening string table file: %w", err)
	}
	defer csv.Close()
	st, err := ReadStringTable(csv, langCode)
	if err != nil {
		return nil, fmt.Errorf("reading string table: %w", err)
	}
	return st, nil
}

// ReadStringTable reads a CSV string table from the reader. It assumes the
// first row is a header. langCode must be a valid BCP 47 language tag.
// In addition to checking the CSV structure as it is parsed, each lineNumber
// is parsed as an int, and each text is also parsed. Any malformed substitution
// tokens or markup tags will cause an error.
func ReadStringTable(r io.Reader, langCode string) (*StringTable, error) {
	lang, err := language.Parse(langCode)
	if err != nil {
		return nil, fmt.Errorf("invalid lang code: %w", err)
	}

	st := make(map[string]*StringTableRow)
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
		// Line number must be an int
		ln, err := strconv.Atoi(rec[4])
		if err != nil {
			return nil, fmt.Errorf("line number not an int: %w", err)
		}
		id := rec[0]
		row := &StringTableRow{
			ID:         id,
			Text:       rec[1],
			File:       rec[2],
			Node:       rec[3],
			LineNumber: ln,
		}
		// Text must be parseable - parse it now to catch errors sooner
		if err := row.parseIfNeeded(); err != nil {
			return nil, fmt.Errorf("text for id %s could not be parsed: %w", id, err)
		}
		st[id] = row
	}
	return &StringTable{
		Language: lang,
		Table:    st,
	}, nil
}

// Render looks up the row corresponding to line.ID, interpolates substitutions
// (from line.Substitutions), applies format functions, and processes style
// tags into attributes.
func (t *StringTable) Render(line Line) (*AttributedString, error) {
	row := t.Table[line.ID]
	if row == nil {
		return nil, fmt.Errorf("string table row for id %q not found or nil", line.ID)
	}
	return row.Render(line.Substitutions, t.Language)
}

// StringTableRow contains all the information from one row in a string table.
type StringTableRow struct {
	ID, Text, File, Node string
	LineNumber           int

	origText   string // parsedText needs updating if Text changes
	parsedText *parsedString
}

// Render interpolates substitutions, applies format functions, and processes
// style tags into attributes.
func (r *StringTableRow) Render(substs []string, lang language.Tag) (*AttributedString, error) {
	if err := r.parseIfNeeded(); err != nil {
		return nil, err
	}
	lr := lineRenderer{
		substs: substs,
		lang:   lang,
	}
	if err := lr.renderString(r.parsedText); err != nil {
		return nil, err
	}
	return lr.attStr(), nil
}

// parseIfNeeded parses r.Text, if it has not been parsed already.
func (r *StringTableRow) parseIfNeeded() error {
	if r.Text == r.origText && r.parsedText != nil {
		return nil
	}
	filename := fmt.Sprintf("%s:%d", r.File, r.LineNumber)
	pt := new(parsedString)
	if err := lineParser.ParseString(filename, r.Text, pt); err != nil {
		return err
	}
	r.origText = r.Text
	r.parsedText = pt
	return nil
}

// AttributedString is a string with additional attributes, such as presentation
// or styling information, that apply to the whole string or substrings.
type AttributedString struct {
	Str        string
	Attributes []*Attribute
}

func (s *AttributedString) String() string { return s.Str }

// ScanAttribEvents calls visit with each Attribute twice: once at the start of
// the attribute, and once at the end. The visit calls will happen in order, but
// not necessarily the same order as they were originally presented in the
// source string.
//
// For example, an attributed string built from the input
// `[a]Here's A![/a][b]Here's B! [c]With C now[/c][/b]`
// could be visited in the order
// (`a`, start), (`b`, start), (`a`, end), (`c`, start), (`b`, end), (`c`, end).
func (s *AttributedString) ScanAttribEvents(visit func(att *Attribute, start bool)) {
	// Make a reference
	starts := s.Attributes
	// Sort starts by start
	sort.SliceStable(starts, func(i, j int) bool {
		return starts[i].Start < starts[j].Start
	})
	// Make a *copy*
	ends := append([]*Attribute(nil), s.Attributes...)
	// Sort ends by end
	sort.SliceStable(ends, func(i, j int) bool {
		return ends[i].End < ends[j].End
	})
	// March through starts and ends, and do visiting.
	// If the attributed string is well-formed (i.e. start <= end for every
	// attribute), all the starts must be visited before all the ends, so really
	// the loop only has to check i < len(starts).
	// But check j < len(ends) anwyay to avoid a panic in case it is malformed.
	i, j := 0, 0
	for i < len(starts) && j < len(ends) {
		if starts[i].Start <= ends[j].End {
			visit(starts[i], true)
			i++
		} else {
			visit(ends[j], false)
			j++
		}
	}
	for ; j < len(ends); j++ {
		visit(ends[j], false)
	}
}

// Attribute describes a range within a string with additional information
// provided by markup tags. Start and End specify the range in bytes. Name is
// the tag name, and Props contains any additional key="value" tag properties.
type Attribute struct {
	Start, End int
	Name       string
	Props      map[string]string
}

var (
	// This lexer is a bit more general than needed since it allows things like
	// nested functions, but... hey I get nested functions for ~free!
	lineLexer = lexer.MustStateful(lexer.Rules{
		"Root": {
			{Name: "Escaped", Pattern: `\\[\{\}\[\]"\\]`, Action: nil},
			{Name: "Markup", Pattern: `\[`, Action: lexer.Push("Markup")},
			{Name: "Subst", Pattern: `{`, Action: lexer.Push("Subst")},
			{Name: "Char", Pattern: `[%\{\["\\]|[^%\{\["\\]+`, Action: nil},
		},
		"Markup": {
			{Name: "Whitespace", Pattern: `\s+`, Action: nil},
			{Name: "Slash", Pattern: `/`, Action: nil},
			{Name: "Ident", Pattern: `\w+`, Action: nil},
			{Name: "Equals", Pattern: `=`, Action: nil},
			{Name: "Subst", Pattern: `{`, Action: lexer.Push("Subst")},
			{Name: "String", Pattern: `"`, Action: lexer.Push("String")},
			{Name: "MarkupEnd", Pattern: `\]`, Action: lexer.Pop()},
		},
		"Subst": {
			{Name: "Index", Pattern: `\d+`, Action: nil},
			{Name: "SubstEnd", Pattern: `}`, Action: lexer.Pop()},
		},
		"String": {
			{Name: "StringEnd", Pattern: `"`, Action: lexer.Pop()},
			lexer.Include("Root"),
		},
	})

	// a line is a kind of string, just missing the quotes...
	lineParser = participle.MustBuild(
		&parsedString{},
		participle.Lexer(lineLexer),
		participle.Elide("Whitespace"),
	)
)

// parsedString is used for both entire lines and the contents of double-quoted
// strings.
type parsedString struct {
	Fragments []*fragment `parser:"@@*"`
}

// fragment is part of a string or line. The parser breaks it into pieces so
// that special pieces (escape sequences, markup, substitutions, and %) can be
// processed in a special way.
type fragment struct {
	Escaped string           `parser:"@Escaped"`
	Markup  *parsedMarkupTag `parser:"| Markup @@ MarkupEnd"`
	Subst   string           `parser:"| Subst @Index SubstEnd"`
	Text    string           `parser:"| @Char"`
}

// stringOrSubst appears inside markup tags. The value={0} prop is emitted
// without quoting the substitution token. Other props are usually of the form
// key="value".
type stringOrSubst struct {
	String *parsedString `parser:"String @@ StringEnd"`
	Subst  string        `parser:" | Subst @Index SubstEnd"`
}

// parsedMarkupTag is used for both format functions (select, plural, ordinal) and
// BBCode-esque markup tags ([b]Bold!?[/b]).
type parsedMarkupTag struct {
	OpeningSlash string        `parser:"@Slash?"` // indicates closing tag of a pair
	Name         string        `parser:"@Ident?"` // used for all except close-all tag [/]
	Props        []*parsedProp `parser:"@@*"`     // optional key="value" or value={0} properties
	ClosingSlash string        `parser:"@Slash?"` // indicates self-closing tag
}

// parsedProp is used for key="value" properties of format funcs and markup
// tags.
type parsedProp struct {
	Key   string         `parser:"@Ident Equals"`
	Value *stringOrSubst `parser:"@@"` // for ordinary values
}

type lineRenderer struct {
	strings.Builder
	attribs []*Attribute
	open    map[string][]*Attribute // lazily created (by openTag)
	substs  []string
	lang    language.Tag
}

func (b *lineRenderer) attStr() *AttributedString {
	return &AttributedString{
		Str:        b.Builder.String(),
		Attributes: b.attribs,
	}
}

func (b *lineRenderer) openTag(name string, props []*parsedProp) error {
	// Render each prop value into its own string, and put into a map
	m := make(map[string]string)
	for _, prop := range props {
		v, err := b.evalStringOrSubst(prop.Value)
		if err != nil {
			return err
		}
		m[prop.Key] = v
	}
	a := &Attribute{
		Start: b.Builder.Len(),
		Name:  name,
		Props: m,
	}
	if b.open == nil {
		b.open = map[string][]*Attribute{name: {a}}
		return nil
	}
	b.open[name] = append(b.open[name], a)
	return nil
}

func (b *lineRenderer) closeTag(name string) error {
	if b.open == nil {
		return fmt.Errorf("tag %q not open", name)
	}
	as := b.open[name]
	l := len(as)
	if l == 0 {
		return fmt.Errorf("tag %q not open", name)
	}
	// Close the last one
	a, as := as[l-1], as[:l-1]
	b.open[name] = as
	a.End = b.Builder.Len()
	b.attribs = append(b.attribs, a)
	return nil
}

func (b *lineRenderer) closeAll() {
	for name, as := range b.open {
		for _, a := range as {
			a.End = b.Builder.Len()
			b.attribs = append(b.attribs, a)
		}
		delete(b.open, name)
	}
}

func (b *lineRenderer) renderString(p *parsedString) error {
	for _, f := range p.Fragments {
		if err := b.renderFragment(f); err != nil {
			return err
		}
	}
	return nil
}

func (b *lineRenderer) renderFragment(s *fragment) error {
	if s == nil {
		return nil
	}
	switch {
	case s.Escaped != "":
		b.WriteString(s.Escaped[1:])
	case s.Markup != nil:
		return b.renderMarkupTag(s.Markup)
	case s.Subst != "":
		b.WriteString(b.evalSubst(s.Subst))
	default:
		b.WriteString(s.Text)
	}
	return nil
}

func (b *lineRenderer) evalSubst(index string) string {
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 || n >= len(b.substs) {
		return "{" + index + "}"
	}
	return b.substs[n]
}

// maps plural.Form values to identifiers used in Yarn Spinner plural and
// ordinal format functions
var formKeyTable = []string{
	plural.Other: "other",
	plural.Zero:  "zero",
	plural.One:   "one",
	plural.Two:   "two",
	plural.Few:   "few",
	plural.Many:  "many",
}

func (b *lineRenderer) renderMarkupTag(f *parsedMarkupTag) error {
	switch {
	case f.Name == "select":
		// [select value={0} m="bro" f="sis" nb="doc" /]
		return b.renderSelectFormatFunc(f)

	case f.Name == "plural":
		// [plural value={0} one="an apple" other="% apples" /]
		return b.renderPluralFormatFunc(f, plural.Cardinal)

	case f.Name == "ordinal":
		// [ordinal value={0} one="%st" two="%nd" ... /]
		return b.renderPluralFormatFunc(f, plural.Ordinal)

	case f.OpeningSlash == "/" && f.Name == "":
		// Close-all tag [/]
		b.closeAll()
		return nil

	case f.OpeningSlash == "/":
		// Close tag [/foo]
		return b.closeTag(f.Name)

	case f.ClosingSlash == "/":
		// Self-closing tag [foo/]
		if err := b.openTag(f.Name, f.Props); err != nil {
			return err
		}
		return b.closeTag(f.Name)

	case f.Name != "":
		// Open tag [foo]
		return b.openTag(f.Name, f.Props)

	default:
		// Uhhhhhh... [] ?
		b.WriteString("[]")
		return nil
	}
}

// evalValueValue returns the string value of the markup tag property called
// "value". This is used by format functions.
func (b *lineRenderer) evalValueValue(f *parsedMarkupTag) (string, error) {
	// Find the value property.
	val, err := b.propValueForKey(f, "value")
	if err != nil {
		return "", err
	}
	// Evaluate its value!
	return b.evalStringOrSubst(val)
}

func (b *lineRenderer) renderSelectFormatFunc(f *parsedMarkupTag) error {
	// Get the value of the "value" property.
	input, err := b.evalValueValue(f)
	if err != nil {
		return err
	}
	// Use that value to find the matching property.
	val, err := b.propValueForKey(f, input)
	if err != nil {
		return err
	}
	// Render that value to the output!
	return b.renderFormatFuncValue(val, input)
}

func (b *lineRenderer) renderPluralFormatFunc(f *parsedMarkupTag, rules *plural.Rules) error {
	// Get the value of the "value" property.
	input, err := b.evalValueValue(f)
	if err != nil {
		return err
	}
	// Use that value to match the cardinal form.
	ops, err := cldr.NewOperands(input)
	if err != nil {
		return err
	}
	form := rules.MatchPlural(b.lang, int(ops.I), int(ops.V), int(ops.W), int(ops.F), int(ops.T))
	if int(form) > len(formKeyTable) {
		return fmt.Errorf("plural form %v not supported", form)
	}
	// Find the plural form in the properties.
	val, err := b.propValueForKey(f, formKeyTable[form])
	if err != nil {
		return err
	}
	// Render that value to the output!
	return b.renderFormatFuncValue(val, input)
}

func (b *lineRenderer) evalStringOrSubst(s *stringOrSubst) (string, error) {
	if s.Subst != "" {
		return b.evalSubst(s.Subst), nil
	}
	inb := &lineRenderer{
		substs: b.substs,
		lang:   b.lang,
	}
	if err := inb.renderString(s.String); err != nil {
		return "", err
	}
	return inb.String(), nil
}

// propValueForKey searches f.Props for the option matching the key, and
// then returns the Value.
func (b *lineRenderer) propValueForKey(f *parsedMarkupTag, key string) (*stringOrSubst, error) {
	for _, opt := range f.Props {
		if opt.Key == key {
			return opt.Value, nil
		}
	}
	return nil, fmt.Errorf("key %q not found in %#v", key, f.Props)
}

func (b *lineRenderer) renderFormatFuncValue(s *stringOrSubst, input string) error {
	// Format func values have an additional token that needs to be processed
	// specially (%).
	if s.Subst != "" {
		b.WriteString(b.evalSubst(s.Subst))
		return nil
	}
	for _, v := range s.String.Fragments {
		if v.Text == "%" {
			b.WriteString(input)
			continue
		}
		if err := b.renderFragment(v); err != nil {
			return err
		}
	}
	return nil
}
