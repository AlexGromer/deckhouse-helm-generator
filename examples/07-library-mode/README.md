# Example 07: Library Mode

Generates a **library chart** holding the helpers shared by all services
(`library.name`, `library.fullname`, `library.labels`, `library.selectorLabels`,
`library.image`, …) and one **application chart per service group** whose
templates call those helpers instead of carrying their own `_helpers.tpl`,
the pattern of common library charts such as `bitnami/common`.

## Input

```
07-library-mode/
├── frontend.yaml   # Deployment + Service for frontend
└── backend.yaml    # Deployment + Service for backend
```

## Usage

```bash
dhg generate \
  -f ./examples/07-library-mode \
  -o ./output/07 \
  --chart-name myapp \
  --chart-version 1.0.0 \
  --mode library

# charts/library is vendored, no dependency build needed
helm template frontend ./output/07/frontend
```

## Library from a registry

When the shared helpers are published once and versioned on their own, point
`dhg` at the registry instead of vendoring the library:

```bash
dhg generate \
  -f ./examples/07-library-mode \
  -o ./output/07 \
  --chart-name myapp \
  --mode library \
  --library-ref oci://registry.example.com/platform/platform-lib \
  --library-version "1.2.0"
```

The service charts then declare the library as an OCI dependency and do not
carry `charts/library`:

```yaml
dependencies:
  - name: platform-lib                              # last element of --library-ref
    version: 1.2.0                                  # --library-version, required with --library-ref
    repository: oci://registry.example.com/platform # the reference without the chart name
```

The library chart is still written to `output/07/platform-lib`, named after the
reference so that its `define "platform-lib.*"` blocks match the `include`s in
the templates. With an exact `--library-version` the library chart gets that
version; with a range (`^1.2.0`) it gets `--chart-version`, so publish a
version that satisfies the range. The library does not depend on the input, so
one published library serves any number of applications; the object names that
`global.namePrefix` needs live in each service chart
(`templates/_dhg-inchart.tpl`). Publish it, then fetch it into each service
chart:

```bash
helm package ./output/07/platform-lib
helm push platform-lib-1.2.0.tgz oci://registry.example.com/platform
helm dependency update ./output/07/frontend
```

Both settings can live in `.dhg.yaml` (`library-ref`, `library-version`; quote
the version so YAML keeps it a string); a command-line flag overrides the file.

## Output

```
output/07/
├── library/                  # type: library
│   ├── Chart.yaml
│   └── templates/
│       └── _helpers.tpl      # define "library.fullname", "library.labels", ...
├── frontend/                 # application chart
│   ├── Chart.yaml            # dependencies: library (file://charts/library, vendored)
│   ├── values.yaml           # flat values: enabled, deployment, service, ...
│   ├── README.md             # parameter table
│   ├── charts/library/       # copy of the library chart, lint/template need no dependency build
│   └── templates/
│       ├── frontend-deployment.yaml   # {{ include "library.labels" $ }} ...
│       ├── frontend-service.yaml
│       └── NOTES.txt
└── backend/                  # same structure
```

## When to Use

- Several services should share one definition of names and labels
- Organisation-wide label or naming conventions are changed once, in the
  library chart, and picked up by every service chart once the service charts
  are regenerated

Compared with `--mode separate`, the service charts are identical except
that helpers come from the library instead of a per-chart `_helpers.tpl`.
