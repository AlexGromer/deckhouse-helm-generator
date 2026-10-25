package merge

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestSplitLines(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\n", []string{"a\n"}},
		{"a\nb", []string{"a\n", "b"}},
		{"a\r\nb", []string{"a\r\n", "b"}},
		{"\n\n", []string{"\n", "\n"}},
	}

	for _, tc := range cases {
		got := SplitLines(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDiff_Simple(t *testing.T) {
	cases := []struct {
		a, b []string
		want []OpKind
	}{
		{[]string{}, []string{}, nil},
		{[]string{"x"}, []string{"x"}, []OpKind{OpEqual}},
		{[]string{"a", "b", "c"}, []string{"x", "a", "b", "c"}, []OpKind{OpInsert, OpEqual, OpEqual, OpEqual}},
		{[]string{"a", "b", "c", "a", "b", "b", "a"}, []string{"c", "b", "a", "b", "a", "c"}, []OpKind{OpDelete, OpDelete, OpEqual, OpInsert, OpEqual, OpEqual, OpDelete, OpEqual, OpInsert}},
	}

	for _, tc := range cases {
		edits := Diff(tc.a, tc.b)
		var got []OpKind
		for _, e := range edits {
			got = append(got, e.Kind)
		}

		if tc.want == nil && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Diff(%v, %v) kinds = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDiff_LCSProperty(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	vocab := []string{"A", "B", "C", "D", "E"}

	for i := 0; i < 200; i++ {
		lenA := r.Intn(20)
		lenB := r.Intn(20)

		a := make([]string, lenA)
		for j := 0; j < lenA; j++ {
			a[j] = vocab[r.Intn(len(vocab))]
		}

		b := make([]string, lenB)
		for j := 0; j < lenB; j++ {
			b[j] = vocab[r.Intn(len(vocab))]
		}

		edits := Diff(a, b)

		// 1. Reconstruct b from a using edits
		var reconstructed []string
		equals := 0
		for _, e := range edits {
			switch e.Kind {
			case OpEqual:
				reconstructed = append(reconstructed, a[e.A])
				equals++
			case OpInsert:
				reconstructed = append(reconstructed, b[e.B])
			}
		}

		if !reflect.DeepEqual(reconstructed, b) && len(b) > 0 {
			t.Errorf("reconstructed %v != original b %v", reconstructed, b)
		}

		// 2. Check LCS length
		lcs := naiveLCS(a, b)
		if equals != lcs {
			t.Errorf("equals %d != LCS %d for a=%v b=%v", equals, lcs, a, b)
		}
	}
}

func naiveLCS(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				x, y := dp[i-1][j], dp[i][j-1]
				if x > y {
					dp[i][j] = x
				} else {
					dp[i][j] = y
				}
			}
		}
	}
	return dp[len(a)][len(b)]
}

func TestUnified(t *testing.T) {
	// Insertion at start
	a := []string{"1\n", "2\n", "3\n", "4\n", "5\n", "6\n", "7\n", "8\n", "9\n", "10\n"}
	b := []string{"new\n", "1\n", "2\n", "3\n", "4\n", "5\n", "6\n", "7\n", "8\n", "9\n", "10\n"}

	diff := Unified("a.txt", "b.txt", a, b, 3)

	want := `--- a.txt
+++ b.txt
@@ -1,3 +1,4 @@
+new
 1
 2
 3
`
	if diff != want {
		t.Errorf("Unified diff wrong:\nGot:\n%s\nWant:\n%s", diff, want)
	}

	// No newline at end of file
	a = []string{"1\n", "2"}
	b = []string{"1\n", "3"}
	diff = Unified("a.txt", "b.txt", a, b, 3)
	want2 := `--- a.txt
+++ b.txt
@@ -1,2 +1,2 @@
 1
-2
\ No newline at end of file
+3
\ No newline at end of file
`
	if diff != want2 {
		t.Errorf("Unified diff wrong for EOF newline:\nGot:\n%s\nWant:\n%s", diff, want2)
	}
}

// Deadcode needs all internal unexported things covered, or use them in exported functions which are covered.
// Unified covers newHunk and checkNewline.
