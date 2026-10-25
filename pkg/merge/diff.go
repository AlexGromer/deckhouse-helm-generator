package merge

import (
	"fmt"
	"strings"
)

// SplitLines splits a string into lines, preserving '\n' at the end of each line
// and keeping '\r' together with '\n' when applicable.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// OpKind represents the type of an edit operation.
type OpKind int

const (
	OpEqual OpKind = iota
	OpDelete
	OpInsert
)

// Edit represents a single diff operation.
// A, B are indices in a and b respectively. They are -1 if the operation doesn't apply.
type Edit struct {
	Kind OpKind
	A, B int
}

// Diff returns the shortest edit script turning a into b using Myers 1986 algorithm.
func Diff(a, b []string) []Edit {
	n, m := len(a), len(b)
	max := n + m
	if max == 0 {
		return nil
	}

	v := make([]int, 2*max+1)
	trace := make([][]int, 0, max+1)
	offset := max
	v[1+offset] = 0

	for d := 0; d <= max; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])
			if down {
				x = v[k+1+offset]
			} else {
				x = v[k-1+offset] + 1
			}

			y := x - k

			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[k+offset] = x

			if x >= n && y >= m {
				t := make([]int, len(v))
				copy(t, v)
				trace = append(trace, t)
				return backtrack(trace, a, b)
			}
		}
		t := make([]int, len(v))
		copy(t, v)
		trace = append(trace, t)
	}
	return nil
}

func backtrack(trace [][]int, a, b []string) []Edit {
	var script []Edit
	x, y := len(a), len(b)
	max := len(a) + len(b)
	offset := max

	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y

		down := k == -d || (k != d && v[k-1+offset] < v[k+1+offset])
		var prevK int
		if down {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := trace[d-1][prevK+offset]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			script = append(script, Edit{Kind: OpEqual, A: x, B: y})
		}

		if d > 0 {
			if down {
				y--
				script = append(script, Edit{Kind: OpInsert, A: x, B: y})
			} else {
				x--
				script = append(script, Edit{Kind: OpDelete, A: x, B: y})
			}
		}
	}

	for x > 0 && y > 0 {
		x--
		y--
		script = append(script, Edit{Kind: OpEqual, A: x, B: y})
	}

	reverse(script)
	return groupEdits(script)
}

func reverse(s []Edit) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func groupEdits(script []Edit) []Edit {
	var grouped []Edit
	n := len(script)
	for i := 0; i < n; {
		if script[i].Kind == OpEqual {
			grouped = append(grouped, script[i])
			i++
			continue
		}

		start := i
		for i < n && script[i].Kind != OpEqual {
			i++
		}

		var dels, ins []Edit
		for j := start; j < i; j++ {
			if script[j].Kind == OpDelete {
				dels = append(dels, script[j])
			} else {
				ins = append(ins, script[j])
			}
		}
		grouped = append(grouped, dels...)
		grouped = append(grouped, ins...)
	}
	return grouped
}

func Unified(aName, bName string, a, b []string, contextLines int) string {
	script := Diff(a, b)
	if len(script) == 0 || (len(script) == len(a) && script[0].Kind == OpEqual && script[len(script)-1].Kind == OpEqual) {
		isAllEqual := true
		for _, e := range script {
			if e.Kind != OpEqual {
				isAllEqual = false
				break
			}
		}
		if isAllEqual {
			return ""
		}
	}

	var hunks []hunk
	var curr *hunk

	for i := 0; i < len(script); i++ {
		e := script[i]
		if e.Kind == OpEqual {
			continue
		}

		start := i
		for i < len(script) && script[i].Kind != OpEqual {
			i++
		}
		end := i

		h := newHunk(a, b, script, start, end, contextLines)

		if curr == nil {
			curr = &h
		} else {
			if h.aStart-curr.aEnd <= 2*contextLines {
				curr.aEnd = h.aEnd
				curr.bEnd = h.bEnd
				curr.edits = append(curr.edits, script[curr.scriptEnd:h.scriptEnd]...)
				curr.scriptEnd = h.scriptEnd
			} else {
				hunks = append(hunks, *curr)
				curr = &h
			}
		}
		i--
	}

	if curr != nil {
		hunks = append(hunks, *curr)
	}

	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aName, bName)

	for _, h := range hunks {
		aLen := h.aEnd - h.aStart
		bLen := h.bEnd - h.bStart
		aStartHuman := h.aStart + 1
		bStartHuman := h.bStart + 1
		if aLen == 0 {
			aStartHuman = h.aStart
		}
		if bLen == 0 {
			bStartHuman = h.bStart
		}

		aRange := fmt.Sprintf("%d", aStartHuman)
		if aLen != 1 {
			aRange += fmt.Sprintf(",%d", aLen)
		}
		bRange := fmt.Sprintf("%d", bStartHuman)
		if bLen != 1 {
			bRange += fmt.Sprintf(",%d", bLen)
		}

		fmt.Fprintf(&sb, "@@ -%s +%s @@\n", aRange, bRange)

		for _, e := range h.edits {
			switch e.Kind {
			case OpEqual:
				fmt.Fprintf(&sb, " %s", a[e.A])
				checkNewline(&sb, a[e.A])
			case OpDelete:
				fmt.Fprintf(&sb, "-%s", a[e.A])
				checkNewline(&sb, a[e.A])
			case OpInsert:
				fmt.Fprintf(&sb, "+%s", b[e.B])
				checkNewline(&sb, b[e.B])
			}
		}
	}

	return sb.String()
}

type hunk struct {
	aStart, aEnd, bStart, bEnd int
	edits                      []Edit
	scriptStart, scriptEnd     int
}

func newHunk(a, b []string, script []Edit, start, end, ctx int) hunk {
	s := start
	for c := 0; c < ctx && s > 0 && script[s-1].Kind == OpEqual; c, s = c+1, s-1 {
	}

	e := end
	for c := 0; c < ctx && e < len(script) && script[e].Kind == OpEqual; c, e = c+1, e+1 {
	}

	var aStart, bStart int
	if s < len(script) && script[s].Kind != OpInsert {
		aStart = script[s].A
	} else if s < len(script) && script[s].Kind == OpInsert {
		if s > 0 {
			aStart = script[s-1].A + 1
		} else {
			aStart = 0
		}
	}

	if s < len(script) && script[s].Kind != OpDelete {
		bStart = script[s].B
	} else if s < len(script) && script[s].Kind == OpDelete {
		if s > 0 {
			bStart = script[s-1].B + 1
		} else {
			bStart = 0
		}
	}
	if aStart == -1 {
		aStart = 0
	}
	if bStart == -1 {
		bStart = 0
	}

	aEnd, bEnd := aStart, bStart
	for i := s; i < e; i++ {
		if script[i].Kind == OpEqual || script[i].Kind == OpDelete {
			aEnd++
		}
		if script[i].Kind == OpEqual || script[i].Kind == OpInsert {
			bEnd++
		}
	}

	edits := make([]Edit, e-s)
	copy(edits, script[s:e])

	return hunk{
		aStart:      aStart,
		aEnd:        aEnd,
		bStart:      bStart,
		bEnd:        bEnd,
		edits:       edits,
		scriptStart: s,
		scriptEnd:   e,
	}
}

func checkNewline(sb *strings.Builder, s string) {
	if !strings.HasSuffix(s, "\n") {
		sb.WriteString("\n\\ No newline at end of file\n")
	}
}
