package formatter

import (
	"strings"
	"unicode"
)

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var blockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"body": true, "div": true, "dl": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true, "header": true,
	"html": true, "li": true, "main": true, "nav": true, "ol": true, "p": true,
	"section": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "ul": true,
}

func normalizeHTMLTags(line string) string {
	var out strings.Builder
	for pos := 0; pos < len(line); {
		start := nextHTMLTag(line, pos)
		if start < 0 {
			out.WriteString(line[pos:])
			break
		}
		out.WriteString(line[pos:start])
		end := scanHTMLTag(line, start)
		if end < 0 {
			out.WriteString(line[start:])
			break
		}
		out.WriteString(formatHTMLTag(line[start:end]))
		pos = end
	}
	return collapseTemplateSeams(out.String())
}

func collapseTemplateSeams(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '>' {
			out.WriteByte(s[i])
			i++
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j+1 < len(s) && s[j] == '{' && (s[j+1] == '{' || s[j+1] == '%') {
				i = j
			}
			continue
		}
		if i+1 < len(s) && ((s[i] == '}' && s[i+1] == '}') || (s[i] == '%' && s[i+1] == '}')) {
			out.WriteString(s[i : i+2])
			i += 2
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j < len(s) && (s[j] == '<' || (j+1 < len(s) && s[j] == '{' && s[j+1] == '%')) {
				i = j
			}
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func nextHTMLTag(s string, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] != '<' || i+1 >= len(s) {
			continue
		}
		c := s[i+1]
		if c == '/' || c == '!' || c == '?' || unicode.IsLetter(rune(c)) {
			return i
		}
	}
	return -1
}

func scanHTMLTag(s string, start int) int {
	if strings.HasPrefix(s[start:], "<!--") {
		if n := strings.Index(s[start+4:], "-->"); n >= 0 {
			return start + 4 + n + 3
		}
		return -1
	}
	var quote byte
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '>' {
			return i + 1
		}
	}
	return -1
}

func formatHTMLTag(tag string) string {
	if strings.HasPrefix(tag, "<!--") || strings.HasPrefix(tag, "<!") || strings.HasPrefix(tag, "<?") {
		return tag
	}
	inner := strings.TrimSpace(tag[1 : len(tag)-1])
	closing := strings.HasPrefix(inner, "/")
	selfClosing := strings.HasSuffix(inner, "/")
	if closing {
		return "</" + strings.TrimSpace(strings.TrimPrefix(inner, "/")) + ">"
	}
	if selfClosing {
		inner = strings.TrimSpace(strings.TrimSuffix(inner, "/"))
	}
	inner = collapseTagWhitespace(inner)
	name := htmlTagName(inner)
	if selfClosing || voidElements[strings.ToLower(name)] {
		return "<" + inner + " />"
	}
	return "<" + inner + ">"
}

func collapseTagWhitespace(s string) string {
	var out strings.Builder
	var quote byte
	pendingSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			if pendingSpace && out.Len() > 0 {
				out.WriteByte(' ')
				pendingSpace = false
			}
			quote = c
			out.WriteByte(c)
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' {
			pendingSpace = true
			continue
		}
		if pendingSpace && out.Len() > 0 {
			out.WriteByte(' ')
		}
		pendingSpace = false
		out.WriteByte(c)
	}
	return strings.TrimSpace(out.String())
}

func htmlTagName(inner string) string {
	inner = strings.TrimLeft(inner, "/ \t")
	for i, r := range inner {
		if unicode.IsSpace(r) || r == '/' || r == '>' {
			return inner[:i]
		}
	}
	return inner
}

func htmlIndentDelta(line string) (pre, post int) {
	type event struct{ closing, opening bool }
	var events []event
	for pos := 0; ; {
		start := nextHTMLTag(line, pos)
		if start < 0 {
			break
		}
		end := scanHTMLTag(line, start)
		if end < 0 {
			break
		}
		tag := line[start:end]
		pos = end
		if strings.HasPrefix(tag, "<!") || strings.HasPrefix(tag, "<?") {
			continue
		}
		inner := strings.TrimSpace(tag[1 : len(tag)-1])
		closing := strings.HasPrefix(inner, "/")
		name := strings.ToLower(htmlTagName(inner))
		opening := !closing && !strings.HasSuffix(inner, "/") && !voidElements[name]
		events = append(events, event{closing, opening})
	}
	// Matched tags on one physical line are inline and have no net indentation.
	depth := 0
	for _, ev := range events {
		if ev.closing {
			depth--
		} else if ev.opening {
			depth++
		}
	}
	if depth < 0 {
		pre = depth
	} else {
		post = depth
	}
	return
}

func compactSimpleElements(lines []string, width int) []string {
	for changed := true; changed; {
		changed = false
		out := make([]string, 0, len(lines))
		for i := 0; i < len(lines); {
			if i+2 < len(lines) {
				prefix, openName, ok := loneOpeningTag(lines[i])
				closeName, closeOK := loneClosingTag(lines[i+2])
				middle := strings.TrimSpace(lines[i+1])
				candidate := prefix + strings.TrimSpace(lines[i]) + middle + strings.TrimSpace(lines[i+2])
				if ok && closeOK && openName == closeName && middle != "" && !strings.Contains(middle, "<"+openName) && !strings.Contains(middle, "JINJAFMT_RAW_") && len(candidate) <= width {
					out = append(out, candidate)
					i += 3
					changed = true
					continue
				}
			}
			out = append(out, lines[i])
			i++
		}
		lines = out
	}
	return lines
}

func breakLongSimpleElements(lines []string, opts Options) []string {
	var out []string
	for _, line := range lines {
		if len(line) <= opts.PrintWidth {
			out = append(out, line)
			continue
		}
		prefix := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		trimmed := strings.TrimSpace(line)
		openEnd := scanHTMLTag(trimmed, 0)
		if openEnd <= 0 {
			out = append(out, line)
			continue
		}
		name := strings.ToLower(htmlTagName(strings.TrimSpace(trimmed[1 : openEnd-1])))
		closing := "</" + name + ">"
		if !strings.HasSuffix(strings.ToLower(trimmed), closing) || voidElements[name] {
			out = append(out, line)
			continue
		}
		middle := trimmed[openEnd : len(trimmed)-len(closing)]
		if middle == "" || strings.Contains(strings.ToLower(middle), "<"+name) {
			out = append(out, line)
			continue
		}
		out = append(out, prefix+trimmed[:openEnd], prefix+indentation(1, opts)+strings.TrimSpace(middle), prefix+closing)
	}
	return out
}

func splitHTMLSiblings(line string) []string {
	var result []string
	start := 0
	for pos := 0; pos < len(line); {
		i := strings.Index(line[pos:], "><")
		if i < 0 {
			break
		}
		i += pos
		leftStart := strings.LastIndex(line[start:i], "<")
		if leftStart >= 0 {
			leftStart += start
		}
		rightEnd := scanHTMLTag(line, i+1)
		if leftStart >= 0 && rightEnd > 0 {
			left := line[leftStart : i+1]
			right := line[i+1 : rightEnd]
			leftInner := strings.TrimSpace(left[1 : len(left)-1])
			rightInner := strings.TrimSpace(right[1 : len(right)-1])
			leftClosing := strings.HasPrefix(leftInner, "/")
			rightOpening := !strings.HasPrefix(rightInner, "/") && !strings.HasPrefix(rightInner, "!")
			rightName := strings.ToLower(htmlTagName(rightInner))
			if leftClosing && rightOpening && blockElements[rightName] {
				result = append(result, line[start:i+1])
				start = i + 1
			}
		}
		pos = i + 2
	}
	result = append(result, line[start:])
	return result
}

func loneOpeningTag(line string) (prefix, name string, ok bool) {
	prefix = line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "<") || strings.HasPrefix(t, "</") {
		return prefix, "", false
	}
	end := scanHTMLTag(t, 0)
	if end != len(t) {
		return prefix, "", false
	}
	inner := strings.TrimSpace(t[1 : len(t)-1])
	name = strings.ToLower(htmlTagName(inner))
	if name == "" || voidElements[name] || strings.HasSuffix(inner, "/") {
		return prefix, "", false
	}
	return prefix, name, true
}

func loneClosingTag(line string) (name string, ok bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "</") {
		return "", false
	}
	end := scanHTMLTag(t, 0)
	if end != len(t) {
		return "", false
	}
	return strings.ToLower(htmlTagName(strings.TrimSpace(t[1 : len(t)-1]))), true
}
