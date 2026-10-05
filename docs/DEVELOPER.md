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
│   ├── main.go          # Cobra: generate, analyze, migrate, diff, fix, version, features
│   ├── pipeline.go      # Общий конвейер extract → dedupe → process → analyze
│   ├── validate.go      # dhg validate (разбор шаблонов, матрица версий K8s)
│   ├── graph.go         # dhg graph (DOT/Mermaid)
│   └── config.go        # .dhg.yaml: ключи = имена флагов generate
├── pkg/
│   ├── extractor/       # file, cluster (REST по kubeconfig), gitops (git clone), Deduplicate
│   ├── processor/       # Processor, Registry (GVK, приоритеты, generic fallback), plugin.go
│   │   └── k8s/         # Процессоры по видам ресурсов
│   ├── analyzer/        # Детекторы связей, граф, группы; graph.go — DOT/Mermaid/циклы
│   │   └── pattern/     # Проверки паттернов и рекомендации (dhg analyze)
│   ├── generator/       # Режимы universal/separate/library/umbrella, пост-обработка,
│   │                    # реестр features (features.go, features_*.go), CRD → crds/
│   ├── helm/            # Chart.yaml, values, helpers, README, схема values
│   └── types/           # ExtractedResource, ProcessedResource, ResourceGraph, GeneratedChart
├── tests/
│   ├── golden/          # Настоящий dhg + настоящий Helm на всех примерах и фикстурах
│   ├── integration/     # Конвейер на фикстурах
│   └── e2e/             # Рендеринг фикстурных chart'ов через Helm
└── examples/            # Примеры входных манифестов (входы golden-набора)
```

### Этапы конвейера (`cmd/dhg/pipeline.go`)

```
[1] Extract   → extractor.Extract()  → []ExtractedResource → extractor.Deduplicate()
[2] Process   → processor.Registry   → []ProcessedResource   (плагины --plugin первыми)
[3] Analyze   → analyzer.Analyze()   → ResourceGraph (связи, группы)
[4] Generate  → generator.Generate() → []GeneratedChart       (generate)
[4b..]        → пост-обработка флагов generate, затем features (--with), затем --template-dir
[5] Write     → generator.WriteChart()
```

Инвариант процессоров, на который опираются шаблоны: `Result.ServiceName` — допустимый идентификатор Go-шаблона (registry санитизирует его), значения ресурса лежат ровно по `Result.ValuesPath`, который читает шаблон.

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

## 5. Добавление опциональной возможности (`--with`)

Новые возможности не добавляют флагов в `main.go`: они регистрируются в реестре `pkg/generator/features.go` и сразу доступны как `dhg generate --with <имя>`, видны в `dhg features` и автоматически попадают в golden-набор (каждая возможность — на 7 входах, все вместе — во всех режимах на всех входах).

```go
// pkg/generator/features_myteam.go
func init() {
	RegisterFeature(Feature{
		Name:        "my-feature",
		Description: "One line shown by dhg features",
		Params:      map[string]string{"label": "default"}, // все параметры с умолчаниями
		Apply: func(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
			out := cloneChart(chart) // copy-on-write: входной chart не меняется
			name := chartNameOf(chart)
			out.Templates["templates/my-feature.yaml"] = fmt.Sprintf(`{{- if .Values.myFeature.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "%s.fullname" . }}-my-feature
data:
  label: {{ .Values.myFeature.label | quote }}
{{- end }}
`, name)
			values, err := appendTopLevelValues(out.ValuesYAML, "myFeature",
				map[string]interface{}{"enabled": true, "label": fc.Param("label")})
			if err != nil {
				return nil, err
			}
			out.ValuesYAML = values
			return out, nil
		},
	})
}
```

Правила:
- каждое добавление выключается через values (`<feature>.enabled`);
- уникальные пути шаблонов и ключи values, чтобы возможности комбинировались;
- в separate/umbrella values плоские, а helpers называются по chart'у (`chartNameOf`);
- `fc.Graph` даёт доступ к входным ресурсам.

`dhg generate -f examples/05-full-stack --chart-name app --with my-feature --feature-opt my-feature.label=x` — проверить вручную.

---

## 6. Запуск тестов

```bash
make test        # все тесты; tests/golden пропускается без helm
make golden      # DHG_REQUIRE_HELM=1 go test ./tests/golden/
make lint        # golangci-lint v2 (.golangci.yml)
go test ./pkg/generator -run TestApplyFeatures -v
```

**Golden-набор** (`tests/golden`) — главный критерий корректности. Он собирает `dhg` и для каждого входа (`examples/*`, `tests/integration/fixtures/*`) × режима × флагов пост-обработки × features проверяет:
- `helm lint --strict`, `helm template --include-crds`, все `values-*.yaml`;
- число объектов не меньше входного; селекторы workload'ов совпадают с метками pod'ов;
- ссылки (ConfigMap/Secret/PVC/SA, backend'ы Ingress, roleRef, …) и селекторы Service/PDB/NetworkPolicy продолжают разрешаться (`integrity_test.go`);
- `helm unittest` (если установлен плагин), post-renderer (если есть kustomize/kubectl);
- источники `cluster` (fake API-сервер) и `gitops` (локальный репозиторий).

Новый вход — каталог в `tests/integration/fixtures/` или `examples/`: golden подхватит его автоматически.

---

## 7. CI (`.github/workflows/test.yml`)

| Job | Что делает |
|---|---|
| Unit Tests | `go vet`, `go test -race ./cmd/... ./pkg/...` |
| Integration / E2E | `tests/integration`, `tests/e2e` с Helm 3.19 |
| Golden | `tests/golden` с Helm 3.19, helm-unittest, `DHG_REQUIRE_HELM=1` |
| Lint Code | golangci-lint v2 и gofmt — ошибки блокируют |
| Security Scan, Coverage, Build, Benchmarks | как раньше |

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
	// Process returns a Result; Processed=false passes the resource on to the
	// next processor (or the generic fallback).
	Process(ctx Context, obj *unstructured.Unstructured) (*Result, error)
	// Supports lists the exact GVKs (including version) the processor handles.
	Supports() []schema.GroupVersionKind
	// Priority: higher runs first; plugins use PluginPriority (1000).
	Priority() int
	Name() string
}
```

`Result`: `Processed`, `ServiceName`, `TemplatePath`, `TemplateContent`, `ValuesPath`, `Values`, `Dependencies`, `ExternalFiles`, `Metadata`.
