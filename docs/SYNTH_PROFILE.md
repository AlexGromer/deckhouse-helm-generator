# Платформенный профиль синтеза (`--synth-profile`)

Статус: реализовано (ADR-073). Дата: 2026-11-02.

Сборка из образа, compose или исходников даёт каркас: Deployment, Service, ConfigMap и Secret с тем, что есть во входе ([SPEC_SYNTHESIS.md](SPEC_SYNTHESIS.md)). Всё, что задаёт платформа организации, в исходниках не лежит: init-контейнеры со служебными скриптами, общие ConfigMap и Secret, метки волн запуска, тайминги проб под прогрев JVM, ресурсы по умолчанию, Job миграций БД. Профиль — YAML-файл с такими данными. Он вне репозитория dhg: dhg содержит механизм, а содержание принадлежит организации.

```bash
dhg generate -s source -f ./orders --image registry.example.com/shop/orders:1.4.0 \
    --chart-name orders --synth-profile ./platform/synth-profile.yaml
```

Флаг работает для `-s image`, `-s compose` и `-s source`; для других источников это ошибка. Как любой флаг, `synth-profile` можно задать в `.dhg.yaml` (`synth-profile: ./platform/synth-profile.yaml`). Нейтральный пример со всеми ключами: [examples/synth-profile.yaml](../examples/synth-profile.yaml).

## Схема

Все ключи необязательны. Неизвестный ключ (на любом уровне), повторяющийся ключ, неверное регулярное выражение или ошибка синтаксиса шаблона — ошибка при загрузке с указанием места. Профиль применяется к каждому приложению перед построением манифестов (`synth.ApplyProfile`), поэтому результат идёт через обычные процессоры и режимы вывода.

| Ключ | Тип | Что делает |
|---|---|---|
| `vars` | map строк | Переменные шаблонов: `{{ .Vars.<имя> }}` |
| `envFrom` | список `{configMapRef: <шаблон>}` или `{secretRef: <шаблон>}` | Добавляются в `envFrom` контейнера **перед** собственными ConfigMap и Secret приложения `<name>-env` (позднее перекрывает раннее) |
| `initContainers` | список спецификаций контейнера | Добавляются в pod; обязательны `name` и `image`; все строки — шаблоны |
| `podLabels`, `podAnnotations` | map | Метки и аннотации шаблона pod'а. Селектор Deployment и Service не меняется |
| `deploymentLabels` | map | Метки самого Deployment |
| `volumes` | список томов | Добавляются в pod (`name` обязателен), строки — шаблоны |
| `volumeMounts` | список монтирований | Добавляются в контейнер приложения (`name` и `mountPath` обязательны) |
| `defaults.resources` | `{requests, limits}` | Ставятся, только если источники ресурсов не дали. Значения вида `cpu: 5` допустимы, в манифесте они строки |
| `defaults.port` | число | Ставится (имя `http`), только если порт не найден |
| `defaults.probes.{startup,liveness,readiness}` | проба | **Заменяет** пробу, выведенную из источников: тайминги и тип проверки задаёт платформа. Не указанная проба остаётся выведенной |
| `jobs` | список | Job-хуки Helm, см. ниже |
| `secretKeyPatterns` | список регулярных выражений | **Заменяет** встроенное правило выбора Secret (регистр не важен, совпадение ищется в любом месте имени). Встроенное правило перестаёт действовать целиком: перечислите и стандартные части |
| `dockerfileGlobs` | список масок | Дополнительные места Dockerfile (относительно модуля и корня сборки; абсолютные пути и `..` запрещены) |
| `envTemplateFiles` | список масок | Дополнительные файлы `KEY=VALUE` для значений по умолчанию плейсхолдеров |

Проба: ровно одно из `tcp: true`, `path: <шаблон>` (HTTP GET), `exec: [команда]`; порт — `port` или первый порт приложения; поля `periodSeconds`, `timeoutSeconds`, `failureThreshold`, `successThreshold`, `initialDelaySeconds`.

### Шаблоны

Строки профиля, кроме ключей и имён полей, — шаблоны Go (`text/template`). Данные: `.Name` — имя приложения (DNS-1123), `.Vars` — `vars`. Обращение к переменной, которой нет в `vars`, — ошибка с её именем, а не пустая строка. Функции: `default "x" .Vars.y`, `quote`, `lower`, `upper`, `trim`, `trimPrefix "p" s`, `trimSuffix "s" s`, `replace "a" "b" s`. Целые числа в спецификациях становятся целыми в манифесте.

### Jobs

```yaml
jobs:
  - when: liquibase            # liquibase | flyway | always
    name: "{{ .Vars.prefix }}migrate-{{ .Name }}"
    manifest:                  # маппинг или строка с YAML Job; все строки — шаблоны
      spec: { ... }
```

`when: liquibase` и `when: flyway` срабатывают, когда инструмент найден в соседнем модуле сборки (см. SPEC_SYNTHESIS.md, «Миграции БД»); `always` — всегда. `name` задаёт `metadata.name`, `apiVersion` и `kind` по умолчанию `batch/v1` и `Job`. Если в манифесте нет аннотации `helm.sh/hook`, добавляются `helm.sh/hook: pre-install,pre-upgrade` и `helm.sh/hook-delete-policy: before-hook-creation`; свои аннотации (в том числе `hook-weight`) сохраняются.

## Что профиль не делает

- Не меняет имя Deployment и Service и их селектор: они остаются `<name>` и `app.kubernetes.io/name`. Подключение к уже работающим объектам с другими именами — отдельная задача.
- Не читает ничего вне профиля и проекта, не выполняет команды и не берёт значения секретов.
- Не проверяет, что объекты, на которые он ссылается (ConfigMap, Secret, образы), существуют.
