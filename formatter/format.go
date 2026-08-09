package formatter

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Options controls source layout. Zero values are replaced by DefaultOptions.
type Options struct {
	IndentWidth int
	PrintWidth  int
	UseTabs     bool
	EndOfLine   string // "lf", "crlf", or "auto"
}

// DefaultOptions returns the stable command-line defaults.
func DefaultOptions() Options {
	return Options{IndentWidth: 2, PrintWidth: 80, EndOfLine: "auto"}
}

// Format returns a deterministic, idempotent formatting of a Jinja template.
func Format(source string, opts Options) (string, error) {
	opts = normalizedOptions(opts)
	bom := ""
	if strings.HasPrefix(source, "\ufeff") {
		bom, source = "\ufeff", strings.TrimPrefix(source, "\ufeff")
	}
	originalCRLF := strings.Contains(source, "\r\n")
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

	tokens, err := Lex(source)
	if err != nil {
		return "", err
	}
	if err := validate(tokens); err != nil {
		return "", err
	}
	if source == "" {
		return bom, nil
	}

	closers := make(map[string]bool)
	for _, tok := range tokens {
		if tok.Kind == Statement {
			if name, ok := closingName(tok.Keyword); ok {
				closers[name] = true
			}
		}
	}

	type protectedBlock struct {
		text       string
		reindent   bool
		rawElement bool
	}
	protected := make(map[string]protectedBlock)
	var normalized strings.Builder
	for i, tok := range tokens {
		switch tok.Kind {
		case Text:
			normalized.WriteString(tok.Content)
		case Raw, Comment:
			key := uniquePlaceholder(source, i+1)
			protected[key] = protectedBlock{tok.Content, tok.Reindent, tok.RawElement}
			normalized.WriteString(key)
		case Expression, Statement:
			normalized.WriteString(formatJinjaTag(tok))
		}
	}

	expanded := expandStructuralLines(normalized.String(), closers)
	lines := formatLines(expanded, opts, closers)
	lines = compactSimpleElements(lines, opts.PrintWidth)
	lines = breakLongSimpleElements(lines, opts)
	result := strings.Join(lines, "\n")
	for _, key := range sortedKeys(protected) {
		block := protected[key]
		if block.rawElement {
			result = replaceRawElement(result, key, block.text, opts)
		} else if block.reindent {
			result = replaceReindented(result, key, block.text)
		} else {
			result = strings.ReplaceAll(result, key, block.text)
		}
	}
	result = strings.TrimRight(result, " \t\n") + "\n"
	if strings.TrimSpace(result) == "" {
		result = ""
	}

	eol := opts.EndOfLine
	if eol == "auto" && originalCRLF {
		eol = "crlf"
	}
	if eol == "crlf" {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	return bom + result, nil
}

func replaceRawElement(source, key, replacement string, opts Options) string {
	idx := strings.Index(source, key)
	if idx < 0 {
		return source
	}
	lineStart := strings.LastIndex(source[:idx], "\n") + 1
	prefix := source[lineStart:idx]
	if strings.Trim(prefix, " \t") != "" {
		prefix = ""
	}
	lines := strings.Split(replacement, "\n")
	if len(lines) == 1 {
		return strings.ReplaceAll(source, key, strings.TrimSpace(replacement))
	}
	for i := range lines {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			lines[i] = ""
			continue
		}
		if i == 0 {
			lines[i] = trimmed
			continue
		}
		if i == len(lines)-1 {
			lines[i] = prefix + trimmed
			continue
		}
		lines[i] = prefix + indentation(1, opts) + trimmed
	}
	return source[:idx] + strings.Join(lines, "\n") + source[idx+len(key):]
}

func normalizedOptions(opts Options) Options {
	defaults := DefaultOptions()
	if opts.IndentWidth <= 0 {
		opts.IndentWidth = defaults.IndentWidth
	}
	if opts.PrintWidth <= 0 {
		opts.PrintWidth = defaults.PrintWidth
	}
	if opts.EndOfLine == "" {
		opts.EndOfLine = defaults.EndOfLine
	}
	return opts
}

func formatJinjaTag(tok Token) string {
	open, close := "{{", "}}"
	if tok.Kind == Statement {
		open, close = "{%", "%}"
	}
	content := strings.TrimSpace(tok.Content)
	if !strings.Contains(content, "\n") {
		return open + tok.Left + " " + content + " " + tok.Right + close
	}
	lines := dedentLines(strings.Split(content, "\n"))
	return open + tok.Left + "\n" + strings.Join(lines, "\n") + "\n" + tok.Right + close
}

func dedentLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	min := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if min < 0 || n < min {
			min = n
		}
	}
	if min < 0 {
		return nil
	}
	for i := range lines {
		if len(lines[i]) >= min {
			lines[i] = strings.TrimRight(lines[i][min:], " \t")
		}
	}
	return lines
}

func formatLines(source string, opts Options, closers map[string]bool) []string {
	rawLines := strings.Split(source, "\n")
	multiPre, multiPost := multilineStatementDeltas(rawLines, closers)
	result := make([]string, 0, len(rawLines))
	indent, multilineTag, expressionDepth := 0, 0, 0
	blank := false
	for lineIndex, original := range rawLines {
		trimmed := strings.TrimSpace(original)
		if trimmed == "" {
			if len(result) > 0 && !blank {
				result = append(result, "")
				blank = true
			}
			continue
		}
		blank = false
		if isStandaloneClosingStatement(trimmed, closers) && len(result) > 0 && result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}

		// Lines inside a multiline Jinja tag are one level deeper than its delimiters.
		if isMultilineTagClose(trimmed) && multilineTag > 0 {
			multilineTag--
			expressionDepth = 0
		}
		bracketPre, bracketPost := 0, 0
		if multilineTag > 0 {
			bracketPre, bracketPost = bracketIndentDelta(trimmed)
			expressionDepth += bracketPre
			if expressionDepth < 0 {
				expressionDepth = 0
			}
		}
		preJinja, postJinja := statementIndentDelta(trimmed, closers)
		preJinja += multiPre[lineIndex]
		postJinja += multiPost[lineIndex]
		preHTML, postHTML := htmlIndentDelta(trimmed)
		indent += preJinja + preHTML
		if indent < 0 {
			indent = 0
		}

		line := normalizeHTMLTags(trimmed)
		level := indent + multilineTag + expressionDepth
		result = append(result, indentation(level, opts)+line)

		indent += postJinja + postHTML
		if indent < 0 {
			indent = 0
		}
		if multilineTag > 0 {
			expressionDepth += bracketPost
		}
		if isMultilineTagOpen(trimmed) {
			multilineTag++
		}
	}
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result
}

func multilineStatementDeltas(lines []string, closers map[string]bool) (map[int]int, map[int]int) {
	pre, post := make(map[int]int), make(map[int]int)
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "{%" && trimmed != "{%-" && trimmed != "{%+" {
			continue
		}
		var body []string
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if isMultilineTagClose(strings.TrimSpace(lines[j])) {
				end = j
				break
			}
			body = append(body, strings.TrimSpace(lines[j]))
		}
		if end < 0 {
			continue
		}
		keyword := firstWord(strings.Join(body, "\n"))
		role, _ := roleOf(Token{Kind: Statement, Keyword: keyword, Content: strings.Join(body, "\n")}, closers)
		switch role {
		case closer:
			pre[i]--
		case branch:
			pre[i]--
			post[end]++
		case opener:
			post[end]++
		}
		i = end
	}
	return pre, post
}

func isStandaloneClosingStatement(line string, closers map[string]bool) bool {
	tokens, err := Lex(line)
	if err != nil {
		return false
	}
	found := false
	for _, tok := range tokens {
		if tok.Kind == Text && strings.TrimSpace(tok.Content) != "" {
			return false
		}
		if tok.Kind == Statement {
			role, _ := roleOf(tok, closers)
			if role != closer && role != branch {
				return false
			}
			found = true
		}
	}
	return found
}

func statementIndentDelta(line string, closers map[string]bool) (pre, post int) {
	tokens, err := Lex(line)
	if err != nil {
		return 0, 0
	}
	for _, tok := range tokens {
		if tok.Kind == Text && strings.TrimSpace(tok.Content) != "" {
			return 0, 0
		}
		if tok.Kind != Statement {
			continue
		}
		role, _ := roleOf(tok, closers)
		switch role {
		case closer:
			pre--
		case branch:
			pre--
			post++
		case opener:
			post++
		}
	}
	return pre, post
}

func isMultilineTagOpen(line string) bool {
	return line == "{{" || line == "{{-" || line == "{{+" || line == "{%" || line == "{%-" || line == "{%+"
}

func isMultilineTagClose(line string) bool {
	return line == "}}" || line == "-}}" || line == "+}}" || line == "%}" || line == "-%}" || line == "+%}"
}

func indentation(level int, opts Options) string {
	if level <= 0 {
		return ""
	}
	if opts.UseTabs {
		return strings.Repeat("\t", level)
	}
	return strings.Repeat(" ", level*opts.IndentWidth)
}

func uniquePlaceholder(source string, id int) string {
	for {
		key := fmt.Sprintf("JINJAFMT_RAW_%d_TOKEN", id)
		if !strings.Contains(source, key) {
			return key
		}
		id++
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return keys
}

func replaceReindented(source, key, replacement string) string {
	for {
		idx := strings.Index(source, key)
		if idx < 0 {
			return source
		}
		lineStart := strings.LastIndex(source[:idx], "\n") + 1
		prefix := source[lineStart:idx]
		if strings.Trim(prefix, " \t") != "" {
			prefix = ""
		}
		parts := strings.Split(replacement, "\n")
		for i := 1; i < len(parts); i++ {
			if parts[i] != "" {
				parts[i] = prefix + strings.TrimLeft(parts[i], " \t")
			}
		}
		replacement = strings.Join(parts, "\n")
		source = source[:idx] + replacement + source[idx+len(key):]
	}
}

func expandStructuralLines(source string, closers map[string]bool) string {
	var output []string
	for _, line := range strings.Split(source, "\n") {
		tokens, err := Lex(line)
		if err != nil {
			output = append(output, line)
			continue
		}
		depth, needsSplit, structural := 0, false, 0
		for _, tok := range tokens {
			if tok.Kind != Statement {
				continue
			}
			role, _ := roleOf(tok, closers)
			if role == neutral {
				continue
			}
			structural++
			switch role {
			case closer:
				if depth == 0 {
					needsSplit = true
				} else {
					depth--
				}
			case branch:
				if depth == 0 {
					needsSplit = true
				}
			case opener:
				depth++
			}
		}
		if structural == 0 || !needsSplit {
			output = append(output, splitHTMLSiblings(line)...)
			continue
		}
		last := 0
		for _, tok := range tokens {
			if tok.Kind != Statement {
				continue
			}
			role, _ := roleOf(tok, closers)
			if role == neutral {
				continue
			}
			if before := strings.TrimSpace(line[last:tok.Start]); before != "" {
				output = append(output, before)
			}
			output = append(output, formatJinjaTag(tok))
			last = tok.End
		}
		if after := strings.TrimSpace(line[last:]); after != "" {
			output = append(output, splitHTMLSiblings(after)...)
		}
	}
	return strings.Join(output, "\n")
}

func bracketIndentDelta(line string) (pre, post int) {
	var quote rune
	escaped := false
	first := true
	for _, r := range line {
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
		if r == '\'' || r == '"' {
			quote = r
			first = false
			continue
		}
		if unicode.IsSpace(r) {
			continue
		}
		switch r {
		case ')', ']', '}':
			if first {
				pre--
			} else {
				post--
			}
		case '(', '[', '{':
			post++
		}
		first = false
	}
	return
}
