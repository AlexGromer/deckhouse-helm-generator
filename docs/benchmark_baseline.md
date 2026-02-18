# Performance Benchmark Baseline

**Date**: 2026-02-18
**Version**: v0.2.0-dev
**Hardware**: Intel Core i5-6300U @ 2.40GHz, 4 threads
**OS**: Linux 6.18.9+kali-amd64
**Go**: 1.21+

## Full Pipeline Benchmarks

Each resource set = 5 resources (Deployment + Service + ConfigMap + Secret + Ingress).

| Benchmark | Resources | Time (ns/op) | Time (human) | Memory (B/op) | Allocs/op |
|-----------|-----------|-------------|--------------|---------------|-----------|
| Pipeline_10Resources | 50 | 15,088,530 | ~15ms | 7,226,976 | 36,462 |
| Pipeline_100Resources | 500 | 190,971,205 | ~191ms | 80,368,413 | 377,738 |
| Pipeline_1000Resources | 5000 | 3,373,835,812 | ~3.4s | 1,115,365,269 | 5,570,261 |

## Scaling Analysis

| Factor | Time scaling | Memory scaling | Allocs scaling |
|--------|-------------|----------------|----------------|
| 10x → 100x (10→100 sets) | 12.7x | 11.1x | 10.4x |
| 100x → 1000x (100→1000 sets) | 17.7x | 13.9x | 14.7x |

Pipeline scales approximately linearly with slight super-linear overhead at 1000+ sets due to relationship detection O(n^2) patterns.

## How to Run

```bash
# Quick benchmark (3 iterations)
go test ./tests/integration/ -bench=BenchmarkPipeline -benchmem -benchtime=3x -run=^$

# Full benchmark (auto-calibrated iterations)
go test ./tests/integration/ -bench=BenchmarkPipeline -benchmem -run=^$

# Compare with baseline (requires benchstat)
go install golang.org/x/perf/cmd/benchstat@latest
go test ./tests/integration/ -bench=BenchmarkPipeline -benchmem -run=^$ -count=5 > new.txt
benchstat baseline.txt new.txt
```

## CI Integration

Add to CI pipeline (informational, non-blocking):

```yaml
benchmark:
  stage: test
  script:
    - go test ./tests/integration/ -bench=BenchmarkPipeline -benchmem -benchtime=3x -run=^$ | tee benchmark.txt
  artifacts:
    paths:
      - benchmark.txt
  allow_failure: true
```
