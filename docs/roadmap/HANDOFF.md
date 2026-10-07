# Передача работы: состояние на 2026-10-07

Ветка `claude/vibrant-hopper-7xilwh` (от `main` @ `778e1fe`). Всё ниже — что сделано, что проверено и что осталось, в порядке выполнения.

## 1. Что в ветке

| Коммит | Содержание | Проверено |
|---|---|---|
| `docs(adr): reconcile ADR statuses…`, `ADR-042…` | Статусы ADR-014…038 и ADR-042 сверены с кодом: Proposed → Accepted / Superseded / Rejected / Deferred | grep-подсчёт сводки |
| `feat(registry): docker credential helpers` | `credHelpers`/`credsStore`, таймаут 30 с, кэш, предупреждения без секретов | unit, race, golden |
| `docs(roadmap): catalog skeleton…` | `docs/roadmap/README.md` (правила, DoD, общие контракты), шаблоны, `BACKLOG.md` | — |
| `feat(source): Quarkus, Micronaut, framework profiles and --image-config` | `pkg/synth/{project,frameworks,container}.go`, `cmd/dhg/profiles.go`, golden-фикстуры `payments-quarkus`, `ledger-micronaut` | unit (мутационная проверка), golden с Helm во всех режимах, `helm lint --strict` с `-f values-profile-*.yaml` |
| `fix(namespace): size ResourceQuota…` | Квота = сумма по workload'ам (раньше всегда была заглушка `1/1Gi/2/2Gi`) | unit, golden |
| `feat(istio): --with istio-ingress…` | Istio Gateway + VirtualService из Ingress (ADR-061) | unit, golden |
| `fix: deterministic grouping, native sidecars, ADR-062` | (1) `GroupResources` и анализатор обходят ресурсы в порядке ключей; группа с занятым именем получает суффикс `-2` вместо перезаписи — раньше chart'ы случайно терялись в режимах separate/library/umbrella (причина нестабильного падения `TestFeaturesPassHelm/all/separate/fixtures/custom-resources`: 16 объектов вместо 23). (2) Сохраняются `restartPolicy` init-контейнеров (native sidecar) и другие поля контейнера. (3) ADR-062, задачи T-01, T-02 | unit; 16 параллельных запусков dhg дают побайтно одинаковый вывод; fidelity golden для sidecar падает без исправления и проходит с ним. **Полный golden после исправления группировки не перезапускался** |
| `docs(roadmap): agent drafts…` | Черновики эпиков от агентов (см. раздел 4) | **не вычитаны** |

## 2. Сделать перед PR

```bash
export PATH=$HOME/go/bin:$PATH        # golangci-lint v2.14, helm, deadcode
gofmt -l cmd pkg tests                # пусто
go vet ./... && golangci-lint run
go test -race -count=1 ./cmd/... ./pkg/...
go test -count=1 ./tests/integration/... ./tests/e2e/...
deadcode -test ./...
go build -o /tmp/dhg ./cmd/dhg
for i in 1 2 3; do DHG_REQUIRE_HELM=1 go test -count=1 ./tests/golden/ | tail -1; done   # 3 зелёных подряд
```

- Убедиться, что новый тест `TestGroupResources_DeterministicAndNoNameClash` **падает** на старом `pkg/generator/grouping.go` (`git stash push pkg/generator/grouping.go`, прогнать, `git stash pop`).
- Если golden где-то ожидал старые случайные имена групп — исправить ожидания (имена теперь детерминированы: label → компонента связей → namespace, при коллизии `-2`, `-3`).
- Добавить в `docs/RELEASE_NEXT.md` («Исправленные ошибки, менявшие данные») пункт про группировку: имена групп и состав chart'ов зависели от порядка обхода map, группа с совпавшим именем перезаписывала другую.
- Покрытие патча (порог codecov ≈ 85.84 %, считается по пакетам): после последних изменений не измерялось. Предыдущий замер — 95 %.

## 3. PR, мёрж, релиз v1.1.0

1. PR `claude/vibrant-hopper-7xilwh` → `main` по шаблону `.github/pull_request_template.md`. Дождаться зелёного CI (Unit, Lint, Golden, CodeQL, codecov/patch).
2. После мёржа — заметки релиза: `docs/RELEASE_NEXT.md` → `docs/RELEASE_v1.1.0.md` (заголовок, дата, тег, по образцу `docs/RELEASE_v1.0.0.md`); новый пустой `RELEASE_NEXT.md`. Отдельный PR или коммит в `main`.
3. Тег: `git tag -a v1.1.0 -m "Release v1.1.0" && git push origin v1.1.0` — workflow `.github/workflows/release.yml` (GoReleaser: бинарники, ghcr.io образ, cosign, SBOM, Homebrew tap; нужен секрет `HOMEBREW_TAP_GITHUB_TOKEN`).
   - Номер `v1.1.0`, а не `v2.0.0`: изменения выхода несовместимые, но путь модуля новый (`github.com/AlexGromer/...`), а `v2+` потребовал бы суффикс `/v2` (см. раздел «Путь модуля» в `RELEASE_NEXT.md`).

## 4. Каталог `docs/roadmap/`: состояние проработки

Правила, DoD и общие контракты — [README.md](README.md). Шаблоны — `_template/`. Реализация задач — **только после явного разрешения владельца**.

| Эпик | Что есть | Чего нет |
|---|---|---|
| EPIC-01 Gateway API | README, spec, design, tasks 01–06 | вычитка; возможно, не все задачи |
| EPIC-02 Kafka | README, spec, design, tasks 01–07 | вычитка; сверка с контрактами EPIC-03 |
| EPIC-03 PBC/продукт | README, spec, design, task 01 | **остальные задачи**; это владелец общих контрактов — дописать первым |
| EPIC-04 CiliumNetworkPolicy | — | всё |
| EPIC-05 Deckhouse app objects | README, spec, design, tasks 01–12 | вычитка; проверка фактов Deckhouse (CRD, scope, метки), помеченных «не проверено» |
| EPIC-06 Регенерация | README, spec, design, tasks 01–02 | остальные задачи |
| EPIC-07 Supply chain | — | всё |
| EPIC-08 Keycloak | — | всё |
| EPIC-09 Data operators (+ долг `autodeps`/Bitnami) | — | всё |
| EPIC-10 Pod DNS | — | всё |
| EPIC-11 Профили генерации | — | всё |

Черновики писали агенты, работа прервалась на середине. Перед использованием:

1. Дописать EPIC-03 (контракты: типы связей, парсер адресов, модель группировки) — от них зависят EPIC-02, 04, 08, 09.
2. Вычитать EPIC-01, 02, 05, 06: факты (версии, apiVersion, URL), ссылки на код (`файл:функция`), атомарность задач, критерии приёмки, соответствие шаблону.
3. Написать EPIC-04, 07, 08, 09, 10, 11 по шаблону. Исходные формулировки направлений — в `BACKLOG.md` и в обсуждении бэклога (граница scope, источники: ingress-nginx retirement 2025-11-11, Bitnami catalog changes 2025-08-28).
4. Заполнить таблицу задач в `BACKLOG.md` (ID `EPIC-NN/NN`, размер, зависимости, статус `ready`/`draft`).
5. Собрать открытые вопросы всех эпиков в один список решений для владельца.
6. Получить разрешение на реализацию; дальше агент работает по разделу 4 `README.md` (одна задача — один PR).

## 5. Отдельные задачи

- **T-01**, **T-02** — в `BACKLOG.md`.
- Квота считает DaemonSet на один узел и не учитывает KEDA ScaledObject (пометки в шаблоне квоты).
