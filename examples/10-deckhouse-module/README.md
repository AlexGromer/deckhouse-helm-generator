# Deckhouse Module Example

This example demonstrates generating Helm charts from Deckhouse-specific resources.

## Usage

```bash
dhg generate -f ./input -o ./output --chart-name mymodule --deckhouse-module
```

## Contents

- `input/moduleconfig.yaml` - ModuleConfig resource
- `input/ingressnginx.yaml` - IngressNginxController resource
- `input/deployment.yaml` - Standard Deployment

The generator will process these resources and create a Helm chart optimized for Deckhouse deployment.
