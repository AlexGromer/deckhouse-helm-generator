# EPIC-05 / 03: Значения по умолчанию `securityContext` через `global` и исправление `dhg fix`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/01 (для проверки результата в тестах) |
| Требования | R18, R19 (из [spec.md](../spec.md)) |

## Контекст

`dhg fix` (`cmd/dhg/main.go:1439 newFixCmd` → `generator.ApplyAllFixes`, `pkg/generator/autofix.go:322`) обещает «PSS restricted compliance», но:

- `InjectSecurityContext` (`autofix.go:42`) пропускает шаблон, если в нём есть строка `securityContext:` — она есть в **каждом** шаблоне workload'а dhg (`{{- with .podSecurityContext }}` / `{{- with .securityContext }}` в `pkg/processor/k8s/podtemplate.go`), поэтому функция ничего не делает;
- `InjectPSSRestricted` → `InjectPSSDefaults` (`pkg/generator/pss.go:28`) вставляет блок после первой строки с `image:`; первая такая строка — внутри `{{- with .initContainers }}` (`podtemplate.go:231`), поэтому основные контейнеры ничего не получают, а у init-контейнера с собственным `securityContext` появится дублирующийся ключ YAML. Проверено: `dhg fix -f examples/01-simple-web -o out --chart-name web` → `helm template` Deployment'а `nginx` без `securityContext`;
- уровень `baseline` в `pss.go` требует `runAsNonRoot` — это контроль Restricted (F14).

Решение ([design.md §2.5–2.6](../design.md#25-шаблон-podа-задача-03)): шаблон pod'а сливает `securityContext` входа с `global.securityContextDefaults.{pod,container}`; `dhg fix` и возможность `pss-restricted` (задача 04) пишут только эти values.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/k8s/podtemplate.go` | `podTemplate`: блок pod `securityContext` → слияние с `global.securityContextDefaults.pod`; `containerTemplate`: блок `securityContext` контейнера → слияние с `global.securityContextDefaults.container` (оба — для `containers` и `initContainers`, они используют один `containerTemplate`) |
| `pkg/processor/k8s/podtemplate_test.go` (новый) | Тесты текста шаблона |
| `pkg/generator/features.go` | `setGlobalValue(valuesYAML, key string, value interface{}) (string, error)` на `go.yaml.in/yaml/v3` (`yaml.Node`), с сохранением комментариев |
| `pkg/generator/features_test.go` | Тесты `setGlobalValue` |
| `pkg/generator/autofix.go` | `InjectSecurityContext` и `InjectPSSRestricted` заменить на `SetSecurityContextDefaults(chart *types.GeneratedChart, pod, container map[string]interface{}) (*types.GeneratedChart, int, error)`; `ApplyAllFixes` вызывает её один раз со значениями ниже; поля `AutoFixResult.SecurityContextInjected` и `PSSRestrictedApplied` = число шаблонов workload'ов chart'а |
| `pkg/generator/pss.go` | Удалить `InjectPSSDefaults`, `injectSecurityContext`, `restrictedSecurityBlock`, `baselineSecurityBlock`, `restrictedFields`, `baselineFields` (иначе `deadcode` найдёт недостижимое); `isWorkloadTemplate`, `pssWorkloadKinds`, `leadingSpaces` оставить, если используются (`autofix.go`) |
| `pkg/generator/pss_test.go`, `autofix_test.go` | Переписать тесты под новое поведение |
| `cmd/dhg/main.go` | `runFix`: обработать ошибку `ApplyAllFixes` (новая сигнатура возвращает `error`) |
| `tests/golden/deckhouse_test.go` | `TestFixSecurityContext` |
| `README.md` | Строка `dhg fix` в «Остальные команды»: «securityContext через `global.securityContextDefaults`» |
| `docs/RELEASE_NEXT.md` | Исправление `dhg fix` и новый ключ values |

## Шаги

1. Шаблон pod'а — точный текст (отступы как сейчас, `n(…)` — функция отступа в `podTemplate`):
   ```
   {{- with merge (deepCopy (.podSecurityContext | default dict)) (deepCopy ((($.Values.global).securityContextDefaults).pod | default dict)) }}
   securityContext:
     {{- toYaml . | nindent <n> }}
   {{- end }}
   ```
   Контейнер — то же с `.securityContext` и `.container`. `global` присутствует во всех режимах (`pkg/helm/values.go:Build` добавляет `global.imageRegistry`/`imagePullSecrets`; umbrella — `pkg/generator/umbrella.go`), скобки — защита на случай отсутствия ключа.
2. Проверить вручную на всех golden-входах, что рендер без `global.securityContextDefaults` не изменился: `go build -o /tmp/dhg ./cmd/dhg && DHG_REQUIRE_HELM=1 go test ./tests/golden/` (fidelity-проверки сравнивают рендер с входом).
3. `setGlobalValue`: разобрать документ в `yaml.Node`; найти ключ `global` верхнего уровня (создать в конце документа, если нет); если в нём уже есть `key` — ошибка `values.yaml already has global.<key>`; добавить пару ключ/значение (значение — `yaml.Node` из `value`), сериализовать с отступом 2. Комментарии остальных узлов сохраняются (`yaml.v3` хранит `HeadComment`/`LineComment`).
4. `dhg fix` значения (те же поля, что сегодня в `restrictedSecurityBlock`, но в правильном месте):
   - `pod`: `{runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}`;
   - `container`: `{allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}`.
5. Обновить help `dhg fix` (`Long`): «PSS restricted compliance» → «securityContext defaults (global.securityContextDefaults: seccomp RuntimeDefault, no privilege escalation, drop ALL capabilities, runAsNonRoot, read-only root filesystem); values from the input win».

## Тесты

- Unit:
  - `podtemplate_test.go`: шаблон содержит `securityContextDefaults).pod` и `securityContextDefaults).container`; рендер шаблона через `text/template` + Sprig не нужен (в репозитории нет Sprig) — проверяется golden;
  - `features_test.go`: `setGlobalValue` на values с комментарием `# keep` → комментарий сохранён; повторный ключ → ошибка; values без `global` → ключ создан;
  - `autofix_test.go`: `ApplyAllFixes` на chart'е из `generator` с Deployment → values содержат `global.securityContextDefaults.pod.seccompProfile.type: RuntimeDefault`; шаблоны workload'ов не изменены текстово (`strings.Count(tmpl, "securityContext:")` до и после равны).
- Golden (`tests/golden/deckhouse_test.go`):
  - `TestFixSecurityContext`: `dhg fix -f examples/01-simple-web -o out --chart-name web`; `helm template` → у Deployment `nginx`: `spec.template.spec.securityContext.seccompProfile.type == RuntimeDefault`, `runAsNonRoot == true`; у контейнера `nginx`: `allowPrivilegeEscalation == false`, `capabilities.drop == [ALL]`; `pss.Evaluate` отрендеренного pod spec'а → `Restricted`;
  - вход с init-контейнером, у которого есть `securityContext: {runAsUser: 1000}` (`examples/03-batch-processing` или новая мини-фикстура в `tests/golden/testdata/fix-init/`) → у init-контейнера `runAsUser: 1000` (вход победил) и `allowPrivilegeEscalation: false` (слияние), YAML без дублирующихся ключей;
  - все существующие сценарии `TestGeneratedChartsPassHelm` (fidelity) проходят — доказательство, что без defaults рендер не изменился.

## Критерии приёмки

- [ ] AC8 из spec.md выполнен.
- [ ] `deadcode -test ./...` не находит удалённых функций `pss.go`.
- [ ] Значения входа побеждают defaults (тест init-контейнера).
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Возможность `--with pss-restricted` (задача 04). Остальные исправления `dhg fix` (resources, probes, PDB) не меняются.
