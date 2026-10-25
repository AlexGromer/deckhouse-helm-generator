# EPIC-06 / 01: набор выходных файлов (OutputSet) и единая запись

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M |
| Зависит от | — |
| Требования | основа для R1, R6, R10 (spec.md); поведение `generate` не меняется |

## Контекст

Сейчас `runGenerate` (`cmd/dhg/main.go:329`) пишет файлы в пяти местах:

- `generator.WriteChart` (`pkg/generator/generator.go:93`) — Chart.yaml, values.yaml, templates, `_helpers.tpl`, NOTES.txt, `values.schema.json`, `.helmignore`, `ExternalFiles` (с проверкой «не за пределами каталога chart'а» и режимом `0755` для `*.sh`);
- блок `--env-values` (`main.go:735–783`) — `values-<env>.yaml` в каталоге chart'а;
- блок `--monorepo` (`main.go:785–812`) — `Makefile`, `.helmignore`, `ct.yaml` в `--output`;
- блок `--kustomize` (`main.go:814–842`, `writeKustomizeDir` на строке 1657) — `<chart>/kustomize/**`;
- `writeSynthesisReport` (`main.go:1675`) — `SYNTHESIS.md` в `--output`.

Для трёхстороннего слияния нужен полный набор theirs в памяти до записи. Задача — чистый рефакторинг: вывод на диск побайтно не меняется.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/output.go` (новый) | `OutputFile`, `OutputSet`, `NewOutputSet`, `Add`, `Get`, `Paths`, `Len`, `ChartFiles`, `WriteOutputSet` (сигнатуры — design.md §2.1) |
| `pkg/generator/generator.go` | `WriteChart` = `ChartFiles` + запись. Логику путей и режимов перенести в `ChartFiles` |
| `pkg/generator/output_test.go` (новый) | Unit-тесты (ниже) |
| `cmd/dhg/main.go` | В `runGenerate` после валидации chart'ов собрать `OutputSet` из `ChartFiles` каждого chart'а и файлов env/monorepo/kustomize/SYNTHESIS, затем один вызов `generator.WriteOutputSet` (до задачи 05; потом — `writeOutput`) |
| `cmd/dhg/output.go` (новый) | `envValueFiles`, `monorepoFiles`, `kustomizeFiles`, `synthesisFile` (design.md §2.2). `writeKustomizeDir` удалить, если больше не используется (`deadcode`) |
| `cmd/dhg/commands_test.go` | Тест: вывод `generate` до/после рефакторинга одинаков (ниже) |

## Шаги

1. `OutputSet.Add`: путь нормализуется `path.Clean` и должен быть относительным, без `..` в начале. Иначе — ошибка `invalid output path`. Повтор пути с тем же содержимым и режимом — no-op, с другим — ошибка `duplicate output path <p>`.
2. `ChartFiles(chart)`: тот же порядок и те же условия, что в `WriteChart`: `Helpers`, `Notes`, `ValuesSchema` — только если непустые; `.helmignore` — всегда (`helm.GenerateHelmIgnore()`); `ExternalFiles` — путь `chart.Name + "/" + file.Path` с прежней проверкой выхода за каталог; режим `0755` для `*.sh`, иначе `0644`.
3. Сборщики в `cmd/dhg/output.go` повторяют логику блоков `runGenerate` без записи. `synthesisFile` возвращает файл, печать `Note:` остаётся в вызывающем коде (как сейчас).
4. `WriteOutputSet`: `MkdirAll(dir, 0755)` и `os.WriteFile(path, content, mode)` для каждого файла в порядке `Paths()`.
5. Порядок сообщений `verbose` («Written chart…», «Written: Makefile…») сохраняется: печать — после записи, по тем же условиям.

## Тесты

- Unit (`pkg/generator/output_test.go`):
  - `ChartFiles` для chart'а `{Name: "app", Templates: {"templates/a.yaml": "x"}, Helpers: "h", ExternalFiles: [{Path: "post-renderer/kustomize.sh"}, {Path: "README.md"}]}` даёт ровно пути `app/.helmignore`, `app/Chart.yaml`, `app/README.md`, `app/post-renderer/kustomize.sh` (режим `0755`), `app/templates/_helpers.tpl`, `app/templates/a.yaml`, `app/values.yaml`;
  - `ExternalFiles` с путём `../evil` → ошибка `outside chart directory`;
  - umbrella-подчарт `Name: "app/charts/web"` → пути с префиксом `app/charts/web/`;
  - `Add` дубликата с другим содержимым → ошибка; с тем же → ok;
  - `WriteOutputSet` в `t.TempDir()` + чтение обратно: содержимое и режимы совпадают.
- Unit (`cmd/dhg/commands_test.go`): `TestGenerateOutputUnchanged` — для `tests/integration/fixtures/simple-app` с флагами `--mode umbrella --env-values --monorepo --include-schema` и для `examples/05-full-stack` с `--kustomize --post-renderer --airgap-registry r.example.com`. Множество файлов и sha256 каждого сравниваются с эталоном, снятым до рефакторинга. Эталон — `cmd/dhg/testdata/output-unchanged/<case>.sha256`, формат `sha256sum`, генерируется тестом с `-update`.
- Golden: без изменений; весь набор `tests/golden` проходит (вывод не меняется).

## Критерии приёмки

- [ ] Ни один файл `generate` не пишется в обход `OutputSet` (`grep -n "os.WriteFile" cmd/dhg/main.go` — только вне `runGenerate`).
- [ ] `TestGenerateOutputUnchanged` проходит.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Удаление устаревших файлов, `.dhg/`, слияние — задачи 05 и 06.
