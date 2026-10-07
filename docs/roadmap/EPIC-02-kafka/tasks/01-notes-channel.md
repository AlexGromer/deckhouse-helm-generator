# EPIC-02 / 01: Канал заметок для процессоров и features

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S (≤ 1 дня) |
| Зависит от | — |
| Требования | R1 |

## Контекст

ADR-059 требует перечислять невыводимое, но сейчас вывести заметку могут только:
- синтетические источники: `extractor.Reporter.Notes()`, печать в `cmd/dhg/main.go` и `writeSynthesisReport`;
- `processor.ResolveCollisions`: печать в `cmd/dhg/pipeline.go:runPipeline`.

У `processor.Result` (`pkg/processor/processor.go`) и `types.ProcessedResource` (`pkg/types/resource.go`) нет поля для заметок. `Feature.Apply` (`pkg/generator/features.go`) возвращает только `(*types.GeneratedChart, error)`. Задачи 03, 06, 08 этого эпика и задачи EPIC-09 выводят заметки, поэтому механизм нужен первым.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/processor.go` | В `Result` добавить `Notes []string` с комментарием «human-readable notes about this object (printed as "Note: ...")» |
| `pkg/types/resource.go` | В `ProcessedResource` добавить `Notes []string` |
| `cmd/dhg/pipeline.go` | В `runPipeline` копировать `result.Notes` в `ProcessedResource.Notes`. После печати заметок `ResolveCollisions` печатать заметки ресурсов в порядке `processed`, без повторов (`map[string]bool`), формат `Note: %s`. В `pipelineResult` добавить `notes []string` со всеми напечатанными заметками процессоров (без заметок коллизий — их поведение не меняется) |
| `pkg/generator/features.go` | В `FeatureContext` добавить `AddNote func(format string, args ...interface{})` и неэкспортируемый метод `func (fc FeatureContext) note(format string, args ...interface{})`, который ничего не делает при `AddNote == nil`. Добавить `ApplyFeaturesWithNotes(charts []*types.GeneratedChart, names []string, options map[string]map[string]string, graph *types.ResourceGraph) ([]*types.GeneratedChart, []string, error)`: тело текущей `ApplyFeatures`, плюс `AddNote`, который добавляет `fmt.Sprintf(...)` в срез, пропуская уже добавленные строки. `ApplyFeatures` вызывает её и возвращает первые и третье значения |
| `cmd/dhg/main.go` | В `runGenerate` вызывать `ApplyFeaturesWithNotes`; заметки печатать `Note: %s` сразу после применения features. `writeSynthesisReport(outputDir string, p *pipelineResult, extra []string)`: `notes := append(p.synthesis.Notes(), extra...)` для отчёта. Печать в stderr внутри — только `p.synthesis.Notes()`, остальное уже напечатано. В вызов передать `append(pipeline.notes, featureNotes...)`. Ветка `--dry-run` печатает те же заметки |
| `docs/DEVELOPER.md` | Раздел 3.1: абзац «Заметки: `Result.Notes` для невыводимого (ADR-059)». Раздел 5: правило «невыводимое — через `fc.note(...)`» |

## Шаги

1. Добавить поля и функции из таблицы.
2. Обновить `runPipeline` и `runGenerate`.
3. Написать тесты.
4. Обновить `docs/DEVELOPER.md` и `docs/RELEASE_NEXT.md` (строка «Процессоры и `--with`-возможности сообщают невыводимое через `Note:`»).

## Тесты

- **Unit `pkg/generator/features_test.go`:**
  - `TestApplyFeaturesWithNotes`: тестовая feature `note-feature` (регистрируется в тесте через `RegisterFeature` с уникальным именем) вызывает `fc.note("same %s", "note")` на каждом chart'е. На двух chart'ах (`app`, `api`) результат — `[]string{"same note"}`;
  - `TestFeatureContextNoteNil`: `FeatureContext{}.note("x")` не паникует.
- **Unit `cmd/dhg/main_test.go`:** вход — `t.TempDir()` с одним `ConfigMap`. Процессор-плагин здесь не нужен: проверяется `writeSynthesisReport` с фиктивным `extractor.Reporter` (тип в тесте с `Notes() []string{"a"}` и `Inputs() []string{"in"}`) и `extra = []string{"b"}`. В `SYNTHESIS.md` есть строки `- a` и `- b`.
- **Golden:** изменений нет. Существующий набор остаётся зелёным.

## Критерии приёмки

- [ ] `processor.Result.Notes` и `types.ProcessedResource.Notes` существуют; заметки процессоров печатаются один раз.
- [ ] `ApplyFeaturesWithNotes` возвращает уникальные заметки; сигнатура `ApplyFeatures` не изменилась.
- [ ] При синтетическом источнике заметки features и процессоров попадают в `SYNTHESIS.md`.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Заметки конкретных процессоров и features (задачи 03, 06, 08; EPIC-09).
- Отдельный файл отчёта для несинтетических источников: заметки там идут только в stderr. Расширение `SYNTHESIS.md` на все источники — решение владельца, не этой задачи.
