# Спецификация: генерация chart'а из образа, docker-compose и исходников

Статус: реализовано (ADR-058 – ADR-060). Дата: 2026-10-07.

## Содержание

1. [Контекст](#1-контекст)
2. [Цели и не-цели](#2-цели-и-не-цели)
3. [Архитектура](#3-архитектура)
4. [Модель приложения](#4-модель-приложения)
5. [Источник `image`](#5-источник-image)
6. [Источник `compose`](#6-источник-compose)
7. [Источник `source`](#7-источник-source)
8. [Построение манифестов](#8-построение-манифестов)
9. [Отчёт `SYNTHESIS.md`](#9-отчёт-synthesismd)
10. [CLI](#10-cli)
11. [Безопасность](#11-безопасность)
12. [Проверка и критерии приёмки](#12-проверка-и-критерии-приёмки)
13. [План реализации](#13-план-реализации)
14. [Trade-offs и развитие](#14-trade-offs-и-развитие)

---

## 1. Контекст

До этой спецификации dhg принимал на вход только манифесты Kubernetes: файлы, живой кластер или git-репозиторий. Он переупаковывал уже описанный деплой в Helm, но не мог описать деплой сам.

Типичная ситуация: у сервиса есть образ в registry, `docker-compose.yml` для локального запуска или исходники (Spring Boot), но манифестов нет. Их пишут руками, и ошибки повторяются: не тот порт, нет probes, контейнер работает от root.

## 2. Цели и не-цели

**Цели:**

- **Новые источники:** `image`, `compose`, `source`. Каждый строит *синтетические манифесты* из фактов, которые есть во входных данных.
- **Тот же конвейер.** Синтетические манифесты проходят processors → analyzer → generator → features без изменений. Все режимы (`universal`/`separate`/`library`/`umbrella`), флаги и `--with` работают как для манифестов из файлов.
- **Только факты.** Каждое значение chart'а берётся из входа. Чего во входе нет (requests/limits, реплики, хосты Ingress, пароли), то не выдумывается: попадает в отчёт `SYNTHESIS.md` как «заполнить».
- **Без новых зависимостей**, как в ADR-051: registry API через `net/http`, compose и Spring-конфиги через уже подключённый `sigs.k8s.io/yaml`. Compose читается его подпакетом `sigs.k8s.io/yaml/goyaml.v3` (YAML 1.2, как Docker Compose v2: сервис `off` или значение `yes` остаются строками), `application.yml` — основным пакетом (YAML 1.1, как SnakeYAML в Spring Boot).

**Не-цели:**

- Анализ слоёв образа (скачивание и распаковка файловой системы): дорого, хрупко, мало пользы по сравнению с исходниками.
- Сборка образов (`build:` в compose, Dockerfile): dhg описывает деплой, а не CI.
- Полная семантика compose: `networks`, `secrets`/`configs` верхнего уровня, `profiles`, `extends`, `deploy.placement`. Неподдержанные ключи попадают в отчёт.
- Языки и фреймворки, кроме Spring Boot. Остальные проекты обрабатываются через Dockerfile.

## 3. Архитектура

```
 image ref ──► registry v2 API ──► OCI config ─┐
 compose.yml ──► compose parser ───────────────┼──► AppModel[] ──► manifests ──► ExtractedResource ──► (существующий конвейер)
 dir ──► Dockerfile + Spring Boot detector ────┘        │
                                                         └──► notes ──► SYNTHESIS.md
```

- `pkg/synth` — модель `App`, построение манифестов и отчёт. Ничего не знает о том, откуда пришли данные.
- `pkg/synth/registry` — минимальный клиент Docker Registry HTTP API v2 / OCI Distribution.
- `pkg/extractor/{image,compose,source}.go` — экстракторы. Превращают вход в `[]synth.App` и отдают манифесты через стандартный интерфейс `Extractor`.

Экстракторы реализуют дополнительный интерфейс `extractor.Reporter` (`Notes() []string`). Конвейер собирает заметки, CLI печатает их и пишет в `SYNTHESIS.md`.

## 4. Модель приложения

`synth.App` описывает один сервис:

| Поле | Смысл |
|---|---|
| `Name` | DNS-1123 имя: Deployment, Service, метка `app.kubernetes.io/name` |
| `Image` | ссылка на образ |
| `Command`, `Args` | только если вход их явно переопределяет (compose `entrypoint`/`command`) |
| `WorkingDir` | только если переопределён во входе |
| `Ports[]` | `{Name, Port, Protocol}` — порт контейнера; Service публикует тот же номер. Имя `http` — только при известном протоколе (Spring Boot), `management` — порт actuator, иначе `tcp-<порт>`/`udp-<порт>` |
| `Env[]` | `{Name, Value}` → ConfigMap `<name>-env` |
| `SecretEnv[]` | `{Name, Value}` → Secret `<name>-env`; значение пустое, если во входе его нет |
| `User` | `uid[:gid]` из образа, Dockerfile или compose |
| `Volumes[]` | `{Name, MountPath, Kind: emptyDir/pvc/configFile, Size, Content}` |
| `Liveness`, `Readiness` | `{HTTPPath, Port \| Exec []string, Period, Timeout, Failures, InitialDelay}` |
| `Replicas` | только из compose `deploy.replicas`, иначе 1 |
| `Resources` | только из compose `deploy.resources` |
| `Labels` | дополнительные метки pod'а |

## 5. Источник `image`

**Вход:** `--image <ref>` (можно несколько), `--platform` (по умолчанию `linux/amd64`), `--insecure-registry`.

**Разбор ссылки** — по правилам Docker:

| Ссылка | Registry | Репозиторий | Ссылка на манифест |
|---|---|---|---|
| `nginx` | `registry-1.docker.io` | `library/nginx` | `latest` |
| `org/app:1.2` | `registry-1.docker.io` | `org/app` | `1.2` |
| `registry.deckhouse.ru/team/app:1.2` | `registry.deckhouse.ru` | `team/app` | `1.2` |
| `localhost:5000/app@sha256:…` | `localhost:5000` | `app` | digest |

**Протокол:**

1. `GET /v2/<repo>/manifests/<ref>` с `Accept` для OCI index/manifest и Docker manifest list/v2.
2. Если пришёл index или manifest list — выбрать манифест для `--platform`.
3. `GET /v2/<repo>/blobs/<config digest>`.

**Аутентификация:**

- на `401` с `WWW-Authenticate: Bearer realm,service,scope` запрашивается токен (анонимно или с учётными данными);
- на `Basic` используются сами учётные данные;
- учётные данные берутся из `$DOCKER_CONFIG/config.json` или `~/.docker/config.json` в порядке: `credHelpers.<host>` → `auths.<host>` → `credsStore`. Credential helper запускается по протоколу docker-credential-helpers: `docker-credential-<имя> get` из `PATH`, без shell, с таймаутом 30 с; на stdin — хост registry (для Docker Hub — `https://index.docker.io/v1/`), на stdout — JSON `{"ServerURL","Username","Secret"}`. Пустое значение `credHelpers.<host>` означает «только `auths`», как в Docker CLI. Отличие от Docker CLI: при заданном `credsStore` Docker не читает секреты из `auths` (сам он оставляет там пустые записи), dhg использует непустую запись `auths` раньше `credsStore` и не запускает helper;
- результат запоминается для хоста на время запуска; helper вызывается не больше одного раза на хост;
- helper не найден, завершился с ошибкой, ответил «credentials not found», вернул некорректный JSON или identity token (`Username: "<token>"`, OAuth2 refresh token — не поддерживается) — учётных данных нет, запрос идёт анонимно, причина пишется в отчёт, а при ошибке запроса — в текст ошибки.

**Правила вывода** (поля спецификации OCI image config):

| Поле config | Результат | Примечание |
|---|---|---|
| `ExposedPorts` | `containerPort` и порт Service | протокол из суффикса `/tcp`, `/udp` |
| `User` | `runAsUser`, `runAsGroup`; `runAsNonRoot: true` при uid ≠ 0 | имя пользователя (не число) → в отчёт: kubelet не проверит non-root по имени |
| `Volumes` | `emptyDir` | в отчёт: нужна ли персистентность |
| `Healthcheck` (расширение Docker) | exec liveness и readiness | `Interval`, `Timeout`, `Retries`, `StartPeriod` |
| `Labels["org.opencontainers.image.title"]` | имя приложения | иначе — последний сегмент репозитория |
| `Env`, `Entrypoint`, `Cmd`, `WorkingDir` | не копируются: уже есть в образе | имена переменных — в отчёт, как кандидаты в values |

## 6. Источник `compose`

**Вход:** `-f docker-compose.yml`, можно несколько файлов; поддерживаются форматы v2 и v3 и спецификация Compose. Файлы разбираются по YAML 1.2 (см. раздел 2).

| Ключ сервиса | Результат |
|---|---|
| `image` | образ; при одном `build:` → `<service>:latest` + заметка |
| `ports` (`"8080"`, `"80:8080"`, `"127.0.0.1:80:8080/udp"`, длинный синтаксис) | порт контейнера = target; host-порт → заметка |
| `expose` | порты |
| `environment` (map или список) | переменные; имена с `PASSWORD`/`SECRET`/`TOKEN`/`KEY`/`CREDENTIAL` → `SecretEnv` |
| `env_file` | переменные из файла (относительно compose-файла) |
| `entrypoint`, `command` (строка или список) | `command`/`args` |
| `user`, `working_dir` | `securityContext`, `workingDir` |
| `volumes`: именованный том | PVC `1Gi` `ReadWriteOnce` + заметка про размер |
| `volumes`: bind существующего файла ≤ 1 MiB | ConfigMap с содержимым, монтирование через `subPath` |
| `volumes`: bind каталога или отсутствующего файла | `emptyDir` + заметка |
| `healthcheck` (`CMD`, `CMD-SHELL`, `NONE`) | exec probes |
| `deploy.replicas`, `deploy.resources.{limits,reservations}` | `replicas`, `resources` |
| `depends_on` | заметка: Kubernetes не упорядочивает запуск, нужны readiness или retry |
| `networks`, `restart`, `profiles`, `extends`, `secrets`, `configs`, … | заметка «не перенесено» |

**Подстановка переменных:** поддерживаются `${VAR}`, `${VAR:-default}`, `${VAR-default}` и `$VAR` из окружения процесса; `$$` — экранированный `$`. Неразрешённая переменная без значения по умолчанию → пустая строка и заметка.

**Сетевая модель.** Service называется по имени сервиса compose. Поэтому адреса вида `db:5432` из переменных окружения продолжают работать внутри namespace релиза.

## 7. Источник `source`

**Вход:** `-f <каталог проекта>`, `--image` (необязательно: ссылка на образ приложения).

**Dockerfile** (последняя стадия multi-stage):

| Инструкция | Результат |
|---|---|
| `EXPOSE` | порты |
| `USER` | `securityContext` |
| `HEALTHCHECK` | exec probes |
| `VOLUME` | `emptyDir` + заметка |

**Spring Boot** определяется по `spring-boot` в `pom.xml` или `build.gradle(.kts)`.

| Свойство (`application.yml`, `.yaml`, `.properties` в `src/main/resources`) | Результат |
|---|---|
| `spring.application.name` → `artifactId` → `rootProject.name` → имя каталога | имя приложения |
| `server.port` (с поддержкой `${PORT:8080}`), иначе 8080 | порт `http` |
| `spring-boot-starter-actuator` в зависимостях | HTTP liveness `…/health/liveness` и readiness `…/health/readiness`. Пути учитывают `management.endpoints.web.base-path`, `management.server.port` и `server.servlet.context-path` (контекстный путь применяется, только когда management-порт совпадает с основным) |
| `spring.datasource.url`, `.username` | env `SPRING_DATASOURCE_URL`, `SPRING_DATASOURCE_USERNAME` |
| `spring.datasource.password` | `SecretEnv` `SPRING_DATASOURCE_PASSWORD`, пустое значение + заметка |
| `spring.kafka.bootstrap-servers` | env `SPRING_KAFKA_BOOTSTRAP_SERVERS` |
| `spring.security.oauth2.resourceserver.jwt.issuer-uri` | env `SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI` (Keycloak) |

Переменные передаются через relaxed binding Spring Boot: env-переменная перекрывает свойство с тем же смыслом. Значение по умолчанию совпадает с тем, что уже лежит в jar; в chart оно выносится, чтобы его можно было менять per-environment через values.

Профили (`spring.config.activate.on-profile`, `spring.profiles`) не читаются: используется документ без профиля, остальное → заметка.

Образ: `--image`, иначе `<name>:<version|latest>` + заметка.

## 8. Построение манифестов

Для каждого `App` строятся следующие объекты:

- **Deployment** `<name>`:
  - селектор и метки pod'а — `app.kubernetes.io/name: <name>`;
  - контейнер `<name>`;
  - `envFrom` ссылается на ConfigMap и Secret, если они есть;
  - тома и `securityContext` — из модели.
- **Service** `<name>` (ClusterIP) — если есть порты.
- **ConfigMap** `<name>-env`, **Secret** `<name>-env` — если есть переменные.
- **ConfigMap** `<name>-files` — для bind-файлов из compose.
- **PersistentVolumeClaim** `<name>-<volume>` — для именованных томов.

Дальше это обычные входные объекты. Поэтому ADR-049 (сохранение имён), ADR-053 (fidelity) и все features применяются к ним без особых случаев.

## 9. Отчёт `SYNTHESIS.md`

При синтетическом источнике CLI пишет `<output>/SYNTHESIS.md` и печатает каждую заметку в stderr с префиксом `Note:`.

Отчёт содержит:
- исходные данные (образ с digest, файлы);
- список объектов;
- что заполнить вручную (resources, пароли, размеры томов, Ingress);
- что не перенесено.

## 10. CLI

```bash
dhg generate -s image   --image registry.example.com/team/api:1.4.2 --chart-name api
dhg generate -s compose -f docker-compose.yml --chart-name shop --mode separate
dhg generate -s source  -f ./orders-service --image registry.example.com/orders:2.0 --chart-name orders
```

| Флаг | Источник | Значение |
|---|---|---|
| `--image` (повторяемый) | `image`, `source` | ссылка на образ |
| `--platform` | `image` | `os/arch[/variant]`, по умолчанию `linux/amd64` |
| `--insecure-registry` | `image` | HTTP вместо HTTPS (локальный registry) |

## 11. Безопасность

- Пароли и токены никогда не копируются в values из Spring-конфигов: в `SecretEnv` попадает пустое значение.
- Из compose значение копируется как есть: это то, что пользователь сам записал в файл. В отчёте предупреждение: заменить перед коммитом chart'а.
- Учётные данные registry используются только для запросов к этому же registry. Токен не записывается никуда.
- Credential helper запускается без shell (`exec`, бинарь `docker-credential-<имя>` ищется в `PATH`, имя с разделителем пути отклоняется) с таймаутом 30 с. Ни его вывод, ни секреты не попадают в ошибки и отчёт: при ошибке helper'а приводится только первая строка его сообщения (не длиннее 200 символов), при некорректном JSON — только факт.
- Ответы registry ограничены по размеру: манифест ≤ 4 MiB, config ≤ 4 MiB. Digest config-блоба сверяется с полученными байтами.
- Bind-файлы из compose читаются только в пределах размера 1 MiB.

## 12. Проверка и критерии приёмки

- **Unit-тесты:**
  - разбор ссылок на образы;
  - протокол registry на `httptest`: index → manifest → config, Bearer-токен, проверка digest;
  - разбор compose: все формы `ports`, `environment`, подстановка переменных;
  - разбор Dockerfile;
  - определение Spring Boot, вывод путей probes.
- **Golden:** chart из каждого источника проходит `helm lint --strict` и `helm template`, а отрендеренные объекты содержат выведенные факты:
  - `image` — fake registry;
  - `compose` — фикстура с тремя сервисами;
  - `source` — фикстура Spring Boot с Dockerfile.
- **Критерии приёмки:**
  1. Каждый источник даёт chart, который проходит Helm.
  2. Ни одно значение не выдумано: всё, что не выведено, есть в `SYNTHESIS.md`.
  3. Существующие источники и тесты не изменились.

## 13. План реализации

| Этап | Содержание | Результат |
|---|---|---|
| 1 | `pkg/synth`: модель, построение манифестов, заметки | `App` → объекты Kubernetes |
| 2 | Клиент registry + экстрактор `image` | `-s image` |
| 3 | Разбор compose + экстрактор `compose` | `-s compose` |
| 4 | Dockerfile + Spring Boot + экстрактор `source` | `-s source` |
| 5 | CLI, `SYNTHESIS.md`, README, ADR-058 – ADR-060 | документированная функция |
| 6 | Golden-тесты для трёх источников | проверка настоящим Helm в CI |

## 14. Trade-offs и развитие

- **Без `go-containerregistry`.** Минус зависимость и контроль над размером ответов. Цена — нет схем аутентификации, кроме Basic/Bearer, и identity token'ов credential helper'ов (OAuth2 refresh token). Для Docker Hub, Harbor, GitLab, Nexus и Deckhouse registry этого достаточно; credential helpers (`credsStore`, `credHelpers`) поддержаны без библиотеки — по их протоколу stdin/stdout.
- **Порт Service = порт контейнера.** Модель адресации внутри кластера остаётся той же, что в compose (`db:5432`). Host-порты compose в кластере не нужны.
- **Не задаются requests/limits.** Это сознательное решение: выдуманные числа хуже их отсутствия, потому что ломают планирование и капасити. Отчёт требует их заполнить.

**Возможное развитие:**
- `-s source` с `--image`, читающий config образа, — объединение фактов Dockerfile и образа;
- Quarkus и Micronaut;
- профили Spring как values-оверлеи `values-<profile>.yaml`.
