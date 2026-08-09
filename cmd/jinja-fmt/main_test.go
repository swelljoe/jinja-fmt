package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, bytes.NewBufferString("{% if x %}\n{{x}}\n{% endif %}"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if want := "{% if x %}\n  {{ x }}\n{% endif %}\n"; stdout.String() != want {
		t.Fatalf("got %q, want %q", stdout.String(), want)
	}
}

func TestCheckAndWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jinja")
	if err := os.WriteFile(path, []byte("{{x}}"), 0o640); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--check", path}, bytes.NewReader(nil), &stdout, &stderr); code != 1 {
		t.Fatalf("check code=%d, want 1", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--write", path}, bytes.NewReader(nil), &stdout, &stderr); code != 0 {
		t.Fatalf("write code=%d stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{{ x }}\n" {
		t.Fatalf("written data = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestInvalidOptions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--end-of-line", "native"}, bytes.NewReader(nil), &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
}

func TestEmptyDirectory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{t.TempDir()}, bytes.NewReader(nil), &stdout, &stderr); code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
