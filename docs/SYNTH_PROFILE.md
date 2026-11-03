# Платформенный профиль синтеза (`--synth-profile`)

Статус: реализовано (ADR-073, ADR-081, ADR-083). Дата: 2026-11-03.

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
| `naming.{deployment,container,service}` | шаблоны | Имена Deployment, контейнера приложения и Service. По умолчанию — имя приложения. Результат проверяется: Deployment — DNS-1123 subdomain, контейнер — DNS-1123 label, Service — DNS-1035 label; пустой результат — ошибка. Остальные объекты (`<name>-env`, `<name>-files`, PVC) и метка `app.kubernetes.io/name` остаются с именем приложения |
| `selector` | map: ключ метки → шаблон значения | **Заменяет** селектор по умолчанию (`app.kubernetes.io/name: <name>`) сразу в `spec.selector.matchLabels` Deployment, в метках шаблона pod'а и в `spec.selector` Service. Метки селектора побеждают одноимённые из `podLabels`. Ключ — допустимое имя метки, значение после подстановки — непустое допустимое значение метки |
| `service.type` | `ClusterIP`, `NodePort`, `LoadBalancer` | Тип Service; без ключа поле не выводится (действует умолчание кластера). `nodePort` не задаётся никогда |
| `service.extraPorts` | список `{name, port, targetPort?, protocol?}` | Порты Service после портов приложения; контейнеру они не добавляются. `name` обязательно (DNS-1123 label), `port` 1–65535, `targetPort` — число или имя порта контейнера (по умолчанию `port`), `protocol` — `TCP` (по умолчанию), `UDP`, `SCTP`. Имена и пары порт/протокол уникальны среди `extraPorts` и портов приложения. Если у приложения нет портов, Service создаётся из `extraPorts` |
| `strategy` | `{type, rollingUpdate: {maxSurge, maxUnavailable}}` | Стратегия Deployment: `type` — `RollingUpdate` (по умолчанию, если задан `rollingUpdate`) или `Recreate` (с `rollingUpdate` несовместим). `maxSurge` и `maxUnavailable` — неотрицательное число или процент (`"25%"`, `maxUnavailable` не больше `100%`); оба нулевыми быть не могут |
| `jobs` | список | Job-хуки Helm, см. ниже |
| `secretKeyPatterns` | список регулярных выражений | **Заменяет** встроенное правило выбора Secret (регистр не важен, совпадение ищется в любом месте имени). Встроенное правило перестаёт действовать целиком: перечислите и стандартные части |
| `dockerfileGlobs` | список масок | Дополнительные места Dockerfile (относительно модуля и корня сборки; абсолютные пути и `..` запрещены) |
| `envTemplateFiles` | список масок | Дополнительные файлы `KEY=VALUE` для значений по умолчанию плейсхолдеров |

Проба: ровно одно из `tcp: true`, `path: <шаблон>` (HTTP GET), `exec: [команда]`; порт — `port` или первый порт приложения; поля `periodSeconds`, `timeoutSeconds`, `failureThreshold`, `successThreshold`, `initialDelaySeconds`.

### Шаблоны

Строки профиля, кроме ключей и имён полей (а также типов, протоколов и значений `strategy`), — шаблоны Go (`text/template`). Данные: `.Name` — имя приложения (DNS-1123), `.Vars` — `vars`. Обращение к переменной, которой нет в `vars`, — ошибка с её именем, а не пустая строка. Функции: `default "x" .Vars.y`, `quote`, `lower`, `upper`, `trim`, `trimPrefix "p" s`, `trimSuffix "s" s`, `replace "a" "b" s`. Целые числа в спецификациях становятся целыми в манифесте.

### Jobs

```yaml
jobs:
  - when: liquibase            # liquibase | flyway | always
    name: "{{ .Vars.prefix }}migrate-{{ .Name }}"
    manifest:                  # маппинг или строка с YAML Job; все строки — шаблоны
      spec: { ... }
```

`when: liquibase` и `when: flyway` срабатывают, когда инструмент найден в соседнем модуле сборки (см. SPEC_SYNTHESIS.md, «Миграции БД»); `always` — всегда. `name` задаёт `metadata.name`, `apiVersion` и `kind` по умолчанию `batch/v1` и `Job`. Если в манифесте нет аннотации `helm.sh/hook`, добавляются `helm.sh/hook: pre-install,pre-upgrade` и `helm.sh/hook-delete-policy: before-hook-creation`; свои аннотации (в том числе `hook-weight`) сохраняются.

## Вывод профиля из стенда: `dhg profile infer`

Профиль не нужно писать с нуля: `dhg profile infer` читает Deployment'ы (и Service'ы, которые их выбирают) работающего стенда или манифестов и пишет профиль из того, что у них общее (ADR-083).

```bash
dhg profile infer -n shop -o profile.yaml --values-out values-shop.yaml
dhg profile infer -s file -f ./manifests --min-share 0.8 -o profile.yaml
```

| Флаг | Значение |
|---|---|
| `-s`, `--source` | `cluster` (по умолчанию; `--context`, `--kubeconfig`) или `file` (`-f` — файлы и каталоги) |
| `-n`, `--namespace`, `-l`, `--selector` | Какие Deployment'ы читать |
| `--min-share` | Доля Deployment'ов (больше 0, не больше 1; по умолчанию `0.9`), у которых свойство должно совпасть, чтобы войти в профиль. Нужное число — `ceil(доля × N)` |
| `-o` | Файл профиля; без флага профиль идёт в stdout |
| `--values-out` | Файл со значениями стенда для переменных профиля (например `values-<namespace>.yaml`) |

Как читать результат:

- **Имя сервиса** — метка `app`, затем `app.kubernetes.io/name`, затем имя Deployment. **Префикс** — самый длинный `p`, для которого достаточно Deployment'ов названы `p`+сервис; он записывается в `vars.prefix`. Если такого `p` нет для доли `--min-share`, но его имеют более половины Deployment'ов, используется самый частый `p` (`Note:` называет долю), чтобы `naming` не получил буквальный префикс стенда; если `p` содержит имя namespace, `Note:` предупреждает, что префикс описывает окружение. Во всех строках `p`+сервис становится `{{ .Vars.prefix }}{{ .Name }}`, затем `p` — `{{ .Vars.prefix }}`, затем сервис — `{{ .Name }}` (только между символами, не являющимися буквой или цифрой).
- **Что входит.** Ключи `initContainers` (образ и остальная спецификация — самые частые; значения, которые подставляет API-сервер, отбрасываются), `envFrom` (без `<имя>-env`: его строит dhg), `volumes` и `volumeMounts` основного контейнера (том входит, если его монтирует вошедшее монтирование или init-контейнер), `podLabels`, `deploymentLabels`, `selector` (ключи, общие для всех), `container` (`imagePullPolicy`, `lifecycle`, `securityContext`), `pod` (`imagePullSecrets`, `nodeSelector`, `securityContext`) — когда значение совпало у нужного числа Deployment'ов. Метки с разными значениями (ветка сборки), `pod-template-hash`, метки Helm и `app.kubernetes.io/name` пропускаются.
- **Самое частое значение.** `defaults.probes`, `defaults.resources`, `strategy`, `service.type`, `service.extraPorts` (порты Service, которых нет у контейнера) и `naming.{deployment,container,service}` берутся как самое частое значение среди Deployment'ов, у которых оно есть (не меньше нужного числа). Если самое частое значение совпало у меньшего числа Deployment'ов, оно используется и выводится `Note:`. Значения по умолчанию (`ClusterIP`, `RollingUpdate` 25%/25%, имена `{{ .Name }}`) не пишутся.
- **Адреса окружения.** IPv4, имена хостов с точкой, jdbc/http URL с хостом (и такие значения после `--флаг=` или `ИМЯ=`) заменяются на `{{ .Vars.<ключ> }}`; ключ — имя переменной окружения (`DB_HOST` → `dbHost`) или контейнер и поле. В URL с шаблоном (`jdbc:postgresql://db.example.org/{{ .Name }}`) переменной становится только хост (`…Host`). В профиле переменная пуста, значения стенда лежат в `--values-out` — файле с одним ключом `vars`, который сам является допустимым профилем; перенесите значения в `vars` профиля для этого стенда. Без `--values-out` значения остаются в `vars` профиля. Распознаются адреса, которые составляют всю строку или стоят после `ключ=`/`--флаг=`, а внутри текста команды (`command`, `args`, скрипт init-контейнера) — только IPv4 и URL как отдельные слова (`until nc -z 10.1.2.3 5432`); имя хоста без схемы внутри текста не распознаётся (чаще это имя файла). **Учётные данные в URL** (`user:password@host`) не попадают ни в профиль, ни в `--values-out`, ни в `vars`: часть `user:password@` удаляется, а `Note: credentials in URL of …` называет место; передайте их chart'у через Secret. Порт Service, который обслуживает порт приложения под другим номером (Service `80` → контейнер `8080`), в профиль не попадает: chart публикует Service на порту приложения, и `Note: Service port 80 -> 8080` говорит об этом (нужен номер стенда — добавьте порт в `service.extraPorts`).
- Файлы (`app.jar`, `config.yaml`), `localhost` и адреса loopback адресами окружения не считаются.
- Не выводятся: аннотации pod'а, sidecar'ы, Jobs, `secretKeyPatterns`, `dockerfileGlobs`, `envTemplateFiles`.
- Заголовок профиля — комментарий с источником и `min-share`; даты нет, поэтому два запуска по одному входу дают одинаковый файл, а порядок входа на результат не влияет.

Профиль — наиболее частая форма Deployment'ов, а не спецификация: просмотрите его, особенно `Note:` о расхождениях, прежде чем применять.

## Что профиль не делает

- Без ключей `naming` и `selector` не меняет имя Deployment, контейнера и Service и их селектор: они остаются `<name>` и `app.kubernetes.io/name`. Ключи `naming` и `selector` нужны, чтобы чарт подключился к уже работающим объектам с другими именами и метками; изменение селектора существующего Deployment кластер не принимает (`spec.selector` неизменяем), поэтому селектор должен совпадать с работающим.
- Не читает ничего вне профиля и проекта, не выполняет команды и не берёт значения секретов.
- Не проверяет, что объекты, на которые он ссылается (ConfigMap, Secret, образы), существуют.

## Профиль и известные ключи

`envFrom` с `configMapRef` определяет, какие ConfigMap'ы читает `dhg generate -s source --known-keys-from-cluster`: имена (после подстановки `.Name` и `.Vars`) запрашиваются из кластера по одному GET в namespace `-n` (иначе namespace контекста kubeconfig, иначе `default`), берутся только имена ключей `data` и `binaryData`. Плейсхолдеры `${NAME}` проекта с такими именами не становятся переменными chart'а, а попадают в заметку «N variables come from <ConfigMap>». Имя должно быть именем объекта API (DNS-1123: строчные буквы, цифры, `-`, `.`): профиль с `configMapRef`/`secretRef`, имя которых (после подстановки) содержит `/`, `..` и т.п., отклоняется; ответ, у которого `kind` не `ConfigMap`, и редирект не используются. `secretRef` в этом не участвует: Secret'ы dhg не читает. Имена, которых нет в кластере (Vault, внешние Secret'ы), задаются файлом `--known-keys` (`источник:KEY`). Новых ключей профиля нет. Без `--synth-profile` флаг `--known-keys-from-cluster` — ошибка.
