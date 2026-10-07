# EPIC-02 / 02: Инъекция env, томов и монтирований в workload'ы из features

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | — |
| Требования | R2 |

## Контекст

Features (ADR-046) добавляют свои шаблоны и top-level ключи values. Единственное изменение чужих шаблонов — подмена строки `{{- with .podAnnotations }}` в `pkg/generator/vaultagent.go:applyVaultAgentFeature`. Подключить приложение к Secret'у (JAAS Kafka, учётные данные CloudNativePG в EPIC-09) feature не может: env и тома контейнера рендерятся из values workload'а (`pkg/processor/k8s/podtemplate.go:podTemplate`, `containerTemplate`).

Шаблон pod'а одинаков для Deployment (`deployment.go`), StatefulSet и DaemonSet (`common.go`), Job (`job.go`), CronJob (`cronjob.go`). Все они вызывают `podTemplate(ctx.ChartName, indent, restartPolicy)`.

Решение (design.md §4.2, ADR-кандидат 2): feature объявляет добавления в своём top-level ключе values (`<key>.inject["<Kind>/<name>"]`), а шаблон pod'а конкатенирует их со своими списками. Значения `inject` проходят через `tpl`, поэтому могут ссылаться на другие values feature.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/k8s/podtemplate.go` | `podTemplate(chartName string, indent int, restartPolicy, workloadKey string) string`. Перед строкой `template:` вывести блок из design.md §4.2 с `<workloadKey>` в `index $dhgBlock.inject "<workloadKey>"`, ключ — через `strconv.Quote`. Каждая строка блока — с отступом `pad` и открывающим `{{-`, без вывода. Цикл по `volumes, nodeSelector, …` — `volumes` рендерить из `concat (.volumes \| default list) $dhgInjVolumes`. `containerTemplate`: первой строкой `{{- $dhgInj := get $dhgInjContainers .name \| default dict }}`; `env`, `envFrom`, `volumeMounts` — `{{- with concat (.<f> \| default list) ($dhgInj.<f> \| default list) }}`. Init-контейнеры используют тот же `containerTemplate`: если имя init-контейнера совпадает с ключом `containers` в `inject`, инъекция применится и к нему. Это задокументировать в комментарии |
| `pkg/processor/k8s/deployment.go` | `podTemplate(ctx.ChartName, 2, "", "Deployment/"+name)`, где `name := obj.GetName()` |
| `pkg/processor/k8s/common.go` | StatefulSet: `"StatefulSet/"+name`; DaemonSet: `"DaemonSet/"+name` |
| `pkg/processor/k8s/job.go`, `cronjob.go` | `"Job/"+name`, `"CronJob/"+name` |
| `pkg/generator/inject.go` (новый) | Типы `Injection`, `ContainerInjection` (design.md §2.2); `func workloadKey(kind, name string) string { return kind + "/" + name }`; `func injectionValues(m map[string]Injection) map[string]interface{}` — детерминированная сериализация (ключи сортируются YAML-маршалингом; пустые списки не выводятся) |
| `docs/DEVELOPER.md` | Раздел 5: правило «feature меняет workload'ы только через `<key>.inject`», пример values |

## Шаги

1. Изменить `podTemplate`/`containerTemplate` и вызовы.
2. Прогнать `go test ./pkg/processor/...`. Тесты, которые сравнивают текст шаблона целиком, обновить: в ожидаемый текст добавляется блок инъекций. Тесты на подстроки (`toYaml . | nindent`) проверить по одному.
3. Добавить `pkg/generator/inject.go` и unit-тесты.
4. Добавить golden-тест (ниже), прогнать весь golden: fidelity и integrity без изменений ожиданий (AC4 эпика).

## Тесты

- **Unit `pkg/processor/k8s/podtemplate_test.go` (новый):**
  - `TestPodTemplateParses`: `podTemplate("app", 2, "", "Deployment/web")` и `podTemplate("app", 6, "OnFailure", "CronJob/x")` разбираются `text/template/parse` с `parse.SkipFuncCheck` (как `cmd/dhg/validate.go:parseTemplate`);
  - `TestPodTemplateInjectionKey`: в тексте есть `index $dhgBlock.inject "Deployment/web"`;
  - `TestContainerTemplateConcat`: в тексте есть `concat (.env | default list) ($dhgInj.env | default list)`, то же для `envFrom` и `volumeMounts`.
- **Unit `pkg/generator/inject_test.go`:** `injectionValues` для `{"Deployment/a": {Volumes: [v], Containers: {"a": {Env: [e]}}}}` даёт YAML `Deployment/a: {volumes: [v], containers: {a: {env: [e]}}}`; пустые `EnvFrom`/`VolumeMounts` отсутствуют.
- **Golden `tests/golden/inject_test.go` (новый) `TestWorkloadInjection`:**
  - `dhg generate -f examples/01-simple-web --chart-name app` в каждом режиме `universal`, `separate`, `umbrella`;
  - затем `helm template` каждого chart'а с workload'ами с файлом values:

    ```yaml
    zzInjectTest:
      enabled: true
      marker: injected
      inject:
        Deployment/<имя Deployment из examples/01-simple-web>:
          volumes: [{name: extra, emptyDir: {}}]
          containers:
            <имя контейнера>:
              env: [{name: DHG_INJECTED, value: '{{ .Values.zzInjectTest.marker }}'}]
              volumeMounts: [{name: extra, mountPath: /extra}]
    ```

  - проверяется: у Deployment есть env `DHG_INJECTED=injected` (`tpl` сработал) после env входа, том `extra` и монтирование `/extra`;
  - с `zzInjectTest.enabled: false` рендер совпадает с рендером без файла.

  Имена Deployment и контейнера тест читает из входного манифеста, а не задаёт литералами.
- **Golden (существующий набор):** проходит без изменения ожиданий.

## Критерии приёмки

- [ ] Без блоков `inject` рендер всех golden-входов не меняется (fidelity/integrity зелёные).
- [ ] Инъекция из values добавляет env, `envFrom`, `volumeMounts` контейнеру и `volumes` pod'у во всех пяти kind'ах workload'ов; значения проходят `tpl`.
- [ ] Блок с `enabled: false` игнорируется.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Инъекция в Argo `Rollout` (`rollout.go` строит шаблон сам) и в шаблоны features (`argo-rollouts`). Отдельная задача по запросу.
- Удаление или замена существующих env: инъекция только добавляет. Дубли имён фильтрует feature (задача 06, design.md §4.4 п. 4).
- Перевод `vault-agent` на этот механизм.
