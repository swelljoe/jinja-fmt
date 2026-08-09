package formatter

import (
	"fmt"
	"strings"
)

type statementRole uint8

const (
	neutral statementRole = iota
	opener
	branch
	closer
)

type blockFrame struct {
	name string
	tok  Token
}

var standardBlocks = map[string]bool{
	"if": true, "for": true, "block": true, "macro": true, "call": true,
	"filter": true, "with": true, "autoescape": true, "trans": true,
}

func validate(tokens []Token) error {
	closers := make(map[string]bool)
	for _, tok := range tokens {
		if tok.Kind == Statement {
			if name, ok := closingName(tok.Keyword); ok {
				closers[name] = true
			}
		}
	}
	var stack []blockFrame
	for _, tok := range tokens {
		if tok.Kind != Statement {
			continue
		}
		role, name := roleOf(tok, closers)
		switch role {
		case opener:
			stack = append(stack, blockFrame{name, tok})
		case branch:
			if len(stack) == 0 {
				return parseAt(tok, fmt.Sprintf("%q has no enclosing block", tok.Keyword))
			}
			if tok.Keyword == "elif" && stack[len(stack)-1].name != "if" {
				return parseAt(tok, "elif is only valid inside an if block")
			}
		case closer:
			if len(stack) == 0 {
				return parseAt(tok, fmt.Sprintf("closing statement %q has no opener", tok.Keyword))
			}
			got := stack[len(stack)-1].name
			if got != name {
				return parseAt(tok, fmt.Sprintf("closing statement %q does not match open %q block at %d:%d", tok.Keyword, got, stack[len(stack)-1].tok.Line, stack[len(stack)-1].tok.Col))
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		frame := stack[len(stack)-1]
		return parseAt(frame.tok, fmt.Sprintf("unclosed %q block", frame.name))
	}
	return nil
}

func parseAt(tok Token, message string) error {
	return &ParseError{Line: tok.Line, Column: tok.Col, Message: message}
}

func roleOf(tok Token, closers map[string]bool) (statementRole, string) {
	keyword := tok.Keyword
	if name, ok := closingName(keyword); ok {
		return closer, name
	}
	if keyword == "else" || keyword == "elif" || keyword == "pluralize" {
		return branch, keyword
	}
	if standardBlocks[keyword] || closers[keyword] {
		if keyword == "set" && hasTopLevelAssignment(tok.Content) {
			return neutral, ""
		}
		return opener, keyword
	}
	// A capture-style set is only unambiguous when an endset occurs. This also
	// keeps partially edited `{% set name %}` statements formatable.
	if keyword == "set" && closers["set"] && !hasTopLevelAssignment(tok.Content) {
		return opener, keyword
	}
	return neutral, ""
}

func closingName(keyword string) (string, bool) {
	if !strings.HasPrefix(keyword, "end") || len(keyword) == 3 {
		return "", false
	}
	name := strings.TrimPrefix(keyword[3:], "_")
	return name, name != ""
}

func hasTopLevelAssignment(s string) bool {
	var quote rune
	escaped, depth := false, 0
	for _, r := range s {
		if quote != 0 {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '=':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}
