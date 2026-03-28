# Участие в разработке Deckhouse Helm Generator

Спасибо за ваш интерес к участию в проекте!

## Начало работы

### Требования

- Go 1.22+
- Make
- Helm 3.x (для E2E-тестов)

### Настройка окружения

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
make test
```

## Рабочий процесс разработки

1. Сделайте форк репозитория
2. Создайте ветку для функциональности: `git checkout -b feature/my-feature`
3. Внесите изменения
4. Запустите тесты: `make test`
5. Запустите линтер: `make lint` (или `golangci-lint run`)
6. Сделайте коммит с описательным сообщением
7. Отправьте изменения и откройте Pull Request

## Стиль кода

- Следуйте стандартным соглашениям Go (`gofmt`, `goimports`)
- Все экспортируемые функции должны содержать комментарии
- Используйте табличные тесты (table-driven tests)
- Обрабатывайте все ошибки (линтер проверяет `errcheck`)

## Структура проекта

```
cmd/dhg/          # Точка входа CLI
pkg/
  analyzer/       # Обнаружение связей между ресурсами Kubernetes
  extractor/      # Извлечение ресурсов из YAML/директорий
  generator/      # Генерация Helm-чартов
  helm/           # Модель Helm-чарта и рендеринг
  processor/      # Обработка по типам ресурсов (k8s/, deckhouse/)
tests/
  e2e/            # Сквозные тесты с Helm lint
  integration/    # Интеграционные тесты для пайплайнов
```

## Добавление нового процессора

1. Создайте `pkg/processor/k8s/<resource>.go` (или `deckhouse/`)
2. Реализуйте интерфейс `Processor`
3. Зарегистрируйте в `pkg/processor/registry.go`
4. Добавьте модульные тесты в `<resource>_test.go`
5. При необходимости добавьте тестовые фикстуры в `pkg/testutil/fixtures/`

## Тестирование

```bash
make test              # Модульные тесты
make test-integration  # Интеграционные тесты
make test-e2e          # Сквозные тесты
make test-all          # Все тесты
make coverage          # Отчёт о покрытии
```

## Рекомендации по Pull Request

- Делайте PR сфокусированными на одном изменении
- Полностью заполняйте шаблон PR
- Убедитесь, что все проверки CI проходят
- Обновляйте документацию при изменении поведения
- Добавляйте тесты для новой функциональности

## Сообщения коммитов

Следуйте формату Conventional Commits:

```
feat: add support for Argo Rollouts
fix: handle empty ConfigMap data field
docs: update processor development guide
chore: bump golangci-lint to v1.62
test: add edge cases for Secret processor
```

## Сообщение о проблемах

- Используйте [Отчёт об ошибке](.github/ISSUE_TEMPLATE/bug_report.md) для багов
- Используйте [Запрос на функциональность](.github/ISSUE_TEMPLATE/feature_request.md) для новых возможностей
- Проверьте существующие issues перед созданием новых

## Лицензия

Участвуя в проекте, вы соглашаетесь с тем, что ваши вклады будут лицензированы на условиях Apache License 2.0.
