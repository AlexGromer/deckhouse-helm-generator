# Example 06: Separate Mode

Generates an independent Helm chart **per service** from the input manifests.

## Input

```
06-separate-mode/
├── frontend.yaml   # Deployment + Service для frontend
└── backend.yaml    # Deployment + Service для backend
```

## Usage

```bash
dhg generate \
  -f ./examples/06-separate-mode \
  -o ./output/06 \
  --chart-name myapp \
  --chart-version 1.0.0 \
  --mode separate
```

## Expected Output

```
output/06/
├── frontend/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── _helpers.tpl
│       ├── deployment.yaml
│       └── service.yaml
└── backend/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── _helpers.tpl
        ├── deployment.yaml
        └── service.yaml
```

## When to Use

- Services are deployed **independently** (different versions, different schedules)
- Each service has its own CI/CD pipeline
- Team structure: separate ownership per service
- ArgoCD/Flux: one Application per chart

## Comparison with Other Modes

| Feature | Separate | Universal |
|---------|----------|-----------|
| Charts produced | N (one per service) | 1 |
| Independent deploy | ✅ | ❌ |
| Values isolation | ✅ | ❌ |
| Helm history per service | ✅ | ❌ |
