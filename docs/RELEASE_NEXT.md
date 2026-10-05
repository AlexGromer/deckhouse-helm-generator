# Следующий релиз (v1.x)

Ревизия корректности: каждый сгенерированный chart проверяется настоящим Helm, рендер воспроизводит входные манифесты, недостижимый код подключён или удалён. Решения — ADR-045 … ADR-057 в [ADR.md](ADR.md).

## Несовместимые изменения

| Было | Стало | Что сделать |
|---|---|---|
| Объекты переименовывались в `<release>-<chart>-<name>`, селекторы и метки pod'ов заменялись метками chart'а | Сохраняются исходные имена, селекторы и метки (ADR-049) | Повторно сгенерировать chart. Существующие объекты кластера Helm может перенять (`helm install --take-ownership`, Helm ≥ 3.17) |
| CRD рендерились в `templates/` | CRD лежат в `crds/` без шаблонизации (ADR-050) | Обновление CRD — отдельным шагом (`kubectl apply -f crds/`), Helm их не обновляет |
| `--kustomize`: `base` ссылался на шаблоны Helm, патч prod-лимитов `cpu: 1`/`512Mi` | `base` — входные манифесты, overlay'и меняют только `replicas` (ADR-055) | Лимиты для prod задавать патчем в своём overlay или values chart'а |
| Опциональные генераторы — отдельные флаги или недостижимы | Реестр `--with <feature>` и `--feature-opt feature.param=value`, список — `dhg features` (ADR-046) | Заменить флаги на `--with` |
| `--cluster-namespace` (не реализован) | Удалён; фильтр по namespace для всех источников — `--namespace` / `--namespaces` | — |
| `--template-style` (ни на что не влиял) | Удалён; переопределение шаблонов — `--template-dir` и `--template-strategy` | — |
| Зависимость Deckhouse-модуля `helm_lib`, `version: "*"` | `deckhouse_lib_helm`, `version: "~1"` | `helm dependency update` |
| Velero Schedule с `labelSelector` | `orLabelSelectors` (ADR-057) | Velero ≥ 1.10 |
| В separate/library/umbrella два сервиса группы с одинаковым путём values перезаписывали друг друга | Сервис с пересекающимися путями хранит values под своим ключом (ADR-056) | Сверить `values.yaml` таких групп |

## Исправленные ошибки, менявшие данные

- Метки входа перезаписывались метками chart'а.
- Значения ConfigMap/Secret получали или теряли финальный `\n`; экстрактор резал документ на `---` внутри блочного скаляра.
- Нулевые значения терялись (`replicas: 0` → 1, `backoffLimit: 0`, `enabled: false`, `minReplicaCount: 0`).
- Image digest разбирался как tag.
- 20 процессоров custom resources теряли поля `spec` вне белого списка (ADR-054).
- `User` (Deckhouse) сохранял пароль в `values.yaml`.
- Одинаковые kind/name в разных namespace и Deployment'ы одного сервиса молча перезаписывали друг друга.
- separate-режим рендерил 0 объектов, umbrella не генерировался, `values.schema.json` был YAML, HPA/PDB/Gateway/HTTPRoute не рендерились.

## Новое

- Источники `cluster` (client-cert, token/`tokenFile`, exec-плагины ExecCredential v1/v1beta1 — kubelogin для OIDC/Dex; `--cluster-secrets skip|mask|include`) и `gitops` (`--git-path`); `--selector` для файлов.
- `dhg validate` с разбором шаблонов и матрицей версий Kubernetes API, `dhg graph`, `dhg features`.
- Плагины процессоров (`--plugin`), `.dhg.yaml` (`--config`), `--template-dir`.
- Golden-набор `tests/golden`: lint/template, сохранность объектов, ссылки и селекторы, fidelity, `helm unittest`, post-renderer, `kustomize build`; CI-job с Helm 3.19.

## Путь модуля

Модуль переименован в `github.com/AlexGromer/deckhouse-helm-generator` — фактический адрес репозитория; `go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@<версия>` работает начиная с этого релиза. Код, импортировавший пакеты по прежнему пути `github.com/deckhouse/deckhouse-helm-generator`, нужно обновить. Тег релиза — `v1.x`: тег `v2.0.0` и выше Go примет, только если путь модуля оканчивается на `/v2`.

## Известные ограничения

- Объекты рендерятся в namespace релиза (кроме одноимённых объектов из разных namespace).
