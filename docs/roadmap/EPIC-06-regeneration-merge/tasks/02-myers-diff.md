# EPIC-06 / 02: Myers diff, unified diff и исправление `dhg diff`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M |
| Зависит от | — |
| Требования | R28, R29 |

## Контекст

`printUnifiedDiff` (`cmd/dhg/main.go:1399`) сравнивает `lines1[i]` с `lines2[i]` по позиции. После одной вставленной строки все последующие помечаются как изменённые. `runDiff` (`main.go:1279`) всегда возвращает `nil`. Для diff3 (задача 03) нужен алгоритм LCS. Его же используем для `dhg diff`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/merge/doc.go` (новый) | Комментарий пакета: назначение, ссылки на Myers (1986) и Khanna–Kuber–Pierce (2007) |
| `pkg/merge/lines.go` (новый) | `SplitLines` |
| `pkg/merge/diff.go` (новый) | `EditKind`, `Edit`, `Diff` (Myers, design.md §4.1) |
| `pkg/merge/unified.go` (новый) | `Unified` |
| `pkg/merge/diff_test.go`, `unified_test.go` (новые) | Тесты ниже |
| `cmd/dhg/main.go` | `printUnifiedDiff` → `merge.Unified` с раскраской строк `+`/`-`/`@@` как сейчас; флаг `diff --exit-code` (exit 1 при различиях через `exitCodeError`, см. задачу 06, или локальный `errDiffFound`, если 06 ещё нет); `collectFiles` пропускает каталог `.dhg` |
| `cmd/dhg/commands_test.go` | Тесты `dhg diff` |

## Шаги

1. `SplitLines("a\nb")` → `["a\n", "b"]`; `SplitLines("")` → `[]`; `"\r\n"` остаётся в строке.
2. `Diff(a, b)` — жадный Myers с trace и обратным проходом. Возвращает полный скрипт, включая `Equal`. Для `a == b` — только `Equal`.
3. `Unified(aName, bName, a, b, 3)`: заголовки `--- aName`, `+++ bName`, hunk'и по правилам design.md §4.1. Для строки без `\n` в конце после неё выводится `\ No newline at end of file`.
4. В `runDiff` для изменённого файла вместо `printUnifiedDiff` вызвать `merge.Unified(dir1+"/"+rel, dir2+"/"+rel, …)`. Заголовок `@@ modified @@` из `printDiffHeader` убрать (его заменяют заголовки `---`/`+++`). Для `added`/`removed` оставить текущий вывод.

## Тесты

- Unit `Diff`, таблица (a → b → ожидаемая последовательность видов правок):
  - `[] → []` → пусто;
  - `[x] → [x]` → `E`;
  - `[a b c] → [x a b c]` → `I E E E`;
  - `[a b c a b b a] → [c b a b a c]` (пример из статьи Myers) → длина скрипта D = 5 (5 правок не-`Equal`), применение скрипта к a даёт b;
  - свойство на 200 случайных парах (seed фиксирован): применение скрипта к a даёт b; число `Equal` = длина LCS, посчитанной наивным O(NM) DP в тесте.
- Unit `Unified`: вставка первой строки в файл из 10 строк → ровно один hunk `@@ -1,3 +1,4 @@`, одна строка `+`, ни одной `-`; файлы без `\n` в конце → маркер `\ No newline at end of file`; две правки на расстоянии 10 строк → два hunk'а, на расстоянии 5 → один.
- Unit `cmd/dhg`: `dhg diff d1 d2 --color=false`, где `d2/x.yaml` = `d1/x.yaml` с новой первой строкой → в выводе одна строка, начинающаяся с `+`, и ни одной с `-` (AC8 эпика); `--exit-code` → ошибка с кодом 1; одинаковые каталоги + `--exit-code` → nil и «No differences found.»; файл в `d1/.dhg/` не попадает в вывод.

## Критерии приёмки

- [ ] AC8 эпика выполняется.
- [ ] Покрытие `pkg/merge` ≥ 90 %.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

diff3 — задача 03. Цветовая схема `dhg diff` не меняется.
