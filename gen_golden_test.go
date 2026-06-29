package format

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

// TestGenGolden writes golden.go from the live MRI oracle. Run with
// `go test -run TestGenGolden -tags gengolden` only when regenerating; it is
// guarded by an env var so the normal suite never triggers it.
func TestGenGolden(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") == "" {
		t.Skip("set GEN_GOLDEN=1 to regenerate golden.go")
	}
	if !rubyAvailable() {
		t.Fatal("ruby required to regenerate golden")
	}
	f, err := os.Create("golden.go")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fmt.Fprintln(f, "// Code generated from the MRI oracle; DO NOT EDIT.")
	fmt.Fprintln(f, "// Regenerate with: GEN_GOLDEN=1 go test -run TestGenGolden")
	fmt.Fprintln(f, "package format")
	fmt.Fprintln(f)
	fmt.Fprintln(f, "// goldenWant is the MRI-produced output for corpus[i].")
	fmt.Fprintln(f, "var goldenWant = []string{")
	for _, tc := range corpus {
		errTag, out := mriResult(t, tc.format, tc.args)
		if errTag != "" {
			t.Fatalf("corpus case %q unexpectedly errored: %s", tc.format, errTag)
		}
		fmt.Fprintf(f, "\t%s,\n", strconv.Quote(out))
	}
	fmt.Fprintln(f, "}")
}
