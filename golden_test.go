package format

import "testing"

// TestGolden asserts every corpus case against the MRI-captured golden output
// in golden.go. It needs no ruby, so the qemu and Windows CI lanes exercise the
// full conversion surface and the 100% coverage gate holds without an oracle.
func TestGolden(t *testing.T) {
	if len(goldenWant) != len(corpus) {
		t.Fatalf("golden/corpus length mismatch: %d vs %d (regenerate golden.go)",
			len(goldenWant), len(corpus))
	}
	for i, tc := range corpus {
		got, err := Sprintf(tc.format, tc.args...)
		if err != nil {
			t.Errorf("case %q args=%v: unexpected error %v", tc.format, tc.args, err)
			continue
		}
		if got != goldenWant[i] {
			t.Errorf("case %q args=%v\n got=%q\nwant=%q", tc.format, tc.args, got, goldenWant[i])
		}
	}
}

// TestGoldenErrors asserts the error cases against their recorded MRI exception
// class and message, again without ruby.
func TestGoldenErrors(t *testing.T) {
	for _, tc := range errCorpus {
		_, err := Sprintf(tc.format, tc.args...)
		if err == nil {
			t.Errorf("case %q args=%v: expected error, got nil", tc.format, tc.args)
			continue
		}
		fe, ok := err.(*Error)
		if !ok {
			t.Errorf("case %q: error is %T, want *Error", tc.format, err)
			continue
		}
		if fe.Class != tc.class || fe.Message != tc.msg {
			t.Errorf("case %q args=%v\n got=%s: %s\nwant=%s: %s",
				tc.format, tc.args, fe.Class, fe.Message, tc.class, tc.msg)
		}
	}
}
