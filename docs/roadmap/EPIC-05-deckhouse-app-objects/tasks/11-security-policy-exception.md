# EPIC-05 / 11: Ссылки на `SecurityPolicyException`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S (≤ 1 дня) |
| Зависит от | EPIC-05/02 (фикстура и отчёт) |
| Требования | R16 (из [spec.md](../spec.md)) |

## Контекст

`SecurityPolicyException` (`deckhouse.io/v1alpha1`, **namespaced**; F15) — объект приложения: точечное исключение из PSS/`SecurityPolicy` для pod'а или контейнера. Ссылка — метка **pod-шаблона** `security.deckhouse.io/security-policy-exception: <имя>` (весь pod) или `security.deckhouse.io/security-policy-exception.container.<контейнер>: <имя>`. Сам объект dhg уже переносит в chart generic fallback'ом (все поля сохраняются, ADR-054), но связь не видна: ни детектор (`pkg/analyzer/detector/reference.go`), ни golden-проверка целостности (`tests/golden/integrity_test.go:references`) её не знают. Если исключение потеряется при генерации (другое имя, другой chart), pod'ы будут отклонены admission'ом, а golden этого не заметит.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/analyzer/detector/reference.go` | `detectSecurityPolicyExceptionReferences`: метки pod-шаблона (Deployment/StatefulSet/DaemonSet/ReplicaSet/Job — `spec.template.metadata.labels`, CronJob — `spec.jobTemplate.spec.template.metadata.labels`, Pod — `metadata.labels`) → `types.ResourceKey{GVK: {deckhouse.io, v1alpha1, SecurityPolicyException}, Namespace: ns объекта, Name: значение}`; `RelationNameReference`, `Field: "spec.template.metadata.labels[<ключ>]"`; связь создаётся, только если ключ есть в `allResources` |
| `pkg/analyzer/detector/reference_test.go` | Тесты |
| `tests/golden/integrity_test.go` | `references`: для объектов с pod-шаблоном — `SecurityPolicyException/<значение>` по обеим формам метки (`podLabelsOf`) |
| `tests/integration/fixtures/deckhouse-observability/securitypolicyexception.yaml` (новый) | Объект фикстуры |
| `tests/integration/fixtures/deckhouse-observability/deployment-legacy.yaml` | Метка pod-шаблона `security.deckhouse.io/security-policy-exception: legacy-hostpath` |
| `pkg/generator/features_deckhouse.go` | Отчёт: в таблице PSS для workload'а с такой меткой дописать `(exception: SecurityPolicyException/<имя>)` |

## Шаги

1. Ключ метки: точное совпадение `security.deckhouse.io/security-policy-exception` или префикс `security.deckhouse.io/security-policy-exception.container.`.
2. Фикстура:
   ```yaml
   apiVersion: deckhouse.io/v1alpha1
   kind: SecurityPolicyException
   metadata:
     name: legacy-hostpath
     namespace: shop
   spec:
     volumes:
       hostPath:
         allowedValues:
           - path: /var/lib/legacy
             readOnly: false
   ```
   (поля — по CRD `@main modules/015-admission-policy-engine/crds/security-policy-exception.yaml`: `spec.volumes.hostPath.allowedValues[].{path,readOnly}`).
3. Связь не меняет группировку (группа SPE — по `ServiceNameFromResource`, design §4.4); в separate/umbrella SPE может оказаться в другом chart'е — это допустимо (один namespace релизов), отчёт об этом не пишет.

## Тесты

- Unit (`reference_test.go`): Deployment с меткой pod-шаблона и SPE в том же namespace → одна связь `name_reference`; метка `…container.legacy: legacy-hostpath` → связь; SPE в другом namespace → нет связи; метка на верхнем уровне `metadata.labels` (не pod-шаблон) → нет связи (DKP читает pod-шаблон, F15).
- Golden: `integrity_test.go` — новый случай в существующем `TestGeneratedChartsPassHelm` на фикстуре (автоматически); дополнительно unit-тест для `references` (если в `tests/golden` есть unit-тесты помощников — добавить туда; иначе — `TestIntegrityFindsSPE`: вход из двух объектов, рендер без SPE → `checkIntegrity` возвращает проблему `Deployment/legacy references SecurityPolicyException/legacy-hostpath …`).

## Критерии приёмки

- [ ] AC7 из spec.md выполнен.
- [ ] Все golden-сценарии на фикстуре проходят.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Генерация `SecurityPolicyException` по нарушениям PSS (решение о допустимости исключения — за безопасностью, не факт).
