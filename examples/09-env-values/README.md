# Example 09: Environment-Specific Values

The `--env-values` flag generates three environment override files alongside the main chart.
Each file contains only the **differences** from base values (override-only principle).

## Input

```
09-env-values/
└── app.yaml   # Deployment + Service
```

## Usage

```bash
dhg generate \
  -f ./examples/09-env-values \
  -o ./output/09 \
  --chart-name myapp \
  --chart-version 1.0.0 \
  --env-values
```

## Expected Output

```
output/09/
└── myapp/
    ├── Chart.yaml
    ├── values.yaml          # base values
    ├── values-dev.yaml      # dev overrides
    ├── values-staging.yaml  # staging overrides
    └── values-prod.yaml     # prod overrides
```

## Generated Files

### values-dev.yaml
```yaml
# Dev environment overrides — relaxed settings for local development
replicaCount: 1
logLevel: debug
podDisruptionBudget:
  enabled: false
```

### values-staging.yaml
```yaml
# Staging environment overrides — mirrors prod at reduced scale
replicaCount: 2
logLevel: info
podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

### values-prod.yaml
```yaml
# Production environment overrides — hardened settings
replicaCount: 3
logLevel: warn
podDisruptionBudget:
  enabled: true
  minAvailable: 2
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          topologyKey: kubernetes.io/hostname
```

## Deploying to Different Environments

```bash
# Dev (local/minikube)
helm upgrade --install myapp ./output/09/myapp \
  -f ./output/09/myapp/values-dev.yaml

# Staging
helm upgrade --install myapp ./output/09/myapp \
  -f ./output/09/myapp/values-staging.yaml

# Production
helm upgrade --install myapp ./output/09/myapp \
  -f ./output/09/myapp/values-prod.yaml
```

## Override-Only Principle

The generated env files contain **only the keys that differ** between environments.
They do NOT copy the full base `values.yaml`. This means:

1. Adding a new value to `values.yaml` automatically applies to all environments
2. Each env file is minimal and readable
3. `helm upgrade -f values-prod.yaml` correctly merges with base values

## When to Use

- Monorepo: all environments managed in one chart
- GitOps: each environment has its own `values-<env>.yaml` override
- Quick-start: auto-generate sensible defaults per environment
- Any output mode: works with `universal`, `separate`, `library`, `umbrella`
