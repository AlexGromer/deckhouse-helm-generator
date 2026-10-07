# EPIC-05 / 04: Возможность `pss-restricted` (opt-in ужесточение)

| Поле | Значение |
|---|---|
| Статус | draft — ждёт решения владельца по Q1 ([README](../README.md#риски-и-открытые-вопросы)) |
| Размер | S (≤ 1 дня) |
| Зависит от | EPIC-05/03 |
| Требования | R20 (из [spec.md](../spec.md)) |

## Контекст

После задачи 03 шаблон pod'а сливает `securityContext` входа с `global.securityContextDefaults`. Синтезированные workload'ы (`pkg/synth/manifests.go:securityContext`) и большинство входных манифестов не выполняют три контроля Restricted, которые не зависят от образа: Seccomp, Privilege Escalation, Capabilities (F14). Возможность закрывает их значениями по умолчанию, не трогая то, что зависит от образа (`runAsNonRoot`, `runAsUser`).

Открытый вопрос (Q1): допустимо ли добавлять не-факты, даже opt-in. Рекомендация — да, только через `--with`, значения видны в `values.yaml` и выключаются удалением `global.securityContextDefaults`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_deckhouse.go` | `RegisterFeature(Feature{Name: "pss-restricted", Params: {}})`, `applyPSSRestrictedFeature` через `setGlobalValue` (задача 03) |
| `pkg/generator/features_deckhouse_test.go` | Unit-тесты |
| `tests/golden/deckhouse_test.go` | `TestPSSRestrictedFeature` |
| `README.md`, `docs/RELEASE_NEXT.md` | Строка возможности |

## Шаги

1. Значения:
   - `global.securityContextDefaults.pod`: `{seccompProfile: {type: RuntimeDefault}}`;
   - `global.securityContextDefaults.container`: `{allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}`.
2. Если ключ `global.securityContextDefaults` уже есть (например, вход прошёл `dhg fix`) — ошибка `pss-restricted: global.securityContextDefaults is already set`.
3. Возможность пропускает chart без workload'ов (возвращает вход без изменений).
4. Комментарий в values над ключом: `# pss-restricted: defaults merged under each pod/container securityContext; values from the input win.`

## Тесты

- Unit: values после `Apply` содержат оба map'а; повторный `Apply` → ошибка; chart без workload'ов не меняется.
- Golden (`TestPSSRestrictedFeature`): `dhg generate -s compose -f tests/golden/testdata/synth/compose/docker-compose.yml --chart-name app --with pss-restricted,deckhouse-report` во всех режимах `synthModes`; у каждого отрендеренного Deployment'а pod-уровень `seccompProfile.type == RuntimeDefault`, у контейнеров `allowPrivilegeEscalation == false`; в `docs/deckhouse-report.md` нарушения Restricted только «Running as Non-root» (если образ без `USER`).

## Критерии приёмки

- [ ] Решение по Q1 зафиксировано в README эпика.
- [ ] Без `--with pss-restricted` вывод dhg не меняется.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Автоматический выбор `runAsUser`/`runAsNonRoot` (не факт), `readOnlyRootFilesystem` (не PSS).
