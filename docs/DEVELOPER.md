# Руководство разработчика — DHG

> **Тип:** How-To
> **Аудитория:** Go-разработчики среднего уровня, участвующие в разработке DHG
> **Последнее обновление:** 2026-03-30
> **Связанные документы:** [ARCHITECTURE.md](../ARCHITECTURE.md), [ADR.md](ADR.md)

## Обзор

Это руководство объясняет, как настроить среду разработки и как расширить DHG, добавив новые process-обработчики ресурсов, детекторы связей и генераторы chart. Все три точки расширения следуют паттерну plugin-registry (ADR-003) — вы создаёте struct, реализующий интерфейс, и регистрируете его; пайплайн подхватывает его автоматически.

---

## 1. Предварительные требования

| Инструмент | Версия | Назначение |
|-----------|--------|-----------|
| Go | 1.26+ | Сборка и тестирование |
| Make | любая | Build targets (`make build`, `make test`) |
| golangci-lint | v1.62+ | Lint (`make lint`) |
| Helm | 3.x | Integration и e2e тесты |
| git | любая | Контроль версий |

Установка Go 1.26:

```bash
# Linux AMD64
curl -LO https://go.dev/dl/go1.26.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Установка golangci-lint:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b $(go env GOPATH)/bin
```

Клонирование и сборка:

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
# Бинарный файл: ./bin/dhg
```

---

## 2. Структура проекта

```
deckhouse-helm-generator/
├── cmd/dhg/
│   ├── main.go              # CLI (Cobra root + все subcommands), оркестрация пайплайна
│   └── main_test.go         # Интеграционные тесты CLI
├── pkg/
│   ├── analyzer/            # Обнаружение паттернов, граф связей, генерация DOT
│   │   ├── analyzer.go      # Ядро Analyzer, точка входа Analyze()
│   │   ├── graph.go         # GenerateDOTGraph(), обнаружение циклических зависимостей
│   │   ├── detector/        # Детекторы связей (label, reference, annotation, volume, deckhouse)
│   │   └── pattern/         # Проверки паттернов (11), детекторы паттернов (6), Recommender, Formatter
│   ├── extractor/           # Извлечение ресурсов из YAML-файлов, cluster, gitops
│   │   ├── extractor.go     # Интерфейс Extractor + файловая реализация
│   │   ├── cluster.go       # Cluster extractor (динамический client клиент-go)
│   │   ├── gitops.go        # GitOps extractor (go-git)
│   │   └── merger.go        # Дедупликация из нескольких источников и разрешение конфликтов
│   ├── generator/           # Генерация Helm chart (70+ файлов генераторов)
│   │   ├── generator.go     # Интерфейс Generator, DefaultRegistry(), Options
│   │   ├── universal.go     # Режим Universal (единый chart)
│   │   ├── separate.go      # Режим Separate (chart для каждого сервиса)
│   │   ├── library.go       # Режим Library chart
│   │   ├── umbrella.go      # Режим Umbrella chart
│   │   └── ...              # 60+ phase-специфичных генераторов
│   ├── processor/           # Интерфейс Processor и registry
│   │   ├── processor.go     # Интерфейс Processor, Context, Result, BaseProcessor
│   │   ├── registry.go      # Registry — маршрутизация на основе GVK
│   │   └── k8s/             # 50 процессоров по типам ресурсов
│   ├── helm/                # Модели данных Chart.yaml и values.yaml
│   └── types/               # Общие типы (ExtractedResource, ProcessedResource, GeneratedChart и др.)
├── tests/
│   ├── integration/         # Полные тесты пайплайна с реальными YAML-fixtures
│   └── e2e/                 # End-to-end тесты (generate + helm lint)
├── Makefile
├── .goreleaser.yml
└── .golangci.yml
```

### Этапы пайплайна (из `cmd/dhg/main.go`)

```
[1] Extract   → extractor.Extract()     → []ExtractedResource
[2] Process   → processor.Process()     → []ProcessedResource
[3] Analyze   → analyzer.Analyze()      → ResourceGraph
[4] Generate  → generator.Generate()    → []GeneratedChart
[4b..4j]      → Post-processor'ы Phase 2 (copy-on-write)
[4k..4t]      → Security post-processor'ы Phase 2.5
[5] Write     → generator.WriteChart()  → filesystem
```

---

## 3. Добавление нового процессора K8s-ресурсов

Процессор преобразует один объект `*unstructured.Unstructured` в Helm-шаблон и фрагмент `values.yaml`.

### 3.1 Создайте файл процессора

Создайте `pkg/processor/k8s/mykind.go`:

```go
package k8s

import (
    "errors"
    "fmt"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// MyKindProcessor processes MyKind resources.
type MyKindProcessor struct {
    processor.BaseProcessor
}

// NewMyKindProcessor creates a new MyKind processor.
func NewMyKindProcessor() *MyKindProcessor {
    return &MyKindProcessor{
        BaseProcessor: processor.NewBaseProcessor(
            "mykind",
            100, // priority — lower number = higher priority
            schema.GroupVersionKind{
                Group:   "example.io",
                Version: "v1",
                Kind:    "MyKind",
            },
        ),
    }
}

// Process converts a MyKind resource into a Helm template and values.
func (p *MyKindProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
    if obj == nil {
        return nil, errors.New("mykind object is nil")
    }

    serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
    if serviceName == "" {
        serviceName = obj.GetName()
    }

    // Extract values you want to expose in values.yaml
    values := map[string]interface{}{
        "enabled": true,
    }

    // Example: read a spec field
    if field, found, _ := unstructured.NestedString(obj.Object, "spec", "myField"); found {
        values["myField"] = field
    }

    // Build the Helm template
    template := fmt.Sprintf(`apiVersion: example.io/v1
kind: MyKind
metadata:
  name: {{ include "%s.fullname" . }}-mykind
  labels:
    {{- include "%s.labels" . | nindent 4 }}
spec:
  myField: {{ .Values.%s.mykind.myField | quote }}
`, ctx.ChartName, ctx.ChartName, serviceName)

    return &processor.Result{
        Processed:       true,
        ServiceName:     serviceName,
        TemplatePath:    fmt.Sprintf("templates/%s-mykind.yaml", serviceName),
        TemplateContent: template,
        ValuesPath:      fmt.Sprintf("services.%s.mykind", serviceName),
        Values:          values,
    }, nil
}
```

### 3.2 Зарегистрируйте процессор

Откройте `pkg/processor/k8s/registry.go` и добавьте одну строку в `RegisterAll()`:

```go
func RegisterAll(r *processor.Registry) {
    // ... существующие регистрации ...
    r.Register(NewMyKindProcessor())   // добавьте эту строку
}
```

### 3.3 Напишите тесты

Создайте `pkg/processor/k8s/mykind_test.go`. Используйте тот же паттерн, что и в других `_test.go` файлах пакета — создайте `*unstructured.Unstructured`, вызовите `Process()` и проверьте `TemplatePath`, `Values` и `TemplateContent`:

```go
package k8s

import (
    "testing"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestMyKindProcessor_Process(t *testing.T) {
    p := NewMyKindProcessor()

    obj := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "example.io/v1",
            "kind":       "MyKind",
            "metadata": map[string]interface{}{
                "name":      "my-resource",
                "namespace": "default",
            },
            "spec": map[string]interface{}{
                "myField": "hello",
            },
        },
    }

    ctx := processor.Context{
        ChartName:  "testchart",
        OutputMode: types.OutputModeUniversal,
    }

    result, err := p.Process(ctx, obj)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if result.TemplatePath != "templates/my-resource-mykind.yaml" {
        t.Errorf("unexpected template path: %s", result.TemplatePath)
    }

    if result.Values["myField"] != "hello" {
        t.Errorf("unexpected values: %v", result.Values)
    }
}
```

### 3.4 Проверьте

```bash
go test ./pkg/processor/k8s/... -run TestMyKind -v
```

Ожидаемый вывод:

```
--- PASS: TestMyKindProcessor_Process (0.00s)
PASS
```

---

## 4. Добавление нового детектора связей

Детекторы запускаются на этапе 3 (Analyze). Они сканируют `[]ProcessedResource` и добавляют структуры `Relationship` в `ResourceGraph`. Используйте их для моделирования связей типа Service → Deployment или Certificate → Ingress.

### 4.1 Создайте файл детектора

Создайте `pkg/analyzer/detector/mydetector.go`:

```go
package detector

import (
    "github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MyDetector detects relationships between MyKind and Deployment resources.
type MyDetector struct{}

// Name returns the detector's identifier.
func (d *MyDetector) Name() string { return "my-detector" }

// Detect scans all processed resources and records relationships.
func (d *MyDetector) Detect(resources []*types.ProcessedResource, graph *analyzer.ResourceGraph) {
    for _, res := range resources {
        if res.Original.Object.GetKind() != "MyKind" {
            continue
        }

        // Example: find a Deployment with the same name prefix
        for _, other := range resources {
            if other.Original.Object.GetKind() != "Deployment" {
                continue
            }

            if res.ServiceName == other.ServiceName {
                graph.AddRelationship(analyzer.Relationship{
                    From:     res.Original.ResourceKey(),
                    To:       other.Original.ResourceKey(),
                    Type:     "MyKind->Deployment",
                    Strength: 1.0,
                })
            }
        }
    }
}
```

### 4.2 Зарегистрируйте детектор

Откройте `pkg/analyzer/detector/registry.go` и добавьте ваш детектор в `RegisterAll()`:

```go
func RegisterAll(a *analyzer.Analyzer) {
    // ... существующие детекторы ...
    a.RegisterDetector(&MyDetector{})
}
```

### 4.3 Напишите тесты

```go
package detector

import (
    "testing"
    // ... импорты
)

func TestMyDetector_Detect(t *testing.T) {
    // создайте два ProcessedResource (MyKind + Deployment с одинаковым именем сервиса)
    // вызовите Detect()
    // проверьте, что graph.Relationships содержит одну запись с правильным Type
}
```

---

## 5. Добавление нового генератора

Генераторы запускаются после этапа Analyze как post-processor'ы. Они получают `*GeneratedChart` и возвращают новый (copy-on-write — ADR-008). Используйте их для добавления новых файлов шаблонов или записей в values.

### 5.1 Понять контракт copy-on-write

Каждый генератор, модифицирующий `GeneratedChart`, **обязан**:

1. Создать новый `map[string]string` для `Templates`.
2. Скопировать все записи из `chart.Templates` в новую map.
3. Добавить или изменить записи в новой map.
4. Вернуть новый `*types.GeneratedChart` с новой map.

Не изменяйте `chart.Templates` напрямую.

### 5.2 Создайте файл генератора

Создайте `pkg/generator/myfeature.go`:

```go
package generator

import (
    "fmt"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MyFeatureConfig holds configuration for MyFeature generation.
type MyFeatureConfig struct {
    Enabled bool
    Label   string
}

// InjectMyFeature adds a ConfigMap template that exposes MyFeature settings.
// It follows the copy-on-write contract (ADR-008).
func InjectMyFeature(chart *types.GeneratedChart, cfg MyFeatureConfig) *types.GeneratedChart {
    if !cfg.Enabled {
        return chart
    }

    // Step 1: new templates map
    templates := make(map[string]string, len(chart.Templates)+1)

    // Step 2: copy existing templates
    for k, v := range chart.Templates {
        templates[k] = v
    }

    // Step 3: add new template
    templates["templates/myfeature-configmap.yaml"] = fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "%s.fullname" . }}-myfeature
  labels:
    {{- include "%s.labels" . | nindent 4 }}
data:
  label: {{ .Values.myfeature.label | quote }}
`, chart.Name, chart.Name)

    // Step 4: return new chart struct
    return &types.GeneratedChart{
        Name:          chart.Name,
        Path:          chart.Path,
        ChartYAML:     chart.ChartYAML,
        ValuesYAML:    chart.ValuesYAML,
        Templates:     templates,
        Helpers:       chart.Helpers,
        Notes:         chart.Notes,
        ValuesSchema:  chart.ValuesSchema,
        ExternalFiles: chart.ExternalFiles,
    }
}
```

### 5.3 Подключите генератор к пайплайну

Откройте `cmd/dhg/main.go`. В `runGenerate()` добавьте переменную флага в начало `newGenerateCmd()`:

```go
var myFeature bool
// ...
cmd.Flags().BoolVar(&myFeature, "my-feature", false, "Generate MyFeature ConfigMap")
```

Затем примените генератор после генерации базового chart (следуя существующему паттерну Phase 2 post-processor):

```go
if opts.myFeature {
    cfg := generator.MyFeatureConfig{Enabled: true, Label: opts.chartName}
    for i, chart := range charts {
        charts[i] = generator.InjectMyFeature(chart, cfg)
    }
}
```

### 5.4 Напишите тесты

Создайте `pkg/generator/myfeature_test.go`:

```go
package generator

import (
    "strings"
    "testing"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestInjectMyFeature_AddsConfigMap(t *testing.T) {
    chart := &types.GeneratedChart{
        Name:      "testchart",
        Templates: map[string]string{},
    }

    cfg := MyFeatureConfig{Enabled: true, Label: "test-label"}
    result := InjectMyFeature(chart, cfg)

    const key = "templates/myfeature-configmap.yaml"
    content, ok := result.Templates[key]
    if !ok {
        t.Fatalf("expected template %s not found", key)
    }

    if !strings.Contains(content, "kind: ConfigMap") {
        t.Errorf("template does not contain ConfigMap kind")
    }
}

func TestInjectMyFeature_Disabled_ReturnsOriginal(t *testing.T) {
    chart := &types.GeneratedChart{
        Name:      "testchart",
        Templates: map[string]string{},
    }

    result := InjectMyFeature(chart, MyFeatureConfig{Enabled: false})
    if result != chart {
        t.Error("expected original chart to be returned when disabled")
    }
}
```

---

## 6. Запуск тестов

### Юнит-тесты

```bash
# Все пакеты
make test

# Отдельный пакет
go test ./pkg/processor/k8s/... -v

# Отдельный тест
go test ./pkg/generator/... -run TestInjectMyFeature -v
```

### С покрытием

```bash
make coverage
# Открывает отчёт о покрытии; минимальный порог проекта — 70%, текущий уровень — 86%+
```

### Интеграционные тесты

```bash
go test ./tests/integration/... -v -timeout 60s
```

Интеграционные тесты используют реальные YAML-fixtures в `tests/integration/testdata/`. Они запускают полный пайплайн и проверяют структуру сгенерированного chart. Helm должен быть установлен.

### End-to-end тесты

```bash
go test ./tests/e2e/... -v -timeout 120s
```

E2E тесты генерируют chart и затем запускают `helm lint` и `helm template`. Helm должен быть в `$PATH`.

### Бенчмарки

```bash
go test ./pkg/... -bench=. -benchtime=5s
```

### Lint

```bash
make lint
# Эквивалентно: golangci-lint run ./...
```

Конфигурация линтера находится в `.golangci.yml`. CI-пайплайн запускает lint с `continue-on-error: true` — ошибки lint не блокируют слияние, но должны устраняться.

---

## 7. Обзор CI/CD пайплайна

Весь CI работает на GitHub Actions. Файлы workflow находятся в `.github/workflows/`.

| Workflow | Файл | Триггер | Этапы |
|----------|------|---------|-------|
| Test | `test.yml` | push, PR | Unit (матрица Go 1.25+1.26), Integration, E2E, Lint, Security, Coverage |
| Release | `release.yml` | push тега (`v*`) | GoReleaser: сборка, Docker, Homebrew |
| CodeQL | `codeql.yml` | push, расписание | Статический анализ |
| Auto-approve | `auto-approve.yml` | PR | Авто-approve PR от владельца через GitHub App bot |

### Правила защиты веток

PR в `main` должны пройти:

- `Unit Tests (Go 1.26)`
- `Lint Code`
- `Security Scan`
- `Build Binary`

### Порог покрытия

Шаг объединения покрытия в `test.yml` объединяет все профили покрытия и применяет **минимальный порог 70%**. Текущее покрытие по всему проекту составляет 86%+. Новый код должен поддерживать этот уровень.

---

## 8. Процесс релиза

Релизы полностью автоматизированы через GoReleaser при push тега версии.

### Создание релиза

```bash
# Убедитесь, что main чист и тесты проходят
git checkout main
git pull origin main
make test

# Тегируйте релиз
git tag -a v0.8.0 -m "Release v0.8.0"
git push origin v0.8.0
```

GoReleaser затем:
1. Собирает бинарные файлы для Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64).
2. Создаёт GitHub Release с checksums и changelog.
3. Публикует multi-arch Docker образ в `ghcr.io/alexgromer/dhg:v0.8.0` и `:latest`.
4. Обновляет формулу Homebrew tap в `AlexGromer/homebrew-tap`.

### Проверка релиза

```bash
# Проверить страницу GitHub Release
gh release view v0.8.0

# Получить и протестировать Docker образ
docker pull ghcr.io/alexgromer/dhg:v0.8.0
docker run --rm ghcr.io/alexgromer/dhg:v0.8.0 version
```

### Конфигурация Goreleaser

Полная конфигурация сборки находится в `.goreleaser.yml`. Ключевые секции: `builds` (флаги Go build, CGO отключён), `archives` (tar.gz + zip), `dockers` (multi-arch manifest), `brews` (формула Homebrew).

---

## Справочник: интерфейс Processor

```go
// pkg/processor/processor.go

type Processor interface {
    // Name returns a unique identifier for this processor.
    Name() string

    // Priority controls ordering when multiple processors match the same GVK.
    // Lower number = higher priority.
    Priority() int

    // Supports returns true if this processor can handle the given GVK.
    Supports(gvk schema.GroupVersionKind) bool

    // Process transforms an unstructured resource into a Result.
    Process(ctx Context, obj *unstructured.Unstructured) (*Result, error)
}

type Context struct {
    Ctx                 context.Context
    ChartName           string
    OutputMode          types.OutputMode
    Namespace           string
    AllResources        map[types.ResourceKey]*types.ExtractedResource
    ExternalFileManager *value.ExternalFileManager
    ValueProcessor      *value.Processor
}

type Result struct {
    Processed       bool
    ServiceName     string
    TemplatePath    string
    TemplateContent string
    ValuesPath      string
    Values          map[string]interface{}
    Dependencies    []types.ResourceKey
    Metadata        map[string]interface{}
}
```
