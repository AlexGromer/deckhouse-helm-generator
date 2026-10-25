# EPIC-06: дизайн

## 1. Обзор архитектуры

Конвейер до шага записи не меняется: extract → process → analyze → generate → пост-обработка (флаги generate, `--with`, `--template-dir`). Меняется шаг [5] Write.

```
[4..] charts []*GeneratedChart + дополнительные файлы (env values, monorepo, kustomize, SYNTHESIS.md)
   │
   ▼  задача 01
generator.OutputSet (theirs: путь → байты, режим)            ← всё, что пишет генерация, в одном месте
   │
   ├─ без --update ──► regen.Guard: есть .dhg/state.json и правки? → ошибка (R5) │ нет → WriteOutputSet + удаление устаревших (R6)
   │
   └─ --update ─────► regen.LoadState (.dhg/<name>/state.json + base/*.base, sha256)   задача 05
                        │
                        ▼  задача 06
                      regen.BuildPlan(base, ours(диск), theirs)
                        │   ├─ классификация файлов (spec §4.1)
                        │   ├─ merge.Merge3   (текст, задача 03)  ── merge.Diff (Myers, задача 02)
                        │   └─ merge.MergeYAML (YAML, задача 04)  ── merge.Merge3 (гибрид)
                        ▼
                      Plan (в памяти) → bump (задача 07) → Apply (файлы) → SaveState (последним) → MERGE.md, exit 0/3
```

Пакеты:

| Пакет | Назначение | Зависит от |
|---|---|---|
| `pkg/merge` (новый) | Myers diff, unified diff, diff3, структурное слияние YAML. Чистые функции над байтами, без файловой системы | stdlib, `go.yaml.in/yaml/v3` (уже в go.mod как indirect) |
| `pkg/regen` (новый) | Состояние `.dhg/`, классификация, план, применение, отчёт `MERGE.md`, bump | `pkg/merge`, `pkg/generator` (тип `OutputSet`) |
| `pkg/generator` | `OutputSet`, `ChartFiles`, `WriteOutputSet` (вынос из `WriteChart`) | — |
| `cmd/dhg` | Флаги, сбор `OutputSet` в `runGenerate`, запись сохранённых флагов, exit code | все выше |

## 2. Компоненты и изменения

### 2.1. `pkg/generator/output.go` (задача 01)

```go
// OutputFile is one file written by a generation; Path is relative to the
// output directory and uses forward slashes.
type OutputFile struct {
    Path    string
    Content []byte
    Mode    fs.FileMode // 0644, or 0755 for *.sh
}

// OutputSet is everything one generation writes below the output directory.
type OutputSet struct{ files map[string]OutputFile }

func NewOutputSet() *OutputSet
// Add adds a file; a second file with the same path and different content is an error.
func (s *OutputSet) Add(f OutputFile) error
func (s *OutputSet) Get(path string) (OutputFile, bool)
func (s *OutputSet) Paths() []string // sorted
func (s *OutputSet) Len() int

// ChartFiles returns the files WriteChart writes for chart, with paths
// prefixed by chart.Name (e.g. "app/templates/x.yaml", "app/charts/web/Chart.yaml").
func ChartFiles(chart *types.GeneratedChart) ([]OutputFile, error)
// WriteOutputSet writes every file below outputDir (directories 0755).
func WriteOutputSet(s *OutputSet, outputDir string) error
```

`WriteChart` остаётся обёрткой (`ChartFiles` + запись): его вызывают тесты. Проверка «внешний файл вне каталога chart'а» (`generator.go`, ветка `ExternalFiles`) переезжает в `ChartFiles`.

### 2.2. `cmd/dhg/main.go`, `runGenerate`

Код после `// Step 5: Write charts to disk` (строка 715) и до конца функции переписывается. Charts, файлы env values (строки 735–783), monorepo (785–812), kustomize (814–842) и `SYNTHESIS.md` (`writeSynthesisReport`, 1675) добавляются в один `OutputSet`. Затем вызывается `writeOutput(opts, set)` из нового `cmd/dhg/regen.go`. Функции-сборщики:

| Новая функция | Источник логики |
|---|---|
| `envValueFiles(charts, groupingResult) ([]generator.OutputFile, error)` | блок `if opts.envValues` |
| `monorepoFiles(layout) []generator.OutputFile` | блок `if opts.monorepo` |
| `kustomizeFiles(chartName, out *generator.KustomizeOutput) []generator.OutputFile` | блок `if opts.kustomize` + `writeKustomizeDir` |
| `synthesisFile(p *pipelineResult) generator.OutputFile` | `writeSynthesisReport` (печать `Note:` остаётся в нём) |

### 2.3. `pkg/merge` (задачи 02–04)

```go
// SplitLines splits b into lines, each keeping its "\n" ("\r\n" stays inside the line).
func SplitLines(b []byte) []string

type EditKind int // Equal, Delete, Insert
type Edit struct {
    Kind EditKind
    A, B int // index in a (Equal, Delete) and in b (Equal, Insert); -1 when not applicable
}
// Diff returns a shortest edit script turning a into b (Myers 1986).
func Diff(a, b []string) []Edit
// Unified renders a unified diff with ctx lines of context; "" when a == b.
func Unified(aName, bName string, a, b []string, ctx int) string

type Labels struct{ Ours, Base, Theirs string }
type TextResult struct {
    Content   []byte
    Conflicts int // number of conflict hunks written with markers
}
// Merge3 merges ours and theirs against base line by line (diff3).
func Merge3(base, ours, theirs []byte, l Labels) TextResult

type YAMLConflict struct {
    Path               string // "services.webApp.deployment.containers[name=web].image.tag"
    Base, Ours, Theirs string // one-line JSON, "" when absent
    Kind               string // "value", "delete-modify", "modify-delete", "add-add"
}
type YAMLResult struct {
    Content     []byte
    Conflicts   []YAMLConflict
    Reformatted bool // written from the parsed tree, not from the text merge
}
// MergeYAML merges three YAML documents structurally (falls back to Merge3, see §4.3).
func MergeYAML(base, ours, theirs []byte, l Labels) (YAMLResult, error)

// HasConflictMarkers reports dhg's own markers / dhg-conflict comments (R22).
func HasConflictMarkers(content []byte) int
```

### 2.4. `pkg/regen` (задачи 05–07)

```go
const Dir = ".dhg"

type FileState struct {
    Path   string      `json:"path"`
    SHA256 string      `json:"sha256"`
    Mode   fs.FileMode `json:"mode"`
}
type State struct {
    FormatVersion int                    `json:"formatVersion"` // 1
    DHGVersion    string                 `json:"dhgVersion"`
    ChartName     string                 `json:"chartName"`
    Mode          string                 `json:"mode"`
    Options       map[string]interface{} `json:"options"` // flag name → string | []string | bool
    Files         []FileState            `json:"files"`   // sorted by Path
}

// Load reads <out>/.dhg/<name>/state.json and the base files, verifying sha256 (R4).
func Load(outputDir, chartName string) (*State, *generator.OutputSet, error)
// Discover returns the chart names that have state below <out>/.dhg (R9).
func Discover(outputDir string) ([]string, error)
// Save writes base/<path>.base and state.json (state.json last, both via temp file + rename).
func Save(outputDir string, st *State, theirs *generator.OutputSet) error
// Edited lists tracked paths whose content on disk differs from base (R5).
func Edited(outputDir string, base *generator.OutputSet) ([]string, error)

type Action string // "added", "updated", "merged", "conflict", "removed", "kept", "kept-deleted", "unchanged"
type Item struct {
    Path      string
    Action    Action
    Content   []byte      // nil for removed / kept / kept-deleted / unchanged
    Mode      fs.FileMode
    Hunks     int         // text conflict hunks
    YAML      []merge.YAMLConflict
    Reason    string      // modify/delete, add/add, binary, unresolved, invalid-yaml
}
type Plan struct {
    ChartName, Mode, OldVersion, NewVersion string
    Items       []Item // sorted by Path
    Reformatted []string
}
type PlanOptions struct{ Labels merge.Labels }

func BuildPlan(outputDir string, base, theirs *generator.OutputSet, o PlanOptions) (*Plan, error)
func (p *Plan) Bump(policy string, theirs *generator.OutputSet) error // задача 07
func (p *Plan) Apply(outputDir string) error                           // files, deletions, empty dirs
func (p *Plan) Conflicts() int
func (p *Plan) Summary() string     // stdout, spec §5.3
func (p *Plan) MergeReport() string // MERGE.md, spec §4.4
```

### 2.5. `cmd/dhg`

- `cmd/dhg/regen.go` (новый): `writeOutput(opts generateOptions, set *generator.OutputSet) error`. Ветки: `--no-state`, без `--update` (Guard по R5–R8), `--update` (план → bump → apply → save → отчёт).
- `cmd/dhg/main.go`, `newGenerateCmd`: флаги `--update`, `--force`, `--no-state`, `--bump`, `--base-dir`. `RunE` собирает `recordedOptions` обходом `cmd.Flags().VisitAll`: флаги, у которых `Value.String() != DefValue`, без исключений R16. Для slice-флагов значение берётся через `pflag.SliceValue.GetSlice()`.
- `cmd/dhg/main.go`, `PreRunE`: при `--update` после `applyConfigFile` вызывает `applyRecordedOptions(cmd, outputDir, chartName)`. Флаг `--chart-name` обязателен, только если `regen.Discover` нашёл не ровно один каталог.
- `cmd/dhg/config.go`, `applyConfigFile`: устанавливать флаги так, чтобы `flag.Changed == true` (скаляр — `cmd.Flags().Set(name, v)`, список — `SliceValue.Replace` и `flag.Changed = true`). Тогда сохранённые флаги не перекрывают конфиг (R17). Сейчас `Changed` проверяется только в самой `applyConfigFile` (`config.go:55`), так что для существующих команд поведение не меняется.
- `cmd/dhg/main.go`, `main()`: `var ec exitCodeError; if errors.As(err, &ec) { os.Exit(ec.code) }`, иначе `os.Exit(1)`. Тип `exitCodeError{code int; msg string}` — в `regen.go`.
- `dhg diff` (`runDiff`, `printUnifiedDiff`): вывод через `merge.Unified` (с цветом, как сейчас), флаг `--exit-code`, пропуск `.dhg/` в `collectFiles`.

## 3. Модель данных

### 3.1. Каталог `.dhg/` — общий контракт (владелец EPIC-06)

```
<output>/
├── .dhg/
│   └── <chart-name>/                 ← идентичность генерации (spec R9)
│       ├── state.json                ← State, JSON с отступом 2, ключи как в §2.4, оканчивается "\n"
│       ├── base/
│       │   └── <путь в наборе>.base  ← байты прошлой генерации
│       └── digests.yaml              ← ЗАРЕЗЕРВИРОВАНО для EPIC-07 (lock digest'ов образов); EPIC-06 его не читает и не удаляет
├── MERGE.md                          ← только при конфликтах (spec R21)
└── <chart'ы и прочие файлы набора>
```

Правила для других эпиков: файлы внутри `.dhg/<chart-name>/` вне `state.json` и `base/` могут создавать только эпики, перечисленные здесь (сейчас — EPIC-07, `digests.yaml`). Эти файлы не входят в набор выходных файлов и не сливаются.

### 3.2. Пример `state.json`

```json
{
  "formatVersion": 1,
  "dhgVersion": "0.10.0",
  "chartName": "app",
  "mode": "universal",
  "options": {
    "chart-name": "app",
    "file": ["../manifests"],
    "with": ["policies"]
  },
  "files": [
    {"path": "app/.helmignore", "sha256": "…", "mode": 420},
    {"path": "app/Chart.yaml", "sha256": "…", "mode": 420}
  ]
}
```

`mode` — десятичное `fs.FileMode` (420 = 0644, 493 = 0755).

## 4. Алгоритмы

### 4.1. Myers diff (задача 02)

Жадный алгоритм прямого прохода из статьи Myers (1986, §2–3) с сохранением массива `V` на каждом шаге `D` (trace) и обратным проходом для восстановления скрипта. Память — O(D²) в худшем случае. Для файлов chart'а (сотни строк) это допустимо. Линейная по памяти версия (§4b статьи) не нужна. Строки сравниваются как строки Go (байты). Выбор хода на диагонали `k` — классическое правило статьи: если `k == -D` или (`k != D` и `V[k-1] < V[k+1]`), то `x = V[k+1]` (ход вниз — вставка), иначе `x = V[k-1] + 1` (ход вправо — удаление). Скрипт детерминирован. При выводе подряд идущие удаления и вставки одной замены упорядочиваются так: сначала все удаления, потом все вставки.

`Unified`: группировка правок в hunk'и с `ctx` строками контекста; две группы сливаются, если между ними ≤ 2·ctx равных строк. Заголовок `@@ -l,s +l,s @@` по правилам GNU diff (`s` опускается, если равно 1; для пустой стороны `l` — номер строки до неё, `s` = 0). Отсутствие `\n` в конце выводится строкой `\ No newline at end of file`.

### 4.2. diff3 (задача 03)

По формализации Khanna–Kuber–Pierce (2007):

1. `MA` = сопоставление строк base ↔ ours, `MB` = base ↔ theirs, из `Diff(base, ours)` и `Diff(base, theirs)` (пары `Equal`).
2. Позиции `lo, la, lb` (последние выведенные строки base/ours/theirs), начально −1.
3. Ищется наименьшее `i ≥ 1` такое, что **не** выполняется «`base[lo+i]` сопоставлена с `ours[la+i]` в `MA` и с `theirs[lb+i]` в `MB`».
   - Если такого `i` нет (до конца всё стабильно) — вывести остаток как стабильный блок, конец.
   - Если `i > 1` — вывести стабильный блок длиной `i−1` (строки base), сдвинуть все три позиции на `i−1`.
   - Если `i == 1` — найти наименьшее `j > lo`, где `base[j]` сопоставлена и в `MA` (с `a > la`), и в `MB` (с `b > lb`). Если нет — нестабильный блок до конца всех трёх. Иначе нестабильный блок `base[lo+1..j−1]`, `ours[la+1..a−1]`, `theirs[lb+1..b−1]`; позиции := `j−1, a−1, b−1`.
4. Нестабильный блок: ours-часть = base-части → взять theirs; theirs-часть = base-части → взять ours; ours-часть = theirs-части → взять ours (ложный конфликт); иначе — конфликт с маркерами (spec §4.2).

Тест-оракул — `git merge-file -p --diff3 -L ours -L base -L theirs`. На корпусе задачи 03 должны совпадать чистые результаты и признак «есть конфликт». Границы конфликтных участков у git (xdiff, «zealous» merge) могут отличаться, поэтому их не сравниваем.

### 4.3. Структурное слияние YAML (задача 04)

Разбор: `yaml.Unmarshal(b, &node)` пакета `go.yaml.in/yaml/v3` для base, ours, theirs. Отказ от структурного режима — сразу `Merge3` (текст) — при любом из условий: ошибка разбора; больше одного документа; якоря/алиасы (`&x`, `*x`, `<<:`) в любой версии; корень — не отображение.

Равенство значений `eq(x, y)`: `x.Decode(&vx)`, `y.Decode(&vy)`, `reflect.DeepEqual(vx, vy)`. Отсутствие ключа (⊥) отличается от `null`.

`merge(path, b, o, t)` для узлов (любой может быть ⊥):

| Условие | Результат |
|---|---|
| `eq(o, t)` | o |
| `eq(o, b)` | t (в том числе удаление, если t = ⊥) |
| `eq(t, b)` | o |
| o, t (и b, если есть) — отображения | рекурсия по ключам (ниже) |
| o, t (и b) — keyed-списки | рекурсия по `name` (ниже) |
| иначе | **конфликт**: результат o; запись `YAMLConflict{path, b, o, t, kind}` |

`kind`: `value` (все три есть); `add-add` (b = ⊥); `delete-modify` (o = ⊥: пользователь удалил, генератор изменил — результат ⊥); `modify-delete` (t = ⊥: генератор удалил, пользователь изменил — результат o).

Отображения: ключи результата — ключи o в порядке o, затем ключи t, которых нет ни в o, ни в b, в порядке t. Ключ, который есть в b и o, но не в t, проходит через `merge` (удалится, если `eq(o, b)`). Узлы значений берутся из стороны-источника вместе с комментариями. Комментарии ключей o сохраняются.

Keyed-списки: список keyed, если в каждой из имеющихся версий (b, o, t) каждый элемент — отображение со скалярным строковым `name`, и `name` уникальны внутри списка. Элементы сопоставляются по `name`; порядок — как в o, затем новые из t в порядке t; сегмент пути — `[name=<имя>]`. Иначе список атомарен (сравнивается целиком через `eq`).

Аннотация конфликта: к `HeadComment` узла-ключа (для элемента keyed-списка — к ключу `name` элемента) добавляется строка

```
dhg-conflict: generator changed this value from <b> to <t>; your value <o> is kept (see MERGE.md)
```

(`<x>` — JSON в одну строку, до 120 символов). Для `delete-modify` узла нет, конфликт пишется только в `MERGE.md`.

**Гибрид и сохранение форматирования.** Перекодирование `yaml.Node` теряет пустые строки и меняет отступ списков (эксперимент 2026-10-07, см. spec §9). Поэтому:

1. `T := Merge3(base, ours, theirs)` (текст).
2. `S := structural(base, ours, theirs)`.
3. Если `T.Conflicts == 0`, `len(S.Conflicts) == 0` и `eq(parse(T.Content), S.tree)` — результат `T.Content` (форматирование пользователя сохранено, `Reformatted = false`).
4. Иначе — кодирование `S.tree` (`yaml.NewEncoder`, `SetIndent(2)`), `Reformatted = true`, файл попадает в раздел «Переформатированные файлы» `MERGE.md`.

### 4.4. Планировщик (задача 06)

1. `paths := base.Paths() ∪ theirs.Paths()` (отсортированы).
2. Для каждого пути читается O с диска (`os.ReadFile`; отсутствие файла — ⊥; каталог на месте файла — ошибка). Классификация — по spec §4.1.
3. Выбор слияния: `isStructuralYAML(path)` — имя `values.yaml`, `values-*.yaml`, `Chart.yaml` или расширение `.yaml`/`.yml`, если ни один сегмент пути не равен `templates` или `crds`. Иначе — текст. Бинарный (есть `0x00` в любой версии) — конфликт `binary` без слияния.
4. `merge.HasConflictMarkers(O)` > 0 → дополнительно `Reason = unresolved` (R22).
5. Метки: `Ours = "ours: <path> (current)"`, `Base = "base: dhg <state.DHGVersion>"`, `Theirs = "theirs: dhg <version>"`.
6. `Apply`: запись каждого `Content` через временный файл в том же каталоге (`<имя>.dhg-tmp`) и `os.Rename`. Перезаписываются только файлы, где `Content` отличается от O (R23). Затем удаления (`removed`), затем удаление пустых каталогов снизу вверх. Удаляются только каталоги, которые были в base или theirs и стали пустыми.
7. `regen.Save` — после `Apply`. Base := theirs.
8. При конфликтах — `MERGE.md` и `exitCodeError{3}`; при чистом слиянии — удаление собственного `MERGE.md`.

### 4.5. Повышение версии (задача 07)

Каталог chart'а — каталог, где в theirs есть `Chart.yaml`. Файл принадлежит самому глубокому такому каталогу-префиксу: `app/charts/web/templates/x.yaml` относится к `app/charts/web`. Chart «изменён», если хотя бы у одного его файла итог отличается от O, или изменён любой вложенный chart (только для umbrella-родителя). Для каждого изменённого chart'а: `v := semver(O.version)` (из итогового `Chart.yaml` плана), `v' := bump(v, policy)`, затем запись `version: v'` правкой узла yaml.Node итогового `Chart.yaml`. Для каждого повышенного подчарта `P/charts/N` в `P/Chart.yaml` правится `dependencies[name=N].version`. Pre-release и build metadata при bump отбрасываются (`1.2.3-rc.1` → patch → `1.2.3`, как `semver inc` из npm). Невалидная версия — ошибка до записи.

## 5. Альтернативы

| Вариант | Плюсы | Минусы | Решение |
|---|---|---|---|
| **Base: полный снимок в `.dhg/<name>/base/`** | Не зависит от git; работает в CI с shallow clone; детерминирован | Дублирует размер chart'а (килобайты); пользователь должен коммитить `.dhg/` | **Принято** |
| Base: только sha256 файлов (lock-файл) | Маленький | Определяет «изменён ли файл», но не даёт слить: при правке с обеих сторон остаётся только «оставить одно из двух» | Отклонено (sha256 оставлены для проверки целостности, R4) |
| Base: из истории git (коммит прошлой генерации) | Нет дополнительных файлов | Нужен git CLI и полная история (у Argo CD/CI часто shallow clone); правки часто коммитят вместе с генерацией, и чистой base в истории нет; требует метки коммита | Отклонено как основной; как разовый bootstrap — `--base-dir` с ручной выгрузкой ревизии (задача 08) |
| Base: заново сгенерировать из старого входа | Нет снимка | Старый вход недоступен (кластер изменился), другая версия dhg даёт другой результат | Отклонено |
| diff3: `github.com/epiclabs-io/diff3` (MIT) | Готов; Myers и Hunt–McIlroy | Только pseudo-version без тегов; в 2026 сделан generic с ломающим изменением API; внутри goroutine; ~790 строк чужого кода против ~400 своих; ADR-051/060 требуют минимума зависимостей | Отклонено |
| diff3: вызов `git merge-file` | Эталонная реализация, ноль кода | Новая обязательная runtime-зависимость для `generate` (git сейчас нужен только `--source gitops`); временные файлы; разное поведение версий git | Отклонено; git используется как **тест-оракул** (задача 03) |
| diff3: `github.com/sergi/go-diff` (v1.4.0, 2025-06-05) | Тегированные релизы | Diff-match-patch (символьный), не diff3 | Отклонено |
| Своя реализация Myers + diff3 (`pkg/merge`) | Без зависимостей; детерминирована; Myers нужен и для `dhg diff` | ~400 строк + тесты | **Принято** |
| YAML: `sigs.k8s.io/yaml` (map) | Уже используется | Теряет комментарии и порядок ключей | Отклонено |
| YAML: `go.yaml.in/yaml/v3` `Node` | Комментарии и стиль сохраняются; модуль уже в go.mod (indirect → direct, новых модулей нет) | Нормализует пустые строки и отступы списков | **Принято** с гибридом §4.3 |
| CLI: `dhg regenerate` | Отдельная команда, явная | Дублирование ~50 флагов `generate` или рефакторинг их регистрации | См. README Q1; рекомендовано `generate --update` |

## 6. ADR-кандидаты

- **ADR-кандидат: повторная генерация — трёхстороннее слияние со снимком последней генерации в `<output>/.dhg/<chart-name>/`.** Контекст: повторный `generate` теряет правки и оставляет устаревшие файлы (README эпика). Решение: base — байты последней генерации (файлы `*.base`, sha256 в `state.json`); ours — диск; theirs — новая генерация; base обновляется до theirs после каждого слияния. Последствия: пользователь коммитит `.dhg/`; `generate` поверх изменённого chart'а без `--update`/`--force` — ошибка (изменение поведения).
- **ADR-кандидат: Myers diff и diff3 реализованы в `pkg/merge` без внешних зависимостей.** Контекст: ADR-051/ADR-060. Решение: своя реализация по Myers (1986) и Khanna–Kuber–Pierce (2007), `git merge-file` — только тест-оракул. Последствия: тот же движок исправляет `dhg diff`.
- **ADR-кандидат: values.yaml сливается структурно через `go.yaml.in/yaml/v3`, списки атомарны, кроме списков объектов с уникальным `name`; конфликт оставляет значение пользователя с комментарием `dhg-conflict`.** Последствия: YAML всегда валиден после слияния; форматирование сохраняется, если текстовое слияние даёт тот же результат, иначе нормализуется.

ADR-016 не меняется (другая задача — слияние входных источников).

## 7. Влияние на существующее поведение

| Что | Было | Стало |
|---|---|---|
| Каталог `--output` после `generate` | Только chart'ы и файлы | Плюс `.dhg/<chart-name>/` |
| Повторный `generate` поверх изменённого chart'а (с `.dhg/`) | Тихая перезапись | Ошибка; `--update` или `--force` |
| Устаревшие файлы | Оставались (chart мог перестать рендериться) | Удаляются, если не изменены |
| `dhg diff` | Позиционное сравнение, exit 0 | Myers, `--exit-code` |
| `applyConfigFile` | Не помечал флаги `Changed` | Помечает (нужно для R17) |
| Golden-набор | `findCharts` обходит весь `out` | Должен пропускать `.dhg` (в задаче 05). Копии `.base` не называются `Chart.yaml`, но явный пропуск защищает от будущих файлов |

Миграция: старые каталоги без `.dhg/` работают как раньше (R7). Первый `generate` пишет снимок. Чтобы слить правки, сделанные до этого, используется `--update --base-dir` (задача 08).

## 8. Тестирование

- Unit, `pkg/merge`: таблицы для `Diff`/`Unified` (пустые входы, без `\n` в конце, CRLF, одна вставка в начале); diff3-корпус `pkg/merge/testdata/diff3/<case>/{base,ours,theirs,want}` с оракулом git (пропуск, если git не найден); YAML-корпус `pkg/merge/testdata/yaml/<case>/{base,ours,theirs}.yaml` + ожидаемые конфликты в `want.json`.
- Unit, `pkg/regen`: классификация (все 13 строк spec §4.1) на `t.TempDir()`; `Save`/`Load` с порчей sha256; `Apply` с проверкой mtime; bump.
- Unit, `cmd/dhg`: `writeOutput` во всех ветках; `applyRecordedOptions` (приоритеты R17, относительные пути R16); `exitCodeError`.
- Golden (`tests/golden/regen_test.go`, задачи 06 и 09): настоящий dhg + Helm, все режимы, сценарии spec AC4–AC7.
- Integration: не требуется — путь покрыт golden.

## 9. Запросы к другим эпикам

- **EPIC-07:** использует `.dhg/<chart-name>/digests.yaml` (резерв в §3.1). Lock digest'ов не входит в набор выходных файлов и не сливается. При `--update` EPIC-07 читает его как обычно, флаг `pin-digests` восстанавливается из `options`.
- **EPIC-11 (профили):** если профиль задаёт набор флагов, его имя должно попадать в `options` как обычный флаг, чтобы `--update` воспроизводил генерацию.
- **Все эпики, добавляющие флаги `generate`:** новый флаг попадает в `options` автоматически (R16). Флаг, который не должен сохраняться (одноразовое действие), нужно добавить в список исключений `recordExclusions` в `cmd/dhg/regen.go`.

## 10. Масштабирование и развитие

- Слияние JSON (`values.schema.json`) структурно — тем же алгоритмом §4.3 через `encoding/json` (сейчас — текст).
- `dhg generate --update --strategy ours|theirs` для автоматического разрешения конфликтов в CI (по аналогии с `git merge-file --ours/--theirs`).
- Отчёт `MERGE.md` в машинном формате (`--merge-report json`) для ботов в MR.
- Автоматический PR/MR из CI: `dhg generate --update` по расписанию + `--bump patch` (вместе с EPIC-07 `--refresh-digests`).
