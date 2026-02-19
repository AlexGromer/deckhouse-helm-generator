# Monitoring Stack Example

This example demonstrates generating Helm charts with comprehensive monitoring resources.

## Usage

```bash
dhg generate -f ./input -o ./output --chart-name myapp-monitoring
```

## Contents

- `input/deployment.yaml` - Application Deployment with metrics port
- `input/service.yaml` - Service exposing both HTTP and metrics ports
- `input/servicemonitor.yaml` - ServiceMonitor for Prometheus scraping
- `input/prometheusrule.yaml` - PrometheusRule with alert definitions

The generator will create a chart with full monitoring instrumentation.
