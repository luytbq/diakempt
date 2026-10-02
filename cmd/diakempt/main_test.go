package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyCase(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../corpus/cases", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestDefaultOutputGoesNextToInput(t *testing.T) {
	dir := t.TempDir()
	in := copyCase(t, dir, "basic.drawio")
	code, _, stderr := runCLI(in)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := filepath.Join(dir, "basic.tidy.drawio")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("no output at %s: %s", want, stderr)
	}
	if !strings.Contains(stderr, "1 page") || !strings.Contains(stderr, "wrote "+want) {
		t.Errorf("report: %s", stderr)
	}
	code, _, stderr = runCLI(want)
	if code != exitOK || !strings.Contains(stderr, "wrote "+want+" (replaced the existing file)") {
		t.Errorf("rerun on the output: exit %d, %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "basic.tidy.tidy.drawio")); err == nil {
		t.Error("suffix stacked")
	}
}

func TestTidyPath(t *testing.T) {
	for in, want := range map[string]string{
		"a/diagram.drawio":    "a/diagram.tidy.drawio",
		"diagram.xml":         "diagram.tidy.xml",
		"my.flow.drawio":      "my.flow.tidy.drawio",
		"diagram.tidy.drawio": "diagram.tidy.drawio",
		"noext":               "noext.tidy",
	} {
		if got := tidyPath(in); got != want {
			t.Errorf("tidyPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBackupNamesAreNumbered(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "d.drawio")
	var got []string
	for i := 0; i < 3; i++ {
		b, err := backup(name, []byte{byte('a' + i)})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.Base(b))
	}
	if strings.Join(got, ",") != "d.drawio.bak,d.drawio.bak.1,d.drawio.bak.2" {
		t.Errorf("backups = %v", got)
	}
	if data, _ := os.ReadFile(name + ".bak"); string(data) != "a" {
		t.Error("the first backup was overwritten")
	}
}

func TestInPlaceLeavesUnchangedFileAlone(t *testing.T) {
	dir := t.TempDir()
	in := copyCase(t, dir, "basic.drawio")
	code, _, stderr := runCLI("--in-place", in)
	if code != exitOK || !strings.Contains(stderr, "unchanged") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(in + ".bak"); err == nil {
		t.Error("a backup was made for an unchanged file")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := copyCase(t, dir, "basic.drawio")
	if code, _, _ := runCLI(in, "--dry-run"); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("files after dry run: %d", len(entries))
	}
}

func TestStdoutOutput(t *testing.T) {
	dir := t.TempDir()
	in := copyCase(t, dir, "basic.drawio")
	code, stdout, _ := runCLI("-o", "-", in)
	if code != exitOK || !strings.HasPrefix(stdout, "<mxfile") {
		t.Errorf("exit %d, stdout %.40q", code, stdout)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--level", "wild", "x.drawio"},
		{"--type", "mindmap", "x.drawio"},
		{"-o", "out.drawio", "a.drawio", "b.drawio"},
		{"-o", "out.drawio", "--in-place", "a.drawio"},
		{"-o", "-", "--json", "a.drawio"},
		{"--with-align", "--no-align", "a.drawio"},
		{"--snap-ratio", "3", "a.drawio"},
		{"--bogus", "a.drawio"},
	} {
		if code, _, _ := runCLI(args...); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
	}
}

func TestBadFilesExitOneAndOthersStillRun(t *testing.T) {
	dir := t.TempDir()
	good := copyCase(t, dir, "basic.drawio")
	embedded := copyCase(t, dir, "embedded.drawio.svg")
	code, stdout, _ := runCLI("--json", embedded, filepath.Join(dir, "missing.drawio"), good)
	if code != exitFile {
		t.Fatalf("exit %d", code)
	}
	var rep struct {
		Files []struct {
			File   string
			Output string
			Error  *struct{ Code string }
		}
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Files) != 3 || rep.Files[0].Error.Code != "input.embedded" ||
		rep.Files[1].Error.Code != "io.read" || rep.Files[2].Error != nil || rep.Files[2].Output == "" {
		t.Errorf("report: %s", stdout)
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := runCLI("--version")
	if code != exitOK || !strings.HasPrefix(stdout, "diakempt ") {
		t.Errorf("exit %d, %q", code, stdout)
	}
}
