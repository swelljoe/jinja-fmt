# jinja-fmt

`jinja-fmt` is a small, native formatter for Jinja templates. It has no runtime
dependencies, no JavaScript bundle, and no required configuration. The same
source builds for Linux, macOS, and Windows.

The formatter is aimed especially at model chat templates used by llama.cpp,
Transformers, and related tools, while also supporting HTML/Jinja templates.

```console
$ jinja-fmt < chat-template.jinja
$ jinja-fmt --write templates/
$ jinja-fmt --check templates/
```

## Status

The parser handles all Jinja tag forms, whitespace-control markers, nested
standard blocks, capture `set`, extension/custom blocks with matching `end...`
tags, quoted delimiters, multiline tags, raw regions, comments, and Prettier
ignore directives. Jinja expression text is kept intact except for delimiter
spacing and multiline indentation; the formatter does not need to evaluate or
type-check expressions.

Against `prettier-plugin-jinja-template` 2.2.0, it matches the real 129-line
chat-template fixture byte for byte and matches 30 of 35 non-error upstream
golden fixtures. Intentional differences are documented in
[docs/compatibility.md](docs/compatibility.md).

## Install

Build with Go 1.23 or newer:

```console
go install github.com/swelljoe/jinja-fmt/cmd/jinja-fmt@latest
```

From a source checkout:

```console
make build
./bin/jinja-fmt --version
```

The resulting stripped executable is about 2 MB on Linux amd64. Cross-compile
without containers or a C toolchain:

```console
make cross
```

This creates binaries for Linux, macOS, and Windows on amd64 and arm64.

## Usage

```text
Usage: jinja-fmt [options] [file or directory ...]
```

With no path, input is read from stdin and formatted output is written to
stdout. Files passed without `--write` are formatted to stdout. Directories are
walked recursively for `.jinja`, `.jinja2`, `.j2`, `.html`, and `.htm` files;
`.git`, `node_modules`, and `vendor` are skipped.

- `-w`, `--write`: rewrite files in place
- `-c`, `--check`: exit 1 when any file needs formatting
- `--indent-width N`: spaces per level (default 2)
- `--use-tabs`: use one tab per level
- `--print-width N`: preferred line width (default 80)
- `--end-of-line auto|lf|crlf`: line-ending policy (default preserves CRLF)
- `--version`: print the version

Exit status is 0 for success, 1 for a failed formatting check, and 2 for input,
parse, or I/O errors.

## Whitespace semantics

Whitespace is data in Jinja. Like Prettier's Jinja plugin, this tool changes
source indentation and blank lines, which can change rendered output when tags
do not use whitespace control. `-` and `+` markers are always preserved. Review
the first formatting diff and use `{%-`, `-%}`, or your environment's
`trim_blocks`/`lstrip_blocks` settings where output whitespace matters.

## Development

```console
make test       # unit tests, race detector, vet, and diff checks
make build      # bin/jinja-fmt
make cross      # six release targets
```

The implementation uses only the Go standard library.
