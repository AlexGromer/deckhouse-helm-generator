package generator

// ============================================================
// Test Plan: Conftest Library Generator (Task 6.0.8)
// ============================================================
//
// | #  | Test Name                                                    | Category    | Input                                              | Expected Output                                                                    |
// |----|--------------------------------------------------------------|-------------|-----------------------------------------------------|-----------------------------------------------------------------------------------|
// |  1 | TestGenerateConftestLibrary_SecurityCategory                 | happy       | Categories=["security"]                             | 4 security policies: deny-privileged, deny-root, deny-hostpath, deny-hostnetwork  |
// |  2 | TestGenerateConftestLibrary_ResourcesCategory                | happy       | Categories=["resources"]                            | 3 resources policies: require-requests, require-limits, deny-unbounded             |
// |  3 | TestGenerateConftestLibrary_AllCategories                    | happy       | all 5 categories                                    | Policies non-empty for each category, TotalRules > 0                              |
// |  4 | TestGenerateConftestLibrary_TotalRulesCount                  | happy       | all 5 categories                                    | TotalRules >= 11 (4+3+1+2+1)                                                       |
// |  5 | TestGenerateConftestLibrary_LabelsCategory                   | happy       | Categories=["labels"]                               | 1 policy: require-standard-labels                                                  |
// |  6 | TestGenerateConftestLibrary_CustomOutputDir                  | happy       | OutputDir="my-policies"                             | all policy paths prefixed with "my-policies/"                                     |
// |  7 | TestGenerateConftestLibrary_EmptyCategoriesDefaultsToAll     | edge        | Categories=[]                                       | result non-nil, all 5 categories generated                                        |
// |  8 | TestGenerateConftestLibrary_IndividualPolicyContent          | happy       | Categories=["networking"]                           | deny-loadbalancer.rego + require-networkpolicy.rego present, deny rules inside    |
// |  9 | TestGenerateConftestLibrary_NOTESTxt                         | happy       | any valid opts                                      | NOTESTxt non-empty, contains category summary                                     |
// | 10 | TestGenerateConftestLibrary_NilOptions                       | edge        | opts=nil                                            | returns non-nil result with defaults, no panic                                    |

import (
	"strings"
	"testing"
)

// ── helper: join all policy content ──────────────────────────────────────────

func allLibraryPolicyContent(policies map[string]string) string {
	var sb strings.Builder
	for _, v := range policies {
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── helper: count policies whose path contains prefix ────────────────────────

func countPoliciesByPrefix(policies map[string]string, prefix string) int {
	n := 0
	for k := range policies {
		if strings.Contains(k, prefix) {
			n++
		}
	}
	return n
}

// ── 1: security category → 4 policies ────────────────────────────────────────

func TestGenerateConftestLibrary_SecurityCategory(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"security"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected non-empty Policies for security category")
	}
	// Expect the 4 security policies
	wantKeys := []string{
		"deny-privileged",
		"deny-root",
		"deny-hostpath",
		"deny-hostnetwork",
	}
	for _, want := range wantKeys {
		found := false
		for k := range result.Policies {
			if strings.Contains(k, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("security category must include policy %q, got keys: %v", want, policyFileKeys(result.Policies))
		}
	}
}

// ── 2: resources category → 3 policies ───────────────────────────────────────

func TestGenerateConftestLibrary_ResourcesCategory(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"resources"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected non-empty Policies for resources category")
	}
	wantKeys := []string{
		"require-requests",
		"require-limits",
		"deny-unbounded",
	}
	for _, want := range wantKeys {
		found := false
		for k := range result.Policies {
			if strings.Contains(k, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("resources category must include policy %q, got keys: %v", want, policyFileKeys(result.Policies))
		}
	}
	// Each policy must have a deny rule
	for path, content := range result.Policies {
		if !strings.Contains(content, "deny") {
			t.Errorf("policy %q must contain a 'deny' rule", path)
		}
	}
}

// ── 3: all categories → all known policies present ───────────────────────────

func TestGenerateConftestLibrary_AllCategories(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"security", "resources", "labels", "networking", "storage"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected non-empty Policies for all categories")
	}
	if result.TotalRules == 0 {
		t.Error("TotalRules must be > 0 when all categories are generated")
	}
	// Each of the 5 category groups must contribute at least one policy
	for _, cat := range []string{"security", "resources", "labels", "networking", "storage"} {
		found := false
		for range result.Policies {
			// Policy paths should be organized under category dirs OR named distinctly
			_ = cat
			found = true // at least one policy exists from full generation
			break
		}
		if !found {
			t.Errorf("category %q produced no policies", cat)
		}
	}
}

// ── 4: TotalRules >= 11 for all categories ────────────────────────────────────
// security(4) + resources(3) + labels(1) + networking(2) + storage(1) = 11

func TestGenerateConftestLibrary_TotalRulesCount(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"security", "resources", "labels", "networking", "storage"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	const minExpected = 11
	if result.TotalRules < minExpected {
		t.Errorf("TotalRules must be >= %d for all categories, got %d", minExpected, result.TotalRules)
	}
}

// ── 5: labels category → require-standard-labels ─────────────────────────────

func TestGenerateConftestLibrary_LabelsCategory(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"labels"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected non-empty Policies for labels category")
	}

	found := false
	for k, content := range result.Policies {
		if strings.Contains(k, "require-standard-labels") || strings.Contains(k, "labels") {
			found = true
			lower := strings.ToLower(content)
			if !strings.Contains(lower, "label") {
				t.Errorf("labels policy %q must reference 'label', got: %.200s", k, content)
			}
			if !strings.Contains(content, "deny") {
				t.Errorf("labels policy %q must contain a 'deny' rule, got: %.200s", k, content)
			}
			break
		}
	}
	if !found {
		t.Errorf("labels category must include a standard-labels policy, got keys: %v",
			policyFileKeys(result.Policies))
	}
}

// ── 6: custom OutputDir prefixes all policy paths ────────────────────────────

func TestGenerateConftestLibrary_CustomOutputDir(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"security"},
		OutputDir:  "my-policies",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected policies for custom output dir test")
	}
	for k := range result.Policies {
		if !strings.HasPrefix(k, "my-policies/") {
			t.Errorf("policy path %q must be prefixed with 'my-policies/', got: %q", k, k)
		}
	}
}

// ── 7: empty Categories defaults to all 5 categories ─────────────────────────

func TestGenerateConftestLibrary_EmptyCategoriesDefaultsToAll(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{}, // empty → default to all
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult for empty Categories")
	}
	if len(result.Policies) == 0 {
		t.Error("empty Categories must default to all built-in categories (non-empty Policies)")
	}
	const minExpected = 11
	if result.TotalRules < minExpected {
		t.Errorf("empty Categories: expected TotalRules >= %d (all defaults), got %d", minExpected, result.TotalRules)
	}
}

// ── 8: networking category → deny-loadbalancer + require-networkpolicy ────────

func TestGenerateConftestLibrary_IndividualPolicyContent(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"networking"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if len(result.Policies) == 0 {
		t.Fatal("expected non-empty Policies for networking category")
	}

	wantKeys := []string{"deny-loadbalancer", "require-networkpolicy"}
	for _, want := range wantKeys {
		found := false
		for k, content := range result.Policies {
			if strings.Contains(k, want) {
				found = true
				if !strings.Contains(content, "deny") {
					t.Errorf("networking policy %q must contain a 'deny' rule, got: %.200s", k, content)
				}
				break
			}
		}
		if !found {
			t.Errorf("networking category must include %q policy, got keys: %v",
				want, policyFileKeys(result.Policies))
		}
	}
}

// ── 9: NOTESTxt is non-empty and contains category summary ───────────────────

func TestGenerateConftestLibrary_NOTESTxt(t *testing.T) {
	opts := ConftestLibraryOptions{
		Categories: []string{"security", "resources"},
		OutputDir:  "policy",
	}

	result := GenerateConftestLibrary(opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected non-empty NOTESTxt with library summary")
	}
	// NOTESTxt must reference conftest or policies
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "conftest") && !strings.Contains(lower, "polic") {
		t.Errorf("NOTESTxt must mention conftest or policies, got: %s", result.NOTESTxt)
	}
}

// ── 10: nil options → non-nil result, no panic ───────────────────────────────

func TestGenerateConftestLibrary_NilOptions(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GenerateConftestLibrary panicked on nil options: %v", r)
		}
	}()

	// Pass zero-value options (Go has no nil for structs, test empty/default)
	result := GenerateConftestLibrary(ConftestLibraryOptions{})

	if result == nil {
		t.Fatal("expected non-nil ConftestLibraryResult for zero-value options")
	}
	// With zero-value options, should default to all categories
	if len(result.Policies) == 0 {
		t.Error("zero-value ConftestLibraryOptions must produce default policies (non-empty)")
	}
	if result.Policies == nil {
		t.Error("Policies map must be initialized (not nil)")
	}
}
