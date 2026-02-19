# Gateway API Example

This example demonstrates generating Helm charts using the Kubernetes Gateway API.

## Usage

```bash
dhg generate -f ./input -o ./output --chart-name webapp-gateway
```

## Contents

- `input/gateway.yaml` - Gateway with HTTP and HTTPS listeners
- `input/httproute.yaml` - HTTPRoute with path-based routing rules
- `input/deployment.yaml` - Backend Deployment with health checks
- `input/service.yaml` - Backend Service for routing

The generator will create a chart with modern Gateway API networking.
