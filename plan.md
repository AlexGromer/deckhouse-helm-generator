1. **pkg/processor/k8s/argocd.go**:
   - Create `ArgoCDApplicationProcessor` (GVK: `argoproj.io/v1alpha1`, `Application`) and `ArgoCDApplicationSetProcessor` (GVK: `argoproj.io/v1alpha1`, `ApplicationSet`).
   - Extract `project`, `source`, `destination`, `syncPolicy` fields into `Values`.
   - Save specific annotations: `argocd.argoproj.io/sync-wave`, `argocd.argoproj.io/hook`, `argocd.argoproj.io/hook-delete-policy`.
   - Use `processor.SpecOverlay` for fidelity.

2. **pkg/processor/k8s/argocd_test.go**:
   - Add tabular tests covering parsing of `Application` and `ApplicationSet`, checking values, fidelity, and annotations.

3. **pkg/processor/k8s/registry.go**:
   - Register `NewArgoCDApplicationProcessor()` and `NewArgoCDApplicationSetProcessor()`.

4. **pkg/generator/features_argocd.go**:
   - Register feature `argocd` in `init()`.
   - Implement `Apply` method to add `argocd:` object in `values.yaml` if feature is enabled. Handle `repo-url` requirement.
   - Include notes formatting for empty `repo-url`.

5. **pkg/generator/features_argocd_test.go**:
   - Write tests for feature `argocd`.

6. **Pre-commit checks**:
   - Use `gofmt`, `go vet`, `golangci-lint`, `go test`.
