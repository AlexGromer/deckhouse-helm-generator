# EPIC-01 / 01: канал заметок для features (`FeatureContext.Note`)

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S |
| Зависит от | — |
| Требования | R16, R21 (основа для `Note:` всех требований EPIC-01; используется EPIC-04, EPIC-10) |

## Контекст

ADR-059 требует перечислять невыводимое (`SYNTHESIS.md` или `Note:` в stderr). Сейчас `Note:` печатают только конвейер (`cmd/dhg/pipeline.go`: `processor.ResolveCollisions`) и синтетические источники (`cmd/dhg/main.go`: `pipeline.synthesis.Notes()`, `writeSynthesisReport`). У features (`pkg/generator/features.go`: `Feature.Apply(chart, fc)`) канала нет: `FeatureContext` содержит только `Graph` и `Params`. Feature `gateway-api` (и `cilium-network-policy`, `pod-dns`) должна сообщать о неперенесённых аннотациях и пустых параметрах.

Если к моменту реализации такой канал уже добавлен другой задачей — задача закрывается ссылкой на него, а EPIC-01 использует существующий API (при другом имени — исправить ссылки в задачах 05–09 этого эпика).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features.go` | Тип `noteSink` (`seen map[string]bool`, `lines []string`, метод `add(string)`); поле `notes *noteSink` в `FeatureContext`; метод `func (fc FeatureContext) Note(format string, args ...interface{})` (nil-safe: без sink — ничего); `ApplyFeatures` создаёт один `noteSink` на вызов, передаёт его в каждый `FeatureContext` и возвращает `([]*types.GeneratedChart, []string, error)` |
| `cmd/dhg/main.go` | Вызов `generator.ApplyFeatures`: принять заметки; печатать `fmt.Fprintf(os.Stderr, "Note: %s\n", n)` (и в ветке `--dry-run`); если `pipeline.synthesis != nil` — добавить заметки к списку, передаваемому в `writeSynthesisReport` (параметр или поле `pipelineResult`) |
| `pkg/generator/features_test.go` | Обновить вызовы `ApplyFeatures` (третье возвращаемое значение); новые тесты (ниже) |
| `pkg/generator/features_all_test.go`, `features_*_test.go` | Обновить вызовы `ApplyFeatures`, если есть |
| `docs/DEVELOPER.md` | §5: правило «невыводимое — через `fc.Note`» и пример |

## Шаги

1. Ввести `noteSink`; дедупликация по точной строке, порядок — первого добавления.
2. `FeatureContext.Note`: `fc.notes.add(fmt.Sprintf(format, args...))` при `fc.notes != nil`.
3. Изменить `ApplyFeatures`; обновить все вызовы (`grep -rn "ApplyFeatures(" cmd pkg tests`).
4. Печать в `main.go`; формат как у существующих заметок.
5. Обновить `docs/DEVELOPER.md`.

## Тесты

- Unit `pkg/generator/features_test.go`:
  - `TestFeatureNotesDeduplicated`: тестовая feature `test-notes` (регистрируется существующим helper'ом `withTestFeature` из `features_test.go`) вызывает `fc.Note("x %d", 1)` на каждом из двух charts → `ApplyFeatures` возвращает `["x 1"]`.
  - `TestFeatureNotesOrder`: заметки `b`, `a`, `b` → `["b", "a"]`.
  - `TestFeatureContextNoteNilSink`: `FeatureContext{}.Note("x")` не паникует.
- Golden: не требуется (поведение chart'а не меняется). Существующий `TestFeaturesPassHelm` должен проходить.

## Критерии приёмки

- [ ] `ApplyFeatures` возвращает заметки; `dhg generate --with <feature>` печатает их как `Note: …` в stderr, по одной строке на уникальную заметку.
- [ ] При синтетическом источнике заметки features попадают в `SYNTHESIS.md` (раздел `## Notes`).
- [ ] Ни одна существующая feature не меняет поведение.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Заметки конкретных features — в задачах 05–09 этого эпика, EPIC-04, EPIC-10.
