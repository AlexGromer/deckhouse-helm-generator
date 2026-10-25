package merge

import (
	"strings"
)

// Merge3Options specifies options for a 3-way merge.
type Merge3Options struct {
	ShowBase bool
}

// ConflictHunk represents a conflict in a 3-way merge.
type ConflictHunk struct {
	Base, Ours, Theirs []string
}

// MergeResult represents the result of a 3-way text or structural merge.
type MergeResult struct {
	Content       string
	Conflict      bool
	ConflictCount int
	Conflicts     []ConflictHunk
}

// Merge3 merges ours and theirs against base line by line (diff3).
func Merge3(base, ours, theirs string, opts Merge3Options) MergeResult {
	if ours == theirs {
		return MergeResult{Content: ours, Conflict: false, ConflictCount: 0}
	}

	bl := SplitLines(base)
	ol := SplitLines(ours)
	tl := SplitLines(theirs)

	editsOurs := Diff(bl, ol)
	editsTheirs := Diff(bl, tl)

	ma := make([]int, len(bl))
	for i := range ma {
		ma[i] = -1
	}
	for _, e := range editsOurs {
		if e.Kind == OpEqual {
			ma[e.A] = e.B
		}
	}

	mb := make([]int, len(bl))
	for i := range mb {
		mb[i] = -1
	}
	for _, e := range editsTheirs {
		if e.Kind == OpEqual {
			mb[e.A] = e.B
		}
	}

	var sb strings.Builder
	conflicts := 0
	var conflictHunks []ConflictHunk

	lo, la, lb := -1, -1, -1

	for {
		i := 1
		for lo+i < len(bl) {
			aIdx := ma[lo+i]
			bIdx := mb[lo+i]
			if aIdx != -1 && bIdx != -1 && aIdx == la+i && bIdx == lb+i {
				i++
			} else {
				break
			}
		}

		if lo+i >= len(bl) && la+i >= len(ol) && lb+i >= len(tl) {
			for k := 1; k < i; k++ {
				sb.WriteString(bl[lo+k])
			}
			break
		}

		if i > 1 {
			for k := 1; k < i; k++ {
				sb.WriteString(bl[lo+k])
			}
			lo += i - 1
			la += i - 1
			lb += i - 1
			continue
		}

		j := lo + 1
		var a, b int
		found := false
		for j < len(bl) {
			a = ma[j]
			b = mb[j]
			if a > la && b > lb {
				found = true
				break
			}
			j++
		}

		if !found {
			j = len(bl)
			a = len(ol)
			b = len(tl)
		}

		bBlock := bl[lo+1 : j]
		oBlock := ol[la+1 : a]
		tBlock := tl[lb+1 : b]

		res := resolveUnstable(bBlock, oBlock, tBlock)
		if res.conflict {
			conflicts++
			conflictHunks = append(conflictHunks, ConflictHunk{Base: bBlock, Ours: oBlock, Theirs: tBlock})
			sb.WriteString("<<<<<<< user\n")
			for _, l := range oBlock {
				sb.WriteString(l)
				checkNewline3(&sb, l)
			}
			if opts.ShowBase {
				sb.WriteString("||||||| base\n")
				for _, l := range bBlock {
					sb.WriteString(l)
					checkNewline3(&sb, l)
				}
			}
			sb.WriteString("=======\n")
			for _, l := range tBlock {
				sb.WriteString(l)
				checkNewline3(&sb, l)
			}
			sb.WriteString(">>>>>>> generated\n")
		} else {
			for _, l := range res.lines {
				sb.WriteString(l)
			}
		}

		lo = j - 1
		la = a - 1
		lb = b - 1
	}

	return MergeResult{
		Content:       sb.String(),
		Conflict:      conflicts > 0,
		ConflictCount: conflicts,
		Conflicts:     conflictHunks,
	}
}

type resolveResult struct {
	lines    []string
	conflict bool
}

func resolveUnstable(base, ours, theirs []string) resolveResult {
	if stringSlicesEqual(ours, base) {
		return resolveResult{lines: theirs, conflict: false}
	}
	if stringSlicesEqual(theirs, base) {
		return resolveResult{lines: ours, conflict: false}
	}
	if stringSlicesEqual(ours, theirs) {
		return resolveResult{lines: ours, conflict: false}
	}

	eOurs := Diff(base, ours)
	eTheirs := Diff(base, theirs)

	changedBaseOurs := make([]bool, len(base))
	for _, e := range eOurs {
		if e.Kind == OpDelete && e.A >= 0 && e.A < len(base) {
			changedBaseOurs[e.A] = true
		}
	}

	changedBaseTheirs := make([]bool, len(base))
	for _, e := range eTheirs {
		if e.Kind == OpDelete && e.A >= 0 && e.A < len(base) {
			changedBaseTheirs[e.A] = true
		}
	}

	overlap := false
	for i := 0; i < len(base); i++ {
		if changedBaseOurs[i] && changedBaseTheirs[i] {
			overlap = true
			break
		}
	}

	if !overlap {
		insOurs := make([]bool, len(base)+1)
		for _, e := range eOurs {
			if e.Kind == OpInsert {
				idx := e.A
				if idx > len(base) {
					idx = len(base)
				}
				insOurs[idx] = true
			}
		}

		insTheirs := make([]bool, len(base)+1)
		for _, e := range eTheirs {
			if e.Kind == OpInsert {
				idx := e.A
				if idx > len(base) {
					idx = len(base)
				}
				insTheirs[idx] = true
			}
		}

		for i := 0; i <= len(base); i++ {
			if insOurs[i] && insTheirs[i] {
				overlap = true
				break
			}
		}

		if !overlap {
			var merged []string

			// To merge safely when no overlap, we reconstruct the array by keeping what both keep,
			// dropping what either drops, and inserting what either inserts.

			for i := 0; i < len(base); i++ {
				// Insertions before base[i]
				for _, e := range eOurs {
					if e.Kind == OpInsert && e.A == i {
						merged = append(merged, ours[e.B])
					}
				}
				for _, e := range eTheirs {
					if e.Kind == OpInsert && e.A == i {
						merged = append(merged, theirs[e.B])
					}
				}

				if !changedBaseOurs[i] && !changedBaseTheirs[i] {
					merged = append(merged, base[i])
				}
			}
			// Insertions after last base line (e.A == len(base))
			for _, e := range eOurs {
				if e.Kind == OpInsert && e.A == len(base) {
					merged = append(merged, ours[e.B])
				}
			}
			for _, e := range eTheirs {
				if e.Kind == OpInsert && e.A == len(base) {
					merged = append(merged, theirs[e.B])
				}
			}

			return resolveResult{lines: merged, conflict: false}
		}
	}

	return resolveResult{conflict: true}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func checkNewline3(sb *strings.Builder, s string) {
	if !strings.HasSuffix(s, "\n") && s != "" {
		sb.WriteString("\n")
	}
}
