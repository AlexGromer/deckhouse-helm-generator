# EPIC-10: Workloads Storage Scaling

| Поле | Значение |
|---|---|
| Статус | draft |
| Приоритет | P2 |
| Размер | M |
| Зависит от | — |
| Связанные ADR | — |

## Проблема
StatefulSet PVC retention, KEDA, VPA/HPA tuning, topologySpreadConstraints, PDB, B-009, B-015.

## Цель и результат
Масштабирование и хранение.

## Scope
- В scope: HPA, VPA, PDB, KEDA
- Вне scope: —

## Задачи и порядок
| # | Задача | Размер | Зависит от |
|---|---|---|---|
| 01 | [Workloads](tasks/01-workloads.md) | M | — |

## Риски и открытые вопросы
| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
