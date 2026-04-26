package install

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm_Yes(t *testing.T) {
	opts := Options{
		Stdin:  strings.NewReader("y\n"),
		Stdout: &bytes.Buffer{},
	}
	if !confirm(opts, "Do it? [Y/n] ") {
		t.Fatal("expected true for 'y' input")
	}
}

func TestConfirm_No(t *testing.T) {
	opts := Options{
		Stdin:  strings.NewReader("n\n"),
		Stdout: &bytes.Buffer{},
	}
	if confirm(opts, "Do it? [Y/n] ") {
		t.Fatal("expected false for 'n' input")
	}
}

func TestConfirm_EmptyDefaultYes(t *testing.T) {
	opts := Options{
		Stdin:  strings.NewReader("\n"),
		Stdout: &bytes.Buffer{},
	}
	if !confirm(opts, "Do it? [Y/n] ") {
		t.Fatal("expected true for empty input (default yes)")
	}
}

func TestConfirm_Force(t *testing.T) {
	opts := Options{
		Force: true,
		// no Stdin/Stdout needed
	}
	if !confirm(opts, "Do it? [Y/n] ") {
		t.Fatal("expected true when Force is set")
	}
}
