package formatter

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatChatTemplate(t *testing.T) {
	input := `{# chat #}
{%- if tools %}
{%- for tool in tools %}
{{- tool|tojson }}
{%- endfor %}
{%- else %}
{{- messages[0].content }}
{%- endif %}
`
	want := `{# chat #}
{%- if tools %}
  {%- for tool in tools %}
    {{- tool|tojson }}
  {%- endfor %}
{%- else %}
  {{- messages[0].content }}
{%- endif %}
`
	assertFormat(t, input, want)
}

func TestQuotedClosingDelimiter(t *testing.T) {
	input := `{{ "literal }} text" }}
{% if value == '%}' %}yes{% endif %}
`
	want := `{{ "literal }} text" }}
{% if value == '%}' %}yes{% endif %}
`
	assertFormat(t, input, want)
}

func TestHTMLAndMultilineExpression(t *testing.T) {
	input := `<div>
{{
      {
        'key': [
          1,
          2
        ]
      }
}}
</div>`
	want := `<div>
  {{
    {
      'key': [
        1,
        2
      ]
    }
  }}
</div>
`
	assertFormat(t, input, want)
}

func TestCustomAndCaptureBlocks(t *testing.T) {
	input := `{% dnd_area "main" %}
{% set captured %}
{{value}}
{% endset %}
{% end_dnd_area %}`
	want := `{% dnd_area "main" %}
  {% set captured %}
    {{ value }}
  {% endset %}
{% end_dnd_area %}
`
	assertFormat(t, input, want)
}

func TestRawAndRangeIgnoreAreOpaque(t *testing.T) {
	input := `{% raw %}
{{  this is not parsed  }}
{% endraw %}
{# prettier-ignore-start #}
{% malformed
{# prettier-ignore-end #}`
	assertFormat(t, input, input+"\n")
}

func TestSingleLineIgnore(t *testing.T) {
	input := `<div>
<!-- prettier-ignore -->
<span      class="x" > {{unformatted}}</span >
</div>`
	want := `<div>
  <!-- prettier-ignore -->
  <span      class="x" > {{unformatted}}</span >
</div>
`
	assertFormat(t, input, want)
}

func TestScriptAndStyleAreOpaque(t *testing.T) {
	input := `<html>
<script>
{{not_parsed}}
</script>
<style>
.x { color: {{value}}; }
</style>
</html>`
	want := `<html>
  <script>
    {{not_parsed}}
  </script>
  <style>
    .x { color: {{value}}; }
  </style>
</html>
`
	assertFormat(t, input, want)
}

func TestWhitespaceMarkers(t *testing.T) {
	input := `{%-if x+%}
{{-value-}}
{%-endif%}`
	want := `{%- if x +%}
  {{- value -}}
{%- endif %}
`
	assertFormat(t, input, want)
}

func TestStructuralErrors(t *testing.T) {
	tests := []string{
		"{% endif %}",
		"{% if x %}\n{% endfor %}",
		"{% if x %}",
	}
	for _, input := range tests {
		_, err := Format(input, DefaultOptions())
		var parseErr *ParseError
		if !errors.As(err, &parseErr) {
			t.Errorf("Format(%q) error = %v, want ParseError", input, err)
		}
	}
}

func TestCRLFAndBOM(t *testing.T) {
	input := "\ufeff{% if x %}\r\n{{x}}\r\n{% endif %}\r\n"
	want := "\ufeff{% if x %}\r\n  {{ x }}\r\n{% endif %}\r\n"
	assertFormat(t, input, want)
}

func TestIdempotence(t *testing.T) {
	inputs := []string{
		"<ul>\n{% for x in xs %}\n<li>{{x}}</li>\n{% else %}\n<li>none</li>\n{% endfor %}\n</ul>",
		"{% macro m(a, b=') }}') %}\n{{m(1)}}\n{% endmacro %}",
		"<div class=\"{% if x %}a{% else %}b{% endif %}\">{{x}}</div>",
	}
	for _, input := range inputs {
		once, err := Format(input, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		twice, err := Format(once, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if once != twice {
			t.Errorf("not idempotent:\n--- once\n%s--- twice\n%s", once, twice)
		}
	}
}

func TestUseTabs(t *testing.T) {
	got, err := Format("{% if x %}\n{{ x }}\n{% endif %}", Options{UseTabs: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\n\t{{ x }}\n") {
		t.Fatalf("expected tab indentation, got %q", got)
	}
}

func assertFormat(t *testing.T, input, want string) {
	t.Helper()
	got, err := Format(input, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("format mismatch\n--- got\n%s--- want\n%s", got, want)
	}
	again, err := Format(got, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("format is not idempotent\n--- once\n%s--- twice\n%s", got, again)
	}
}

func FuzzFormatIdempotent(f *testing.F) {
	for _, seed := range []string{
		"{{ value }}",
		"{% if x %}{{ x }}{% else %}no{% endif %}",
		"{% raw %}{{ untouched }}{% endraw %}",
		"<div class=\"{{ cls }}\">text</div>",
		"{# comment #}\n{% for x in xs %}\n{{- x -}}\n{% endfor %}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		once, err := Format(source, DefaultOptions())
		if err != nil {
			return
		}
		twice, err := Format(once, DefaultOptions())
		if err != nil {
			t.Fatalf("formatted output failed to parse: %v\n%s", err, once)
		}
		if once != twice {
			t.Fatalf("not idempotent\n--- once\n%s\n--- twice\n%s", once, twice)
		}
	})
}
