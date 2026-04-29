package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestBdyNDInitAddCommitLog(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"nd", "init"}, &out, &errOut); code != 0 {
		t.Fatalf("init code=%d err=%s", code, errOut.String())
	}
	if err := os.WriteFile("a.txt", []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "add", "a.txt"}, &out, &errOut); code != 0 {
		t.Fatalf("add code=%d err=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "commit", "-m", "first"}, &out, &errOut); code != 0 {
		t.Fatalf("commit code=%d err=%s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"nd", "log"}, &out, &errOut); code != 0 {
		t.Fatalf("log code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "first") {
		t.Fatalf("log=%q", out.String())
	}
}
