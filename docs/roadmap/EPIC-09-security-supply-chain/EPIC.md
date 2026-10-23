# EPIC-09: Security Supply Chain

| Поле | Значение |
|---|---|
| Статус | draft |
| Приоритет | P2 |
| Размер | M |
| Зависит от | — |
| Связанные ADR | — |

## Проблема
NetworkPolicy default-deny, RBAC least privilege, image digests/SBOM/cosign, Vault/ExternalSecrets, B-008, B-021.

## Цель и результат
Улучшение безопасности.

## Scope
- В scope: NetworkPolicy, RBAC, Vault
- Вне scope: —

## Задачи и порядок
| # | Задача | Размер | Зависит от |
|---|---|---|---|
| 01 | [Security](tasks/01-security.md) | M | — |

## Риски и открытые вопросы
| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
