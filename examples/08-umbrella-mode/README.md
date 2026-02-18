# Example 08: Umbrella Mode

Generates a **parent umbrella chart** with all services as subcharts in `charts/` directory.
Each subchart has `condition: <name>.enabled` — services can be conditionally disabled at deploy time.

## Input

```
08-umbrella-mode/
├── frontend.yaml   # Deployment + Service для frontend
├── backend.yaml    # Deployment + Service для backend
└── database.yaml   # StatefulSet + Service для database
```

## Usage

```bash
dhg generate \
  -f ./examples/08-umbrella-mode \
  -o ./output/08 \
  --chart-name myapp \
  --chart-version 1.0.0 \
  --mode umbrella
```

## Expected Output

```
output/08/
└── myapp/
    ├── Chart.yaml       # dependencies: [frontend, backend, database]
    ├── values.yaml      # frontend.enabled: true, backend.enabled: true, database.enabled: true
    ├── templates/
    │   └── _helpers.tpl
    └── charts/
        ├── frontend/
        │   ├── Chart.yaml
        │   ├── values.yaml
        │   └── templates/
        ├── backend/
        │   ├── Chart.yaml
        │   ├── values.yaml
        │   └── templates/
        └── database/
            ├── Chart.yaml
            ├── values.yaml
            └── templates/
```

## Deploying with Conditional Services

```bash
# Full stack
helm upgrade --install myapp ./output/08/myapp

# Without database (e.g., using external RDS)
helm upgrade --install myapp ./output/08/myapp --set database.enabled=false

# Frontend only (dev environment)
helm upgrade --install myapp ./output/08/myapp \
  --set backend.enabled=false \
  --set database.enabled=false
```

## Parent values.yaml Structure

```yaml
# Umbrella chart values — per-subchart sections
global:
  imageRegistry: ""

frontend:
  enabled: true
  replicaCount: 2

backend:
  enabled: true
  replicaCount: 3

database:
  enabled: true
  replicaCount: 1
```

## When to Use

- Services are deployed **together** as a stack but some may be optional
- Replace external services at runtime (`--set database.enabled=false` when using RDS)
- ArgoCD/Flux: single Application with conditional service toggles
- Helmfile alternative: single chart with feature flags

## Comparison with Other Modes

| Feature | Umbrella | Separate |
|---------|----------|----------|
| Single deploy command | ✅ | ❌ (N deploys) |
| Conditional services | ✅ | ❌ |
| Independent versioning | ❌ | ✅ |
| Inter-service values | ✅ (cascading) | ❌ |
