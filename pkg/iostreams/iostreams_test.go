package iostreams

import (
	"bufio"
	"fmt"
	"os"
	"testing"
)

func TestStopAlternateScreenBuffer(t *testing.T) {
	ios, _, stdout, _ := Test()
	ios.SetAlternateScreenBufferEnabled(true)

	ios.StartAlternateScreenBuffer()
	fmt.Fprint(ios.Out, "test")
	ios.StopAlternateScreenBuffer()

	// Stopping a subsequent time should no-op.
	ios.StopAlternateScreenBuffer()

	const want = "\x1b[?1049htest\x1b[?1049l"
	if got := stdout.String(); got != want {
		t.Errorf("after IOStreams.StopAlternateScreenBuffer() got %q, want %q", got, want)
	}
}

func TestIOStreams_pager(t *testing.T) {
	t.Skip("TODO: fix this test in race detection mode")
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager(fmt.Sprintf("%s -test.run=TestHelperProcess --", os.Args[0]))
	t.Setenv("GH_WANT_HELPER_PROCESS", "1")
	if err := ios.StartPager(); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(ios.Out, "line1"); err != nil {
		t.Errorf("error writing line 1: %v", err)
	}
	if _, err := fmt.Fprintln(ios.Out, "line2"); err != nil {
		t.Errorf("error writing line 2: %v", err)
	}
	ios.StopPager()
	wants := "pager: line1\npager: line2\n"
	if got := stdout.String(); got != wants {
		t.Errorf("expected %q, got %q", wants, got)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Printf("pager: %s\n", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
}
