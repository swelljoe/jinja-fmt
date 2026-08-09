// Package formatter implements a dependency-free, template-aware Jinja formatter.
package formatter

import (
	"fmt"
	"strings"
	"unicode"
)

// Kind identifies a top-level Jinja lexical item.
type Kind uint8

const (
	Text Kind = iota
	Expression
	Statement
	Comment
	Raw
)

// Token is a lossless top-level token. Content excludes delimiters for Jinja
// tags and contains the complete source for Text and Raw tokens.
type Token struct {
	Kind                  Kind
	Content, Left, Right  string
	Keyword               string
	Start, End, Line, Col int
	Reindent              bool // continuation lines follow the placeholder's indentation
	RawElement            bool // opaque script/style element
}

// ParseError reports a malformed template with a source location.
type ParseError struct {
	Line, Column int
	Message      string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message)
}

// Lex splits source into Jinja tags and literal regions. Delimiters inside
// quoted strings are ignored, including escaped quotes.
func Lex(source string) ([]Token, error) {
	var out []Token
	line, col := 1, 1
	for pos := 0; pos < len(source); {
		next, special := nextSpecial(source, pos)
		if next < 0 {
			out = appendText(out, source[pos:], pos, len(source), line, col)
			break
		}
		if next > pos {
			chunk := source[pos:next]
			out = appendText(out, chunk, pos, next, line, col)
			line, col = advance(line, col, chunk)
			pos = next
		}

		startLine, startCol := line, col
		switch special {
		case "<!--":
			end := strings.Index(source[pos+4:], "-->")
			if end < 0 {
				end = len(source)
			} else {
				end += pos + 7
			}
			// A range ignore is one opaque token so its contents are byte-for-byte stable.
			if isHTMLIgnoreStart(source[pos:end]) {
				if stop := findHTMLIgnoreEnd(source, end); stop >= 0 {
					end = stop
				}
			}
			reindent := false
			if isHTMLIgnoreSingle(source[pos:end]) {
				end = findIgnoredLineEnd(source, end)
				reindent = true
			}
			raw := source[pos:end]
			out = append(out, Token{Kind: Raw, Content: raw, Start: pos, End: end, Line: line, Col: col, Reindent: reindent})
			line, col = advance(line, col, raw)
			pos = end
		case "<script", "<style":
			name := special[1:]
			end := findRawHTMLElementEnd(source, pos, name)
			if end < 0 {
				// It may merely be a similarly named custom element; leave it to HTML formatting.
				out = appendText(out, source[pos:pos+1], pos, pos+1, line, col)
				pos++
				col++
				continue
			}
			raw := source[pos:end]
			out = append(out, Token{Kind: Raw, Content: raw, Start: pos, End: end, Line: line, Col: col, RawElement: true})
			line, col = advance(line, col, raw)
			pos = end
		case "{#":
			idx := strings.Index(source[pos+2:], "#}")
			if idx < 0 {
				return nil, &ParseError{startLine, startCol, "unclosed Jinja comment"}
			}
			end := pos + 2 + idx + 2
			content := source[pos+2 : end-2]
			if isJinjaIgnoreStart(content) {
				if stop := findJinjaIgnoreEnd(source, end); stop >= 0 {
					end = stop
				}
			}
			raw := source[pos:end]
			out = append(out, Token{Kind: Comment, Content: raw, Start: pos, End: end, Line: line, Col: col})
			line, col = advance(line, col, raw)
			pos = end
		case "{{", "{%":
			closing := "}}"
			kind := Expression
			if special == "{%" {
				closing, kind = "%}", Statement
			}
			endStart := findTagEnd(source, pos+2, closing)
			if endStart < 0 {
				name := "expression"
				if kind == Statement {
					name = "statement"
				}
				return nil, &ParseError{startLine, startCol, "unclosed Jinja " + name}
			}
			inner := source[pos+2 : endStart]
			left, right, content := splitModifiers(inner)
			end := endStart + 2
			keyword := ""
			if kind == Statement {
				keyword = firstWord(content)
				if keyword == "" {
					return nil, &ParseError{startLine, startCol, "empty Jinja statement"}
				}
				if keyword == "raw" {
					if rawEnd := findEndRaw(source, end); rawEnd >= 0 {
						raw := source[pos:rawEnd]
						out = append(out, Token{Kind: Raw, Content: raw, Start: pos, End: rawEnd, Line: line, Col: col})
						line, col = advance(line, col, raw)
						pos = rawEnd
						continue
					}
					return nil, &ParseError{startLine, startCol, "unclosed Jinja raw block"}
				}
			}
			raw := source[pos:end]
			out = append(out, Token{Kind: kind, Content: content, Left: left, Right: right, Keyword: keyword, Start: pos, End: end, Line: line, Col: col})
			line, col = advance(line, col, raw)
			pos = end
		}
	}
	return out, nil
}

func appendText(tokens []Token, text string, start, end, line, col int) []Token {
	if text == "" {
		return tokens
	}
	return append(tokens, Token{Kind: Text, Content: text, Start: start, End: end, Line: line, Col: col})
}

func nextSpecial(s string, from int) (int, string) {
	best, which := -1, ""
	for _, marker := range []string{"{{", "{%", "{#", "<!--"} {
		if n := strings.Index(s[from:], marker); n >= 0 && (best < 0 || from+n < best) {
			best, which = from+n, marker
		}
	}
	lower := strings.ToLower(s[from:])
	for _, marker := range []string{"<script", "<style"} {
		if n := strings.Index(lower, marker); n >= 0 && (best < 0 || from+n < best) {
			// Ensure this is a tag name boundary.
			after := from + n + len(marker)
			if after == len(s) || unicode.IsSpace(rune(s[after])) || s[after] == '>' {
				best, which = from+n, marker
			}
		}
	}
	return best, which
}

func findTagEnd(s string, from int, closing string) int {
	fallback := strings.Index(s[from:], closing)
	var quote byte
	escaped := false
	for i := from; i+1 < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if s[i:i+2] == closing {
			return i
		}
	}
	// A lenient fallback matches Jinja's useful error-recovery behavior for
	// partially edited templates with an unmatched quote.
	if fallback >= 0 {
		return from + fallback
	}
	return -1
}

func splitModifiers(inner string) (left, right, content string) {
	content = inner
	if len(content) > 0 && (content[0] == '-' || content[0] == '+') {
		left, content = content[:1], content[1:]
	}
	content = strings.TrimSpace(content)
	if len(content) > 0 && (content[len(content)-1] == '-' || content[len(content)-1] == '+') {
		right, content = content[len(content)-1:], strings.TrimSpace(content[:len(content)-1])
	}
	return
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return s[:i]
		}
	}
	return s
}

func advance(line, col int, s string) (int, int) {
	if n := strings.Count(s, "\n"); n > 0 {
		line += n
		col = len(s) - strings.LastIndex(s, "\n")
	} else {
		col += len(s)
	}
	return line, col
}

func isJinjaIgnoreStart(content string) bool {
	return strings.TrimSpace(strings.Trim(content, "-+")) == "prettier-ignore-start"
}

func findJinjaIgnoreEnd(s string, from int) int {
	for pos := from; pos < len(s); {
		n := strings.Index(s[pos:], "{#")
		if n < 0 {
			return -1
		}
		start := pos + n
		end := strings.Index(s[start+2:], "#}")
		if end < 0 {
			return -1
		}
		stop := start + 2 + end + 2
		content := s[start+2 : stop-2]
		if strings.TrimSpace(strings.Trim(content, "-+")) == "prettier-ignore-end" {
			return stop
		}
		pos = stop
	}
	return -1
}

func isHTMLIgnoreStart(comment string) bool {
	inside := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(comment, "<!--"), "-->"))
	return inside == "prettier-ignore-start"
}

func isHTMLIgnoreSingle(comment string) bool {
	end := strings.Index(comment, "-->")
	if end < 0 {
		return false
	}
	inside := strings.TrimSpace(strings.TrimPrefix(comment[:end], "<!--"))
	return inside == "prettier-ignore"
}

func findIgnoredLineEnd(s string, from int) int {
	firstNewline := strings.IndexByte(s[from:], '\n')
	if firstNewline < 0 {
		return len(s)
	}
	firstNewline += from
	if strings.TrimSpace(s[from:firstNewline]) != "" {
		return firstNewline
	}
	secondNewline := strings.IndexByte(s[firstNewline+1:], '\n')
	if secondNewline < 0 {
		return len(s)
	}
	return firstNewline + 1 + secondNewline
}

func findHTMLIgnoreEnd(s string, from int) int {
	for pos := from; pos < len(s); {
		n := strings.Index(s[pos:], "<!--")
		if n < 0 {
			return -1
		}
		start := pos + n
		n = strings.Index(s[start+4:], "-->")
		if n < 0 {
			return -1
		}
		end := start + 4 + n + 3
		inside := strings.TrimSpace(s[start+4 : end-3])
		if inside == "prettier-ignore-end" {
			return end
		}
		pos = end
	}
	return -1
}

func findEndRaw(s string, from int) int {
	for pos := from; pos < len(s); {
		n := strings.Index(s[pos:], "{%")
		if n < 0 {
			return -1
		}
		start := pos + n
		endStart := findTagEnd(s, start+2, "%}")
		if endStart < 0 {
			return -1
		}
		_, _, body := splitModifiers(s[start+2 : endStart])
		if firstWord(body) == "endraw" {
			return endStart + 2
		}
		pos = endStart + 2
	}
	return -1
}

func findRawHTMLElementEnd(s string, start int, name string) int {
	lower := strings.ToLower(s[start:])
	needle := "</" + name
	n := strings.Index(lower, needle)
	if n < 0 {
		return -1
	}
	closeStart := start + n
	gt := strings.IndexByte(s[closeStart:], '>')
	if gt < 0 {
		return -1
	}
	return closeStart + gt + 1
}
