# EPIC-05 / 12: Совместимость с `OperationPolicy` кластера

| Поле | Значение |
|---|---|
| Статус | draft — открытый вопрос Q5 ([README](../README.md#риски-и-открытые-вопросы)) |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/02 |
| Требования | R17 (из [spec.md](../spec.md)) |

## Контекст

`OperationPolicy` (`deckhouse.io/v1alpha1`, **cluster-scoped**; F15) — операционные политики платформы (разрешённые registry, обязательные ресурсы и probes, запрещённые теги и т. д.; поля — F16). dhg её не создаёт (граница проекта), но команда приложения хочет узнать **до деплоя**, будет ли chart отклонён. Проверка — чтение политики и отчёт, без изменения chart'а.

**Почему draft:** не решено, откуда брать политики (Q5): файл (`--feature-opt deckhouse-report.policies=<путь>`) или кластер (`-s cluster` выгружает объекты, но `OperationPolicy` cluster-scoped и сейчас попадает в chart, а не в «контекст»). Рекомендация — файл/каталог YAML.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/compliance/operationpolicy/operationpolicy.go` (новый) | `type Policy struct{…}` (подмножество F16); `Load(paths []string) ([]Policy, error)` (YAML, multi-doc, только `kind: OperationPolicy`); `Check(p Policy, obj *unstructured.Unstructured) []Finding` |
| `pkg/compliance/operationpolicy/operationpolicy_test.go` (новый) | Тесты |
| `pkg/generator/features_deckhouse.go` | Параметр `policies` у `deckhouse-report`; раздел `## OperationPolicy` |
| `tests/golden/testdata/deckhouse/operationpolicy.yaml` (новый) | Пример политики |
| `tests/golden/deckhouse_test.go` | `TestOperationPolicyReport` |

## Шаги

1. Поддерживаемые проверки (точная семантика — по описаниям полей CRD `@main modules/015-admission-policy-engine/crds/operation-policy.yaml`; перед реализацией выписать её в этот файл):
   - `allowedRepos` — образ каждого контейнера начинается с одного из префиксов;
   - `requiredResources.{limits,requests}` — у каждого контейнера заданы перечисленные ресурсы (`cpu`, `memory`);
   - `disallowedImageTags` — тег образа не из списка;
   - `requiredProbes` — у каждого контейнера (не init) есть перечисленные probes;
   - `requiredLabels.labels[].{key,allowedRegex}` и `requiredLabels.watchKinds` — метки объектов перечисленных kind'ов (поля проверены по CRD `@main`);
   - `maxRevisionHistoryLimit` — `spec.revisionHistoryLimit` Deployment'а ≤ значения (не задано — нарушение: умолчание Kubernetes 10; уточнить по описанию CRD);
   - `imagePullPolicy` — значение у каждого контейнера;
   - `priorityClassNames`, `ingressClassNames`, `storageClassNames` — значение из списка;
   - `replicaLimits.{minReplicas,maxReplicas}` — `spec.replicas`.
   Остальные поля (`disallowedTolerations`, `requiredAnnotations`, `checkHostNetworkDNSPolicy`, `checkContainerDuplicates`, `gpuResourceRestriction`) — отметить в отчёте «not checked by dhg».
2. `spec.match.labelSelector` применяется к меткам объекта (верхний уровень для контроллеров — F13); `namespaceSelector` — пропускается с пометкой «namespace labels are unknown to dhg».
3. Отчёт: `| Object | Policy | Rule | Finding |`.

## Тесты

- Unit: по одной нарушающей и одной допустимой паре на каждую проверку шага 1; `labelSelector` не совпал → проверки не применяются; `Load` пропускает другие kind'ы и падает на битом YAML с именем файла.
- Golden: фикстура `deckhouse-observability` + `tests/golden/testdata/deckhouse/operationpolicy.yaml` (`allowedRepos: [registry.example.com/shop/]`, `requiredProbes: [livenessProbe, readinessProbe]`) → в отчёте `Deployment/orders | … | requiredProbes | container orders has no livenessProbe`.

## Критерии приёмки

- [ ] Q5 решён; семантика полей сверена с описаниями CRD.
- [ ] Без параметра `policies` раздел отсутствует.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

`SecurityPolicy` (развитие, design §10); создание или изменение политик.
