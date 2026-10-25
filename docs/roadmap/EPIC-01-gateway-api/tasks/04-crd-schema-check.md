# EPIC-01 / 04: golden — проверка полей custom resources по схемам CRD

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M |
| Зависит от | — |
| Требования | R21 (EPIC-01); используется EPIC-04 (AC о полях `CiliumNetworkPolicy`) |

## Контекст

`helm template` не проверяет custom resources: CRD Gateway API и Cilium в golden-окружении не установлены, поэтому опечатка в поле (`backendRef` вместо `backendRefs`) или поле experimental-канала пройдёт `helm lint --strict` и `helm template`. Правило проекта — не выдумывать API и поля. Проверка нужна без кластера и без новых Go-зависимостей: openAPI-схемы из CRD хранятся в `tests/golden/testdata/crds/`, а тест обходит отрендеренные объекты.

## Изменения

| Файл | Что сделать |
|---|---|
| `tests/golden/testdata/crds/gateway-api-v1.6.3-standard/` (новый) | Скопировать из тега `v1.6.3` репозитория `kubernetes-sigs/gateway-api` файлы `config/crd/standard/gateway.networking.k8s.io_{httproutes,grpcroutes,tlsroutes,listenersets,referencegrants,gateways}.yaml`; первой строкой каждого — комментарий `# Source: https://github.com/kubernetes-sigs/gateway-api/blob/v1.6.3/<путь>, Apache-2.0` |
| `tests/golden/testdata/crds/cilium-v1.17.17/ciliumnetworkpolicies.yaml` (новый) | Из тега `v1.17.17` `cilium/cilium`: `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml` (та же схема, что в `modules/021-cni-cilium/crds/cilium/` Deckhouse `main`), комментарий источника и лицензии Apache-2.0 |
| `tests/golden/testdata/crds/cert-manager/certificates.yaml` (новый) | CRD `certificates.cert-manager.io` из релиза cert-manager; версию выбрать актуальную на момент реализации и указать в комментарии (в проработке не проверено) |
| `tests/golden/crdschema_test.go` (новый) | Загрузчик схем и валидатор (ниже) |
| `tests/golden/golden_test.go` | В `checkCharts` после `helm template`: `for _, p := range checkCRSchemas(parseStream(out)) { t.Errorf(...) }` |

## Шаги

1. Загрузчик: `loadCRDSchemas(dir string) map[string]map[string]interface{}` — ключ `<group>/<version>/<Kind>`, значение — `spec.versions[i].schema.openAPIV3Schema` для версий с `served: true`.
2. Валидатор `validateAgainstSchema(path string, v interface{}, schema map[string]interface{}) []string`:
   - `type: object` с `properties` — каждый ключ значения есть в `properties`, иначе `"<path>.<key>: field not in schema"`; ключи из `required` присутствуют; `x-kubernetes-preserve-unknown-fields: true` или `additionalProperties` — пропустить проверку неизвестных ключей (для `additionalProperties` со схемой — проверять значения по ней);
   - `type: array` — `maxItems`, рекурсия в `items`;
   - `type: string` — `enum`, `maxLength`, `pattern` (Go `regexp`; если шаблон не компилируется в RE2 — пропустить и записать в лог теста `t.Log` один раз на шаблон);
   - `type: integer` — `minimum`, `maximum`, `enum`;
   - `x-kubernetes-int-or-string: true` — допускать число или строку;
   - правила CEL (`x-kubernetes-validations`) не вычисляются: их покрывают unit-тесты features.
3. `checkCRSchemas(objs []object) []string`: для каждого объекта `apiVersion`+`kind` из таблицы — проверить `spec` (и `metadata` не проверять); объекты неизвестных GVK пропустить.
4. Подключить в `checkCharts`. Запустить весь golden-набор: если существующий вход (`examples/12-gateway-api`, `fixtures/gateway-app`) не проходит — это находка: исправить вход, если он сам невалиден, или процессор, если он теряет/искажает поле; описать в PR.

## Тесты

- Unit (в том же пакете, без Helm) `tests/golden/crdschema_test.go`:
  - `TestValidateAgainstSchema_UnknownField`: `HTTPRoute` с `spec.rules[0].backendRef` → одна ошибка с путём `spec.rules[0].backendRef`.
  - `TestValidateAgainstSchema_ExperimentalField`: `spec.rules[0].retry` по standard-схеме v1.6.3 → ошибка.
  - `TestValidateAgainstSchema_Enum`: `requestRedirect.statusCode: 300` → ошибка enum; `308` → ок.
  - `TestValidateAgainstSchema_MaxItems`: 17 правил → ошибка `maxItems`.
  - `TestValidateAgainstSchema_CNP`: `CiliumNetworkPolicy` с `egress[0].toFQDNs[0].matchName` и `toPorts[0].ports[0].protocol: TCP` → ок; `protocol: ICMP` → ошибка enum (`TCP;UDP;SCTP;ANY`).
- Golden: весь набор (`DHG_REQUIRE_HELM=1 go test ./tests/golden/`) проходит с новой проверкой.

## Критерии приёмки

- [ ] Каждый отрендеренный объект групп `gateway.networking.k8s.io`, `cilium.io`, `cert-manager.io` (`Certificate`) во всех golden-тестах проверяется по схеме.
- [ ] Unit-тесты выше проходят; искусственно добавленное в шаблон неизвестное поле роняет golden-тест (проверить вручную, в PR — вывод).
- [ ] Источники и лицензии CRD указаны в файлах.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Вычисление правил CEL; проверка `metadata`; другие CRD (добавляются тем же способом в своих задачах).
