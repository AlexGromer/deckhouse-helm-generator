package merge

import (
	"strings"
	"testing"
)

func TestMerge3_Clean(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "line1\nours2\nline3\n"
	theirs := "line1\nline2\ntheirs3\n"

	res := Merge3(base, ours, theirs, Merge3Options{})

	if res.Conflict {
		t.Fatalf("expected no conflict, got %v", res.ConflictCount)
	}

	want := "line1\nours2\ntheirs3\n"
	if res.Content != want {
		t.Errorf("expected %q, got %q", want, res.Content)
	}

	// False conflict
	res = Merge3(base, "both_changed\n", "both_changed\n", Merge3Options{})
	if res.Conflict {
		t.Fatalf("expected no conflict, got %v", res.ConflictCount)
	}
	if res.Content != "both_changed\n" {
		t.Errorf("expected %q, got %q", "both_changed\n", res.Content)
	}
}

func TestMerge3_Conflict(t *testing.T) {
	base := "line1\nline2\nline3\n"
	ours := "line1\nours2\nline3\n"
	theirs := "line1\ntheirs2\nline3\n"

	res := Merge3(base, ours, theirs, Merge3Options{ShowBase: true})

	if !res.Conflict {
		t.Fatalf("expected conflict")
	}

	if res.ConflictCount != 1 {
		t.Errorf("expected 1 conflict, got %d", res.ConflictCount)
	}

	want := "line1\n<<<<<<< user\nours2\n||||||| base\nline2\n=======\ntheirs2\n>>>>>>> generated\nline3\n"
	if res.Content != want {
		t.Errorf("expected %q\n, got %q\n", want, res.Content)
	}

	resNoBase := Merge3(base, ours, theirs, Merge3Options{ShowBase: false})
	wantNoBase := "line1\n<<<<<<< user\nours2\n=======\ntheirs2\n>>>>>>> generated\nline3\n"
	if resNoBase.Content != wantNoBase {
		t.Errorf("expected %q\n, got %q\n", wantNoBase, resNoBase.Content)
	}
}

func TestMerge3_NoNewlineEOF(t *testing.T) {
	base := "line1\nline2"
	ours := "line1\nours2"
	theirs := "line1\ntheirs2"

	res := Merge3(base, ours, theirs, Merge3Options{})
	if !res.Conflict {
		t.Fatalf("expected conflict")
	}

	if !strings.Contains(res.Content, "<<<<<<< user\nours2\n=======\ntheirs2\n>>>>>>> generated") {
		t.Errorf("expected markers in %q", res.Content)
	}
}
