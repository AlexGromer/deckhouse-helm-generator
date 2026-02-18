# Velocity Tracking — v0.3.0

**Started**: 2026-02-18 (UTC)
**Completed**: 2026-02-18 (UTC)
**Methodology**: Timestamp-based per-task tracking
**Velocity baseline** (from v0.2.0): 15.6x (solo estimate / actual AI time)
**Expected velocity for v0.3.0**: 8-12x (more complex tasks)

> **Note on timestamps**: v0.3.0 выполнялся до внедрения real-time timestamp трекинга.
> Actual times помечены `~approx` — оценки по структуре сессий, не реальный трекинг.
> Начиная с v0.4.0: `date -u` записывается в начале и конце каждой задачи.

---

## Task Execution Log

| Task | Description | Est (solo) | Est (AI) | Actual | Velocity | Notes |
|------|-------------|-----------|---------|--------|----------|-------|
| 1.1 | Service Grouping Algorithm | 11.0h | 1.1h | ~0.35h | ~31x | GroupResources(), LabelGrouper, NameGrouper |
| 1.2 | Separate Generator | 13.0h | 1.3h | ~0.45h | ~29x | SeparateGenerator, buildFlatValues, 12 tests |
| 1.3 | Inter-Chart Dependencies | 9.0h | 0.9h | ~0.25h | ~36x | DetectCrossChartDeps, circular detection |
| 1.4 | Shared Values (Global) | 9.0h | 0.9h | ~0.20h | ~45x | ExtractGlobalValues, imageRegistry, envVars |
| 1.5 | Integration Tests — Separate | 9.0h | 0.9h | ~0.35h | ~26x | 7 tests, fixtures, pipeline_separate_test.go |
| 2.1 | Base Library Chart | 15.0h | 1.5h | ~0.45h | ~33x | LibraryGenerator, 18 named templates, type: library |
| 2.2 | Wrapper Charts | 11.0h | 1.1h | ~0.30h | ~37x | generateWrapperChart, wrapper_test.go (9+1 tests) |
| 2.3 | DRY Named Templates | 9.0h | 0.9h | ~0.35h | ~26x | 9 sub-templates, library_helpers.go |
| 2.4 | Integration Tests — Library | 7.0h | 0.7h | ~0.55h | ~13x | Медленнее: chartutil.Values / dig type bug |
| 3.1 | Parent Chart with Subcharts | 13.0h | 1.3h | ~0.50h | ~26x | UmbrellaGenerator, 13 tests, OutputModeUmbrella |
| 3.2 | Conditional Subcharts | 5.0h | 0.5h | ~0.15h | ~33x | condition: <name>.enabled; 3.1 уже реализовал |
| 3.3 | Integration Tests — Umbrella | 7.0h | 0.7h | ~0.30h | ~23x | 9 tests (helm template, helm lint) |
| 4.1 | Environment-Specific Values | 11.0h | 1.1h | ~0.25h | ~44x | GenerateEnvValues(), 22 tests; float64/int fix |
| 4.2 | Documentation & Release Prep | 7.0h | 0.7h | ~0.30h | ~23x | README, CHANGELOG v0.3.0, examples 06-09 |
| 4.3 | Release v0.3.0 | 5.0h | 0.5h | ~0.20h | ~25x | git tag v0.3.0, Docker build, benchmarks |
| **TOTAL** | | **141.0h** | **13.1h** | **~4.95h** | **~28.5x** | Все данные ~approx |

---

## Running Metrics

| Metric | Value |
|--------|-------|
| Tasks completed | **15 / 15** ✅ |
| Total actual time | **~4.95h** (~approx) |
| Running avg velocity | **~28.5x** (~approx) |
| Remaining | 0h — COMPLETE |

---

## Velocity by Phase

| Phase | Tasks | Est (solo) | Actual | Avg Velocity | Notes |
|-------|-------|-----------|--------|-------------|-------|
| Separate Generator | 1.1-1.5 | 51.0h | ~1.60h | ~32x | Чистая фаза, паттерны хорошо определены |
| Library Generator | 2.1-2.4 | 42.0h | ~1.65h | ~25x | Медленнее: chartutil.Values bug (~20 мин) |
| Umbrella Generator | 3.1-3.3 | 25.0h | ~0.95h | ~26x | 3.2 почти бесплатная — реализована в 3.1 |
| Env Values + Polish | 4.1-4.3 | 23.0h | ~0.75h | ~31x | float64/int fix ~10 мин |
| **Overall** | **1.1-4.3** | **141.0h** | **~4.95h** | **~28.5x** | |

---

## Comparison with v0.2.0

| Metric | v0.2.0 | v0.3.0 | Delta |
|--------|--------|--------|-------|
| Tasks | 24 | 15 | -9 (крупнее задачи) |
| Subtasks | ~100 | 131 | +31 |
| Solo estimate | 290.5h | 141.0h | -149.5h |
| AI estimate | 21.55h | 13.1h | -8.45h |
| Actual | 21.55h | ~4.95h (~approx) | ~-16.6h |
| Avg velocity | 15.6x | ~28.5x (~approx) | +83% |

---

## Bugs that cost time

| Bug | Task | ~Time lost | Fix |
|-----|------|-----------|-----|
| `chartutil.Values` не совместим с Sprig `dig` | 2.4 | ~20 мин | `.values \| toJson \| fromJson` |
| `float64` vs `int` в `sigs.k8s.io/yaml.Unmarshal` | 4.1 | ~10 мин | `toInt(v interface{}) int` хелпер |
| Subchart path `"charts"` vs `"charts/"` (trailing slash) | 3.1 | ~5 мин | добавить `/` в конец Path |

---

## Timestamp Protocol (начиная с v0.4.0)

```bash
# Начало задачи — запустить и записать в трекер:
date -u +"%Y-%m-%d %H:%M:%S UTC"

# Конец задачи (после финального go test ./...) — то же самое
date -u +"%Y-%m-%d %H:%M:%S UTC"
```

Формат строки в таблице: `| Task | ... | START_UTC | END_UTC | duration = end-start |`
