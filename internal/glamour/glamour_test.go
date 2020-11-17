package glamour

import (
	"bytes"
	"io/ioutil"
	"testing"
)

const (
	generate = false
	markdown = "markdown_test.in"
	rendered = "markdown_test.out"
)

func TestTermRendererWriter(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle("dark"),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := ioutil.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Write(in)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Close()
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	// generate
	if generate {
		err := ioutil.WriteFile(rendered, b, 0644)
		if err != nil {
			t.Fatal(err)
		}
		return
	}

	// verify
	td, err := ioutil.ReadFile(rendered)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(td, b) {
		t.Errorf("Rendered output doesn't match!\nExpected: `\n%s`\nGot: `\n%s`\n",
			string(td), b)
	}
}

func TestTermRenderer(t *testing.T) {
	r, err := NewTermRenderer(
		WithStandardStyle("dark"),
	)
	if err != nil {
		t.Fatal(err)
	}

	in, err := ioutil.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	b, err := r.Render(string(in))
	if err != nil {
		t.Fatal(err)
	}

	// verify
	td, err := ioutil.ReadFile(rendered)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(td, []byte(b)) {
		t.Errorf("Rendered output doesn't match!\nExpected: `\n%s`\nGot: `\n%s`\n",
			string(td), b)
	}
}

func TestStyles(t *testing.T) {
	_, err := NewTermRenderer(
		WithAutoStyle(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTermRenderer(
		WithStandardStyle("auto"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTermRenderer(
		WithEnvironmentConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRenderHelpers(t *testing.T) {
	in, err := ioutil.ReadFile(markdown)
	if err != nil {
		t.Fatal(err)
	}

	b, err := Render(string(in), "dark")
	if err != nil {
		t.Error(err)
	}

	// verify
	td, err := ioutil.ReadFile(rendered)
	if err != nil {
		t.Fatal(err)
	}

	if b != string(td) {
		t.Errorf("Rendered output doesn't match!\nExpected: `\n%s`\nGot: `\n%s`\n",
			string(td), b)
	}
}
