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
