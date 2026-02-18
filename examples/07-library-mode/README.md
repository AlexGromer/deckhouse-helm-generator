# Example 07: Library Mode

Generates a **shared library chart** with DRY named templates, plus thin **wrapper charts** for each service.

## Input

```
07-library-mode/
├── frontend.yaml   # Deployment + Service для frontend
└── backend.yaml    # Deployment + Service для backend
```

## Usage

```bash
dhg generate \
  -f ./examples/07-library-mode \
  -o ./output/07 \
  --chart-name myapp \
  --chart-version 1.0.0 \
  --mode library
```

## Expected Output

```
output/07/
├── myapp/                    # library chart (type: library)
│   ├── Chart.yaml            # type: library
│   └── templates/
│       ├── _helpers.tpl
│       ├── _deployment.tpl   # define "library.deployment"
│       ├── _service.tpl      # define "library.service"
│       ├── _resources.tpl    # define "library.resources"
│       ├── _env.tpl          # define "library.env"
│       └── ...
├── frontend/                 # wrapper chart
│   ├── Chart.yaml            # dependencies: [myapp library]
│   ├── values.yaml
│   └── templates/
│       ├── _helpers.tpl
│       ├── deployment.yaml   # calls {{ include "library.deployment" . }}
│       └── service.yaml
└── backend/                  # wrapper chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
```

## DRY Shared Templates

The library chart contains named templates used by all wrappers:

| Template | Description |
|----------|-------------|
| `library.resources` | CPU/memory requests and limits |
| `library.probes` | livenessProbe and readinessProbe |
| `library.env` | Environment variables |
| `library.volumeMounts` | Volume mounts |
| `library.volumes` | Volumes |
| `library.labels` | Standard Kubernetes labels |
| `library.annotations` | Annotations |
| `library.securityContext` | Pod security context |
| `library.containerSecurityContext` | Container security context |

## When to Use

- Multiple services share **identical template patterns**
- You want a **single source of truth** for template logic
- Reducing maintenance overhead across many charts
- Enforcing organizational standards for all services

## Comparison with Other Modes

| Feature | Library | Separate |
|---------|---------|----------|
| Template duplication | ❌ (DRY) | ✅ (per chart) |
| Shared standards | ✅ | ❌ |
| Update all charts at once | ✅ (library version bump) | ❌ (N charts) |
| Helm history per service | ✅ | ✅ |
