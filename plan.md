1.  **Implement methods in `pkg/generator/output.go`**:
    *   Add `Get`, `Paths`, `Len`, `ChartFiles` and `WriteOutputSet` to `OutputSet`.
    *   Use `cat` to read `pkg/generator/output.go` to verify its implementation.

2.  **Implement `pkg/regen/state.go`**:
    *   Define `FileState` (`Path`, `SHA256`, `Mode`) and `State` (`FormatVersion: 1`, `DHGVersion`, `ChartName`, `Mode`, `Options`, `Files`).
    *   Implement `Load(outputDir, chartName string)`: Reads state and `base/*.base` files, validating SHA256. Returns `*State, *generator.OutputSet`.
    *   Implement `Discover(outputDir string)`: Finds subdirectories in `.dhg/`.
    *   Implement `Save(outputDir string, st *State, theirs *generator.OutputSet)`: Writes base files to `.dhg/<chartName>/base/` and `state.json` atomically via tmp file.
    *   Implement `Edited(outputDir string, base *generator.OutputSet)`: Compares on-disk files with the loaded base state to return tracked files that were modified or removed.
    *   Use `cat` to read `pkg/regen/state.go` to verify its implementation.

3.  **Implement `pkg/regen/plan.go`**:
    *   Define `Plan` struct and `BuildPlan(outputDir string, base, theirs *generator.OutputSet, o PlanOptions)`.
    *   Implement three-way merge logic classifying files into `added`, `updated`, `merged`, `conflict`, `removed`, `kept`, `kept-deleted`, `unchanged`.
    *   Use `merge.MergeYAML` for `.yaml` files not in `templates/` or `crds/`, and `merge.Merge3` for other text files.
    *   Detect unresolved markers (`<<<<<<< ours:`, `>>>>>>> theirs:`, `dhg-conflict:`).
    *   Implement `Apply(outputDir string)`: Writes changed files atomically to `outputDir`, preserving `mtime` for unchanged files, deletes `removed` files and empty directories.
    *   Use `cat` to read `pkg/regen/plan.go` to verify its implementation.

4.  **Implement `pkg/regen/bump.go`**:
    *   Implement `Bump(policy string, theirs *generator.OutputSet)` to increase SemVer (`patch`, `minor`, `major`) in `Chart.yaml` of changed charts and update `dependencies[].version` in the umbrella parent.
    *   Use `cat` to read `pkg/regen/bump.go` to verify its implementation.

5.  **Implement `pkg/regen/report.go`**:
    *   Implement methods for `Plan`: `Conflicts() int`, `Summary() string`, `MergeReport() string` which formats conflicts into Markdown (`<!-- dhg merge report -->`).
    *   Use `cat` to read `pkg/regen/report.go` to verify its implementation.

6.  **Implement `pkg/regen/state_test.go`**:
    *   Write `state_test.go` by creating table tests for Load (testing valid and invalid SHA256 hashes), Save (testing atomic writes and directory creation), Discover, and Edited functionalities.
    *   Use `cat` to read `pkg/regen/state_test.go` to verify its implementation.

7.  **Implement `pkg/regen/plan_test.go`**:
    *   Write `plan_test.go` by creating tests for BuildPlan to verify correct file classification (`added`, `updated`, `merged`, `conflict`, `removed`, `kept`, `kept-deleted`, `unchanged`) based on base, ours, and theirs inputs. Also test marker detection.
    *   Use `cat` to read `pkg/regen/plan_test.go` to verify its implementation.

8.  **Implement `pkg/regen/bump_test.go`**:
    *   Write `bump_test.go` by testing Bump method for patch, minor, major policies on `Chart.yaml` version field and updating parent dependencies.
    *   Use `cat` to read `pkg/regen/bump_test.go` to verify its implementation.

9.  **Implement `pkg/regen/report_test.go`**:
    *   Write `report_test.go` by writing tests checking the output strings of `Summary()` and `MergeReport()` logic to ensure markdown table format and conflict comments match the spec.
    *   Use `cat` to read `pkg/regen/report_test.go` to verify its implementation.

10. **Modify `cmd/dhg/config.go`**:
    *   In `config.go`, set `flag.Changed = true` when applying config values to respect precedence rules (CLI > config > state > default).
    *   Use `cat` to read `cmd/dhg/config.go` to verify its implementation.

11. **Modify `cmd/dhg/main.go` flag definition**:
    *   Add `--update`, `--force`, `--no-state`, `--bump`, `--base-dir` flags to `dhg generate` inside `newGenerateCmd`.
    *   Use `cat` to read `cmd/dhg/main.go` to verify its implementation.

12. **Modify `cmd/dhg/regen.go` flag struct**:
    *   Add new flag definitions `update`, `force`, `noState`, `bump`, `baseDir` into `generateOptions`.
    *   Use `cat` to read `cmd/dhg/regen.go` (or `main.go` if in there) to verify its implementation.

13. **Modify `cmd/dhg/main.go` generation logic (`runGenerate`)**:
    *   If `--update` is set, check `.dhg/<chart-name>/state.json`. If found, load options from state.
    *   If `baseDir` is set, load state from there.
    *   Intercept output saving: Instead of `generator.WriteChart`, use `generator.WriteOutputSet`.
    *   Build plan with `pkg/regen.BuildPlan`.
    *   Print plan summary to stdout.
    *   Apply bump policy.
    *   Call `plan.Apply()`.
    *   Save new state unless `--no-state` is set.
    *   Return appropriate exit codes (0 for success, 3 for unresolved conflicts/write, 1 for error).
    *   Use `cat` to read `cmd/dhg/main.go` to verify its implementation.

14. **Modify `cmd/dhg/main.go` diff logic (`runDiff`)**:
    *   Add `--exit-code` flag.
    *   Filter out `.dhg/` directory when collecting files.
    *   Use `merge.Unified` to generate the diff output.
    *   Return exit code 1 if differences exist and `--exit-code` is provided.
    *   Use `cat` to read `cmd/dhg/main.go` to verify its implementation.

15. **Run tests and linters**:
    *   Run `rm -f plan.md patch_*.py`.
    *   Run `gofmt -s -w .`.
    *   Run `go vet ./...`.
    *   Run `golangci-lint run ./pkg/regen/... ./pkg/generator/... ./cmd/dhg/...`.
    *   Run `go test -race ./pkg/regen/... ./pkg/merge/... ./pkg/generator/... ./cmd/dhg/...`.

16. **Complete pre-commit steps**:
    *   Complete pre-commit steps to ensure proper testing, verification, review, and reflection are done.

17. **Submit**:
    *   Submit changes.
