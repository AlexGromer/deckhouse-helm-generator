# EPIC-05 / 01: Пакет проверки Pod Security Standards

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | — |
| Требования | R1, R2 (из [spec.md](../spec.md)) |

## Контекст

В dhg нет кода, который отвечает на вопрос «какому уровню PSS соответствует pod». Есть:

- `pkg/generator/pss.go` — текстовая вставка `securityContext` (с ошибкой, см. задачу 03), без оценки;
- `pkg/generator/policyascode.go` — каталог правил Kyverno/Rego для `--with policies` (часть правил помечена «Pod Security Standards (Baseline/Restricted)», но это шаблоны политик, а не оценка в Go).

Нужна чистая функция оценки pod spec'а по таблице Kubernetes PSS (spec.md, факт F14). Её используют `deckhouse-report` (задача 02) и `dhg fix` (задача 03).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/compliance/pss/pss.go` (новый) | Типы `Level`, `Violation`, `Result`; функции `Evaluate`, `PodSpec`, `ParseLevel`, `Result.ViolationsOf` (сигнатуры — [design.md §2.1](../design.md#21-pkgcompliancepss-новый-пакет-задача-01)) |
| `pkg/compliance/pss/controls.go` (новый) | Таблица контролей: по функции на контроль, константы разрешённых значений (capabilities Baseline, sysctls, типы томов, SELinux-типы) |
| `pkg/compliance/pss/doc.go` (новый) | Комментарий пакета со ссылкой на источник F14 и датой сверки 2026-10-07 |
| `pkg/compliance/pss/pss_test.go` (новый) | Табличные тесты |

## Шаги

1. Создать пакет. Константы (точно по F14):
   - Baseline capabilities `add`: `AUDIT_WRITE, CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, MKNOD, NET_BIND_SERVICE, SETFCAP, SETGID, SETPCAP, SETUID, SYS_CHROOT`;
   - безопасные sysctls: `kernel.shm_rmid_forced, net.ipv4.ip_local_port_range, net.ipv4.ip_unprivileged_port_start, net.ipv4.tcp_syncookies, net.ipv4.ping_group_range, net.ipv4.ip_local_reserved_ports, net.ipv4.tcp_keepalive_time, net.ipv4.tcp_fin_timeout, net.ipv4.tcp_keepalive_intvl, net.ipv4.tcp_keepalive_probes`;
   - SELinux `type`: `""`, `container_t`, `container_init_t`, `container_kvm_t`, `container_engine_t`; `user`/`role` — только пусто;
   - AppArmor: `appArmorProfile.type` ∈ {не задано, `RuntimeDefault`, `Localhost`}; аннотации `container.apparmor.security.beta.kubernetes.io/*` ∈ {`runtime/default`, `localhost/*`} (читаются из `metadata.annotations` pod-шаблона — поэтому `Evaluate` принимает и аннотации, см. сигнатуру в design §2.1);
   - Restricted: типы томов `configMap, csi, downwardAPI, emptyDir, ephemeral, persistentVolumeClaim, projected, secret`; `add` ⊆ {`NET_BIND_SERVICE`}; `drop` ∋ `ALL`.
2. `PodSpec(obj)`: Deployment/StatefulSet/DaemonSet/ReplicaSet/Job/`argoproj.io` Rollout → `spec.template.spec`; CronJob → `spec.jobTemplate.spec.template.spec`; Pod → `spec`; иначе `ok=false`. Аннотации — из соседнего `metadata.annotations` (у Pod — `metadata.annotations` объекта).
3. Реализовать алгоритм [design.md §4.1](../design.md#41-оценка-pss-r1). `Field` — путь с индексами (`spec.containers[1].securityContext.capabilities.add[0]`), путь от корня pod spec'а с префиксом `spec.` (как в таблице стандарта). Порядок `Violations` — по `Field`, затем по `Control`.
4. Host Probes/Lifecycle Hooks: `livenessProbe|readinessProbe|startupProbe.httpGet.host|tcpSocket.host`, `lifecycle.postStart|preStop.httpGet.host|tcpSocket.host` — допустимо только не задано или `""`.
5. Host Ports: `ports[].hostPort` — допустимо не задано или `0` («known list» не поддерживается, как во встроенном PSA).
6. `runAsUser`: на уровне pod'а и контейнеров; `0` — нарушение Restricted.

## Тесты

- Unit (`pkg/compliance/pss/pss_test.go`), табличные, по каждому контролю — минимум одно нарушение и одно допустимое значение:
  - пустой pod spec `{containers: [{name: a, image: x}]}` → `Level == Baseline`; нарушения Restricted ровно: `Privilege Escalation` (`spec.containers[0].securityContext.allowPrivilegeEscalation`), `Running as Non-root`, `Seccomp`, `Capabilities` (drop);
  - pod `securityContext: {runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}`, контейнер `{allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}` → `Restricted`, нарушений 0;
  - то же + контейнер `runAsNonRoot: false` → нарушение Restricted «Running as Non-root» на `spec.containers[0].securityContext.runAsNonRoot`;
  - `hostNetwork: true` → `Privileged`, нарушение Baseline «Host Namespaces» `spec.hostNetwork`;
  - `volumes: [{name: d, hostPath: {path: /x}}]` → `Privileged`, «HostPath Volumes» `spec.volumes[0].hostPath`;
  - `capabilities.add: [NET_ADMIN]` → Baseline-нарушение; `[NET_BIND_SERVICE]` при `drop: [ALL]` → допустимо и для Restricted;
  - `ports: [{containerPort: 80, hostPort: 8080}]` → нарушение; `hostPort: 0` → нет;
  - `livenessProbe.httpGet.host: "10.0.0.1"` → нарушение «Host Probes / Lifecycle Hooks»;
  - `seccompProfile.type: Unconfined` у контейнера → нарушение Baseline;
  - `sysctls: [{name: kernel.msgmax, value: "1"}]` → нарушение; `net.ipv4.tcp_syncookies` → нет;
  - `seLinuxOptions.type: spc_t` → нарушение; `container_engine_t` → нет; `seLinuxOptions.user: u` → нарушение;
  - `procMount: Unmasked` → нарушение;
  - `appArmorProfile.type: Unconfined` → нарушение; аннотация `container.apparmor.security.beta.kubernetes.io/a: unconfined` → нарушение;
  - `windowsOptions.hostProcess: true` → нарушение;
  - том `nfs` → нарушение Restricted «Volume Types», Baseline — нет;
  - `os.name: windows` + пустой securityContext → Restricted-нарушения Privilege Escalation/Seccomp/Capabilities отсутствуют;
  - `runAsUser: 0` у pod'а → нарушение «Running as Non-root user»;
  - init- и ephemeral-контейнеры проверяются (`spec.initContainers[0]…`, `spec.ephemeralContainers[0]…`);
  - поле неверного типа (`privileged: "yes"` строкой) → не паника, нарушение;
  - `PodSpec` для CronJob, Pod, ConfigMap (`ok=false`);
  - `ParseLevel("Restricted")`, `ParseLevel("bad")` → ошибка.
- Golden: не требуется (пакет не меняет chart).

## Критерии приёмки

- [ ] Каждый контроль F14 покрыт допустимым и нарушающим примером; покрытие пакета ≥ 90 %.
- [ ] Пакет не импортирует `pkg/generator`, `pkg/processor`, `cmd`; новых модулей в `go.mod` нет.
- [ ] `readOnlyRootFilesystem` не влияет на результат (отдельный тест).
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Отчёт (задача 02), изменение шаблона pod'а и `dhg fix` (задача 03), вывод в `dhg analyze` (развитие, design §10).
