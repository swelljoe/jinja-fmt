package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/swelljoe/jinja-fmt/formatter"
)

var version = "0.1.0"

type config struct {
	write, check, showVersion bool
	indent, width             int
	useTabs                   bool
	eol                       string
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jinja-fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := config{}
	fs.BoolVar(&cfg.write, "write", false, "rewrite files in place")
	fs.BoolVar(&cfg.write, "w", false, "rewrite files in place (shorthand)")
	fs.BoolVar(&cfg.check, "check", false, "check whether files are formatted")
	fs.BoolVar(&cfg.check, "c", false, "check whether files are formatted (shorthand)")
	fs.IntVar(&cfg.indent, "indent-width", 2, "spaces per indentation level")
	fs.IntVar(&cfg.width, "print-width", 80, "preferred maximum line width")
	fs.BoolVar(&cfg.useTabs, "use-tabs", false, "indent with tabs")
	fs.StringVar(&cfg.eol, "end-of-line", "auto", "line endings: auto, lf, or crlf")
	fs.BoolVar(&cfg.showVersion, "version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: jinja-fmt [options] [file or directory ...]")
		fmt.Fprintln(stderr, "With no paths, reads stdin and writes formatted text to stdout.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if cfg.showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if cfg.write && cfg.check {
		fmt.Fprintln(stderr, "jinja-fmt: --write and --check cannot be combined")
		return 2
	}
	if cfg.indent < 1 || cfg.width < 1 || (cfg.eol != "auto" && cfg.eol != "lf" && cfg.eol != "crlf") {
		fmt.Fprintln(stderr, "jinja-fmt: invalid formatting option")
		return 2
	}

	opts := formatter.Options{IndentWidth: cfg.indent, PrintWidth: cfg.width, UseTabs: cfg.useTabs, EndOfLine: cfg.eol}
	paths, err := collectPaths(fs.Args())
	if err != nil {
		fmt.Fprintf(stderr, "jinja-fmt: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		if len(fs.Args()) > 0 {
			return 0
		}
		if cfg.write || cfg.check {
			fmt.Fprintln(stderr, "jinja-fmt: --write/--check requires at least one path")
			return 2
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "jinja-fmt: stdin: %v\n", err)
			return 2
		}
		formatted, err := formatter.Format(string(data), opts)
		if err != nil {
			fmt.Fprintf(stderr, "jinja-fmt: stdin:%v\n", err)
			return 2
		}
		if _, err := io.WriteString(stdout, formatted); err != nil {
			return 2
		}
		return 0
	}

	changed, failed := 0, false
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "jinja-fmt: %s: %v\n", path, err)
			failed = true
			continue
		}
		formatted, err := formatter.Format(string(data), opts)
		if err != nil {
			fmt.Fprintf(stderr, "jinja-fmt: %s:%v\n", path, err)
			failed = true
			continue
		}
		if bytes.Equal(data, []byte(formatted)) {
			if cfg.check {
				fmt.Fprintf(stdout, "%s\n", path)
			}
			continue
		}
		changed++
		switch {
		case cfg.write:
			if err := replaceFile(path, []byte(formatted)); err != nil {
				fmt.Fprintf(stderr, "jinja-fmt: %s: %v\n", path, err)
				failed = true
			}
		case cfg.check:
			fmt.Fprintf(stdout, "%s\n", path)
		default:
			if len(paths) > 1 {
				fmt.Fprintf(stdout, "==> %s <==\n", path)
			}
			io.WriteString(stdout, formatted)
		}
	}
	if failed {
		return 2
	}
	if cfg.check && changed > 0 {
		fmt.Fprintf(stderr, "jinja-fmt: %d file(s) would be reformatted\n", changed)
		return 1
	}
	return 0
}

func collectPaths(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			paths = append(paths, arg)
			continue
		}
		err = filepath.WalkDir(arg, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path != arg && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".jinja", ".jinja2", ".j2", ".html", ".htm":
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func replaceFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	// Windows does not provide Unix rename-over-an-existing-file semantics.
	// A direct truncate/write preserves the path and is the portable fallback.
	if runtime.GOOS == "windows" {
		return os.WriteFile(path, data, info.Mode().Perm())
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".jinja-fmt-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(info.Mode().Perm()); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err == nil {
		ok = true
	}
	return err
}
