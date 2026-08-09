# Formatting and compatibility contract

This document defines what `jinja-fmt` recognizes and which behavior is meant
to be stable.

## Jinja syntax

The lexer recognizes:

- expressions: `{{ ... }}`
- statements: `{% ... %}`
- comments: `{# ... #}`
- every `-` and `+` whitespace-control delimiter combination
- single- and double-quoted strings with backslash escapes
- multiline expressions and statements
- raw blocks, whose contents are never interpreted

Closing delimiters inside quoted strings do not terminate a tag. Expression
and statement bodies are accepted without a hard-coded expression grammar, so
filters, tests, slicing, calls, mappings, tuples, extension operators, and
implementation-specific syntax remain usable. Only outer whitespace and
multiline indentation are changed; tokens and quote style inside an expression
are preserved.

## Block structure

Standard blocks (`if`, `for`, `block`, `macro`, `call`, `filter`, `with`,
`autoescape`, `trans`, and capture `set`) are nested and validated. `else`,
`elif`, and `pluralize` are branches. Underscored closing forms such as
`end_dnd_area` are supported.

An unknown statement becomes a block when the same document contains a
matching `endNAME` or `end_NAME`. This supports Jinja extensions without a
registry. Neutral statements such as `include`, `extends`, `import`, `from`,
`do`, assignment `set`, `break`, and `continue` do not alter indentation.

Unlike `prettier-plugin-jinja-template` 2.2.0, unmatched, crossed, and unclosed
Jinja blocks are errors. A formatter should not silently bless a structurally
invalid template.

## HTML behavior

HTML element nesting contributes indentation, common void elements are printed
with ` />`, tag-internal whitespace is normalized, short text/expression-only
elements are kept on one line, and long simple elements are expanded. This is
a lightweight layout engine, not Prettier's complete HTML/CSS/JavaScript stack.

`script` and `style` elements are opaque. This avoids corrupting embedded Jinja
or treating a chat-template string as JavaScript. Their contents are not run
through a JS or CSS formatter.

## Ignore and preservation regions

The following are supported:

```jinja
{# prettier-ignore-start #}
...preserved exactly...
{# prettier-ignore-end #}
```

```html
<!-- prettier-ignore-start -->
...preserved exactly...
<!-- prettier-ignore-end -->
```

`<!-- prettier-ignore -->` preserves the following physical line. Raw blocks,
HTML comments, script/style regions, and range-ignore contents are opaque to
the Jinja parser.

## Differences from the npm plugin

The compatibility baseline is `prettier-plugin-jinja-template` 2.2.0 with
Prettier 3.9.6. The native formatter intentionally differs in these areas:

- invalid or unclosed Jinja blocks are rejected;
- embedded JavaScript and CSS are preserved instead of formatted;
- HTML layout is close but is not promised to be byte-identical for complex
  optional-end-tag, table-repair, or whitespace-sensitive inline markup;
- Prettier's unrelated options and parsers are not exposed.

For the primary chat-template use case, formatting is byte-identical to the
reference fixture used during development and idempotence is tested natively.

