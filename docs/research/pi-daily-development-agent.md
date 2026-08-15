# Pi como agente excelente para el desarrollo diario: lecciones para Matty

> Investigación primaria y recomendaciones para Matty Core v0.1 y la visión posterior de “Pi para el desarrollo diario”.
>
> **Nota de decisión:** los ADR posteriores gobiernan el roadmap. Las opciones
> exploradas aquí —en particular telemetry y backends hipotéticos— no reabren
> Zero Telemetry ni autorizan abstracciones antes de que exista variación real.

## Resumen ejecutivo

Pi funciona mejor como **harness mínimo y extensible**, no como un agente monolítico. Su configuración por defecto entrega cuatro herramientas (`read`, `write`, `edit`, `bash`), mientras que extensiones, skills, prompts y paquetes agregan comportamiento; además expone modos interactivo, print/JSON, RPC y SDK. Pi deliberadamente no incorpora de serie subagentes, plan mode, permisos emergentes, to-dos ni background bash ([README oficial](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md#philosophy)). Esta separación es la principal lección arquitectónica para Matty.

**Recomendación central:** Matty Core v0.1 debe ser una base pequeña de runtime/capabilities con contratos estables para ejecución, políticas, eventos, artefactos, cancelación, presupuestos y reanudación. Los flujos de producto —subagentes, memoria, revisión, verificación, autonomía AFK, dashboards y routing de modelos— deben componerse después como capacidades o paquetes. Esta división conserva el contexto útil y evita convertir cada tarea pequeña en una costosa ceremonia de orquestación.

Prioridades:

1. **P0, Core v0.1:** manifiesto tipado de capacidades; autoridad mínima por rol; ciclo de vida observable; cancelación/timeouts/presupuestos; artefactos estructurados y reanudables; política de un solo escritor; resultado de verificación basado en evidencia; controles de instalación/proveniencia.
2. **P1, primer flujo diario:** `clarify → inspect → implement → deterministic verify → fresh review → fix → human diff review`, con rutas cortas para tareas acotadas.
3. **P1, control:** inspeccionar, pausar, abortar y dirigir trabajo en curso; aprobaciones según riesgo, no popups indiscriminados.
4. **P2:** memoria local, auditable, acotada y corregible; subagentes paralelos de solo lectura; worktrees para escritores aislados; observabilidad local. Cualquier export OpenTelemetry requeriría reabrir explícitamente Zero Telemetry y no forma parte de la recomendación vigente.
5. **P3:** loops AFK medibles como autoresearch, selección automática de modelo y automatización recurrente, solo después de contar con aislamiento, presupuestos y evaluaciones propias.

La evidencia externa respalda la disciplina de contexto, no una receta universal: en un benchmark interno de tareas reales, Databricks observó que el mismo modelo/esfuerzo podía costar más de 2× según el harness y que Pi reenviaba alrededor de 3× menos contexto por turno; también aclara que su benchmark no es exhaustivo ni universal ([Databricks](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#harnesses-have-a-major-impact-on-efficiency)). Matty debe, por tanto, medir calidad, coste y duración **en sus propios flujos**, no optimizar por anécdotas o precio por token.

## Alcance y método

Los dos puntos de partida se trataron como índices secundarios:

- El [diccionario de AI Hero](https://www.aihero.dev/ai-coding-dictionary) ayudó a ordenar conceptos —harness, contexto, handoff, skill, subagente, verificación—, pero no se usa como autoridad para decisiones técnicas.
- El artículo de [Earendil](https://earendil.com/posts/pi-autoresearch-and-databricks/) condujo a los casos de Databricks y Shopify; sus afirmaciones importantes se comprobaron en las fuentes originales.

La base factual del informe son la [documentación y código oficial de Pi](https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent), la [especificación Agent Skills](https://agentskills.io/specification), repositorios originales de extensiones y publicaciones first-party de [Shopify](https://shopify.engineering/autoresearch) y [Databricks](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase). Las cifras publicadas por un proyecto sobre sí mismo se presentan como resultados auto-reportados, no como validación independiente.

Para la separación de roadmap se usa únicamente el alcance indicado: **Matty Core v0.1 es una foundation de runtime/capabilities**. No se presume que el repositorio ya implemente o carezca de componentes concretos.

## 1. Qué hace valioso a Pi

### 1.1 Núcleo mínimo, superficies potentes

Pi mantiene pequeño el camino normal, pero ofrece cuatro superficies complementarias:

- **Contexto estable:** carga `AGENTS.md`/`CLAUDE.md` desde el ámbito global y la jerarquía del proyecto; `SYSTEM.md` puede reemplazar y `APPEND_SYSTEM.md` ampliar el prompt ([README: Context Files](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md#context-files)).
- **Skills bajo demanda:** al inicio solo se incorporan nombre y descripción; el modelo lee `SKILL.md` cuando la tarea coincide. Es progressive disclosure y reduce contexto permanente ([docs de Skills](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/skills.md#how-skills-work)). La especificación recomienda instrucciones menores de 5.000 tokens, `SKILL.md` menor de 500 líneas y referencias cargadas según necesidad ([Agent Skills: Progressive disclosure](https://agentskills.io/specification#progressive-disclosure)).
- **Extensiones programables:** pueden registrar tools/comandos/UI, interceptar o modificar llamadas, bloquear herramientas, inyectar contexto, personalizar compaction y persistir entradas ([docs de Extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md#extensions)).
- **Runtime embebible:** `AgentSession` ofrece prompt, steering/follow-up, eventos, abort y compaction; `AgentSessionRuntime` reemplaza sesiones al crear, reanudar, importar o bifurcar ([SDK oficial](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/sdk.md#createagentsessionruntime-and-agentsessionruntime)).

**Aplicación a Matty:** el núcleo debe definir los mecanismos; la política y los workflows deben vivir en componentes reemplazables. Una capability declarativa es preferible a añadir instrucciones globales para cada caso.

### 1.2 Context engineering: cargar menos, pero mejor

Un buen contexto diario tiene tres capas:

1. **Brief estable y corto:** convenciones, comandos y límites del repositorio en `AGENTS.md`.
2. **Skill activable:** procedimiento especializado, ejemplos y punteros a referencias.
3. **Artefacto de tarea:** objetivo, restricciones, decisiones, progreso, evidencia y próximos pasos.

Pi preserva sesiones como árboles JSONL y permite branching/fork; la compactación conserva mensajes recientes y resume los antiguos, pero es explícitamente lossy, mientras el historial completo sigue en el archivo ([README: Sessions/Compaction](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md#sessions)). Su resumen estructurado incluye objetivo, restricciones, progreso, decisiones, próximos pasos, contexto crítico y archivos leídos/modificados ([docs de Compaction](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/compaction.md#summary-format)).

**Patrón:** usar artefactos explícitos como fuente de handoff y la compaction como optimización, no como memoria autorizada. Un resumen debe apuntar a código, tests, diff y decisiones primarias, no sustituirlos.

### 1.3 Steering y control sin romper el flujo

Pi distingue mensajes de **steering**, entregados después de las tool calls del turno actual, y **follow-up**, entregados cuando el agente termina; Escape aborta y devuelve los mensajes en cola ([README: Message Queue](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md#message-queue)). El SDK reproduce esas primitivas con `steer()`, `followUp()` y `abort()` ([SDK: Prompting and Message Queueing](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/sdk.md#prompting-and-message-queueing)).

Esto es más útil que una dicotomía “manual/autónomo”: Matty debería modelar autonomía como un continuo con **inspect, steer, pause, abort y approve** disponibles durante la ejecución.

## 2. Implementaciones reales comparadas

| Implementación | Qué aporta | Patrón transferible | Límite o advertencia |
|---|---|---|---|
| [Pi oficial](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md) | Runtime, sesiones arbóreas, eventos, herramientas, recursos, SDK/RPC | Núcleo pequeño; capacidades reemplazables; intervención durante el turno | No ofrece sandbox ni permisos de tool incorporados; corre con la autoridad del usuario ([Security](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/security.md#no-built-in-sandbox)) |
| [pi-autoresearch](https://github.com/davebcn87/pi-autoresearch) | Loop `editar → medir → conservar/revertir → repetir`, log JSONL, checks y dashboard | Objetivo cuantificable, baseline, log append-only, backpressure determinista, reanudación por artefacto | Solo es correcto si la métrica y checks capturan calidad; el propio caso Shopify relata “hacks” que aceleraban eliminando trabajo válido ([Shopify](https://shopify.engineering/autoresearch#the-magic-of-autoresearch)) |
| [pi-subagents](https://github.com/nicobailon/pi-subagents) | Roles, ejecución fresh/forked, workflows, background, FleetView, worktrees, artefactos | `clarify → scout → worker → fresh reviewers → worker`; reviewers frescos; un escritor o worktrees; límites de profundidad | Mucha superficie y coste potencial; los hijos normalmente no reciben tool de subagentes y existe guard de profundidad para evitar recursión ([Workflows](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#recursion-guard)) |
| [pi-memory](https://github.com/chendpoc/pi-memory) | Markdown local, recall pre-turn, extracción en compaction/shutdown, redacción, consolidación | Memoria visible/corregible, scope distinto para root/subagente, crecimiento acotado y degradación segura | La recuperación automática puede introducir contexto obsoleto o secretos no detectados; debe conservar provenance y permitir revisión |
| [Chalin](https://github.com/0xkuze/pi-chalin) | Routing, DAGs, memoria, aprobaciones, single-writer, artefactos, paneles y evals | Ruta directa para trabajo pequeño; routing por amplitud/riesgo; políticas y evaluaciones como parte del producto | Es una “modular monolith” amplia; sus propias evals admiten que la comparación routed completa sigue pendiente y que una muestra routed usó más tokens ([reporte del proyecto](https://github.com/0xkuze/pi-chalin/blob/main/docs/chalin-vs-gentle-eval-results.md#seguimiento-routed)) |
| [pi-verify-all](https://github.com/saburto/pi-verify-all) | Pipeline declarativo de formato/tests/build/E2E, health checks, timeout, logs y retry | Contrato de verificación versionado y ejecutable; evidencia por paso; retry acotado | Reintentar automáticamente tras `agent_end` puede formar loops y ejecutar comandos peligrosos; requiere presupuesto y clasificación de riesgo |
| [pi-opentelemetry](https://github.com/devkade/pi-opentelemetry) | Trazas `session → agent → turn → tool`, métricas de tokens/coste/duración y redacción | Esquema estándar, cardinalidad controlada, exporters no bloqueantes, perfil de privacidad | Capturar prompts, paths o outputs puede exfiltrar código/secretos incluso con redacción; “strict” debe estar disponible |

### 2.1 Autoresearch: autonomía ganada por medición

La implementación original separa **infraestructura genérica** (tools de experimento, widget y log) de **skill de dominio** (comando, métrica, alcance e ideas), y persiste `.auto/prompt.md`, `.auto/measure.sh`, `.auto/log.jsonl` y checks opcionales ([README de pi-autoresearch](https://github.com/davebcn87/pi-autoresearch#how-it-works)). Cada resultado queda en un log append-only y el loop conserva mejoras o revierte regresiones; los checks de tipos/tests/lint bloquean un `keep` aun cuando mejore la métrica ([README: session files](https://github.com/davebcn87/pi-autoresearch#whats-included)).

Shopify reporta mejoras reales —por ejemplo, un caso de build 65% más rápido—, pero también que el agente proponía atajos inaceptables y que el humano descartó hacks ([Shopify](https://shopify.engineering/autoresearch#the-magic-of-autoresearch)). Por ello, el patrón generalizable no es “loop infinito”; es:

> hipótesis registrada → cambio reversible → medición repetible → invariantes obligatorios → decisión keep/revert → presupuesto/stop → revisión humana.

### 2.2 Subagentes: aislamiento de contexto antes que “más inteligencia”

`pi-subagents` usa hijos especializados (`scout`, `researcher`, `worker`, `reviewer`, `oracle`) y recomienda reviewers con contexto fresco después de implementar ([README](https://github.com/nicobailon/pi-subagents#builtin-agents)). En workflows con escritura paralela puede crear worktrees separados; si no se aísla, recomienda mantener un solo escritor ([Workflows: Worktree isolation](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#worktree-isolation)). Sus artefactos de ejecución contienen estado, eventos JSONL, logs, modelo, tokens, coste, tool/turn count y paths; la documentación pide consumir JSON en vez de raspar la terminal ([Observability: Async run artifacts](https://github.com/nicobailon/pi-subagents/blob/main/docs/observability.md#async-run-artifacts)).

**Aplicación:** Matty no necesita “swarm” en v0.1; necesita un contrato de child-run que haga posible después:

- contexto `fresh | fork | artifact-only`;
- tools/capabilities permitidas por rol;
- cwd/worktree y política de escritura;
- límites de profundidad, hijos, concurrencia, tiempo, tokens y coste;
- un único resultado estructurado con evidencia y artefactos;
- cancelación propagada y estado terminal demostrable.

### 2.3 Memoria: conocimiento curado, no historial ilimitado

`pi-memory` diferencia la compaction de una sesión de la memoria entre sesiones, guarda notas en Markdown, recuerda contexto antes de responder, redacta patrones comunes de secretos, limita el archivo principal y consolida offline ([README](https://github.com/chendpoc/pi-memory#what-it-does)). También reduce el contexto de memoria para subagentes y continúa sin inyección si retrieval falla ([README: Key Advantages](https://github.com/chendpoc/pi-memory#key-advantages)).

La lección no es adoptar inmediatamente embeddings o SQLite. Primero debe existir un registro auditable con:

- contenido, scope (`user`, `project`, `task`, `role`), autor/origen y timestamp;
- evidencia o puntero que lo sustenta;
- estado `candidate | approved | superseded | rejected`;
- TTL/revisión, redacción y borrado;
- retrieval con presupuesto y explicación de por qué se incluyó.

### 2.4 Verificación y revisión son capas distintas

Databricks evaluó correctness con tests retenidos, no con un LLM judge, porque observó que el judge favorecía “sonar correcto”; además aisló el historial Git tras detectar que el agente podía recuperar la solución original ([Databricks: Task Construction y Guardrails](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#task-construction)). `pi-verify-all` muestra cómo declarar pasos, condiciones, timeouts, health checks y logs, y cómo exponer el pipeline al agente como tool ([repositorio original](https://github.com/saburto/pi-verify-all#configuration)).

Por tanto:

- **checks deterministas** deciden si compila, testea, tipa o cumple invariantes observables;
- **reviewers frescos** buscan errores, falta de tests, seguridad y complejidad no capturada;
- **humano** revisa el diff y decisiones de producto/riesgo.

Ninguna capa sustituye a las otras.

## 3. Recomendaciones para Matty Core v0.1

Estas mejoras son compatibles con un alcance de foundation; no requieren entregar todavía un agente diario completo.

### P0.1 — Contrato uniforme de capability

Definir una capability como datos + implementación:

```ts
interface CapabilityManifest {
  id: string;
  version: string;
  kind: "tool" | "role" | "workflow" | "policy" | "observer";
  inputSchema: JsonSchema;
  outputSchema: JsonSchema;
  effects: Array<"read" | "write" | "exec" | "network" | "credentials" | "spawn">;
  scopes: string[];              // paths, hosts, comandos o recursos
  concurrency: "parallel-safe" | "single-writer" | "exclusive";
  cancellable: boolean;
  provenance: { source: string; integrity?: string };
}
```

Pi ya permite allowlist/exclusión de tools y custom tools tipadas; las extensiones pueden bloquear llamadas en `tool_call` ([SDK: Tools](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/sdk.md#tools), [Extensions: tool_call](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md#tool_call)). Matty debe elevar esa posibilidad a contrato de runtime, no depender de instrucciones textuales.

**Criterio de aceptación:** antes de ejecutar, el runtime puede responder “qué capability, efectos, scopes, origen y política autorizaron esta acción”.

### P0.2 — Run/child-run state machine y eventos estables

Estados mínimos: `queued → starting → running → waiting_approval|paused → succeeded|failed|cancelled|timed_out`. Eventos: run/turn/tool start-update-end, policy decision, budget update, artifact emitted, verification result y terminal proof.

Pi expone eventos de sesión, agente, turno, mensaje, provider y tool, incluyendo `agent_settled`, que distingue el final real de un `agent_end` seguido por retry/compaction/follow-up ([Extensions: Lifecycle Overview](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md#lifecycle-overview)). `pi-subagents` demuestra el valor de JSONL y snapshots machine-readable, y advierte no inferir terminación solo de timestamps o desaparición de PID ([Observability: Process-terminal proof](https://github.com/nicobailon/pi-subagents/blob/main/docs/observability.md#process-terminal-proof)).

**Criterio de aceptación:** consumidores pueden reconstruir estado tras crash sin parsear texto de UI y sin confundir “respuesta acabó” con “workflow quedó settled”.

### P0.3 — Presupuestos, cancelación y backpressure

Todo run debe admitir límites de wall time, turns, tool calls, bytes de output, tokens/coste, hijos, profundidad y concurrencia. AbortSignal/cancel debe propagarse a tools e hijos. El output grande debe truncarse con puntero al artefacto completo.

Pi ya entrega abort y señales a tools/compaction ([SDK: AgentSession](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/sdk.md#agentsession)); `pi-subagents` aplica límites de profundidad y protocolo/output ([Workflows: Recursion guard](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#recursion-guard), [Observability: Child-protocol bounds](https://github.com/nicobailon/pi-subagents/blob/main/docs/observability.md#child-protocol-bounds)).

### P0.4 — Autoridad mínima y Single Writer

Separar capacidades por rol: inspector/reviewer sin escritura; worker con escritura solo en workspace confiable; researcher sin shell; operaciones de release/deploy fuera de autoridad por defecto. Paralelizar lecturas; serializar escritura, salvo worktrees explícitamente aislados.

Los propios ejemplos oficiales de Pi incluyen permission gate, paths protegidos, dirty-repo guard y sandbox/Gondolin ([Extension Examples: Lifecycle & Safety](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/README.md#lifecycle--safety)). Esto muestra que policy hooks son útiles, pero la documentación aclara que trust de proyecto **no es sandbox** ([Security: Project Trust](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/security.md#project-trust)).

### P0.5 — Artefactos y handoffs tipados

Un run debe producir un `RunResult` pequeño:

```ts
interface RunResult {
  status: "succeeded" | "failed" | "partial" | "blocked";
  summary: string;
  decisions: Array<{ claim: string; rationale: string; evidence: ArtifactRef[] }>;
  changes: ArtifactRef[];        // diff/patch/commits
  verification: CheckResult[];   // comando, exit, duración, log, entorno
  risks: string[];
  nextActions: string[];
  usage: { tokens?: number; cost?: number; durationMs: number };
}
```

Persistir eventos append-only y blobs/logs aparte; los mensajes al modelo reciben proyecciones acotadas. Es coherente con el árbol JSONL de Pi y sus detalles persistidos por tool para reconstrucción al bifurcar ([Extension Examples: State persistence](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/README.md#key-patterns)).

### P0.6 — Verification contract

Core no necesita conocer npm, pytest o Bazel; sí debe modelar `CheckSpec` y `CheckResult`: comando/capability, cwd, env allowlist, timeout, precondición, severidad, exit code, duración, stdout/stderr artifact y cache key. El workflow decide cuándo correrlo.

**Regla:** ninguna completion de implementación se marca “verified” sin resultados ejecutados; ausencia de checks es `unverified`, no éxito implícito. La metodología Databricks refuerza el uso de tests conductuales retenidos y revisión manual de la calidad del benchmark ([Databricks](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#why-build-your-own-benchmark)).

### P0.7 — Supply chain y aislamiento como interfaces

Los paquetes Pi ejecutan código arbitrario con acceso completo y los skills pueden inducir cualquier acción; la documentación exige revisar fuentes antes de instalar ([README: Pi Packages](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/README.md#pi-packages)). Matty debe registrar source/version/integrity, permitir pins y deny-by-default para capabilities nuevas. Para trabajo no confiable o AFK, la frontera debe ser OS/VM/container; Pi recomienda contenedor, VM/micro-VM, credenciales mínimas y red restringida ([Security: Running Untrusted or Unmonitored Work](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/security.md#running-untrusted-or-unmonitored-work)).

Core v0.1 no debe introducir todavía un `ExecutionBackend` abstracto: hoy sólo existe el host como adapter real. Si un backend aislado se vuelve una necesidad concreta, ese segundo adapter justificará extraer el seam y certificarlo.

## 4. Funcionalidades posteriores para “Pi diario”

No deben bloquear v0.1, pero sí apoyarse en sus contratos.

### P1 — Workflow cotidiano adaptativo

Ruta directa para cambios pequeños; ruta orquestada solo por amplitud, incertidumbre o riesgo:

```text
1. Intake: reformular objetivo, restricciones y acceptance criteria.
2. Decide route:
   - bounded/read-only/small edit → ejecución directa;
   - desconocido → scout fresh/read-only;
   - decisión riesgosa → oracle/design review;
   - implementación amplia → plan/artifact + worker.
3. Implement: un escritor; checkpoints reversibles.
4. Verify: checks deterministas de menor a mayor coste.
5. Review: reviewer fresh, read-only, contra objetivo + diff + resultados.
6. Fix: mismo writer aplica hallazgos aceptados; máximo N rondas.
7. Human gate: diff, riesgos, tests y acciones externas.
8. Handoff: RunResult y artefactos; memoria candidata separada.
```

El patrón de reviewers frescos y worker único procede de `pi-subagents` ([Workflows](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#recommended-orchestration-pattern)); el checkpoint Git por turno existe como ejemplo oficial de extensión ([git-checkpoint.ts](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/git-checkpoint.ts)).

### P1 — Control humano por nivel de riesgo

- **Auto:** lectura, búsqueda, tests conocidos y edición dentro del worktree.
- **Notify:** cambios amplios pero reversibles, dependencias o comandos costosos.
- **Approve:** escritura fuera de scope, red con datos, secretos, migraciones, force/reset, publicación/deploy.
- **Deny:** escalada de privilegios o acción fuera de policy.

El hook `tool_call` de Pi puede bloquear o pedir confirmación y sus ejemplos protegen `.env`, `.git` y `node_modules` ([Extensions quick start](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md#quick-start), [protected-paths.ts](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/examples/extensions/protected-paths.ts)). Un popup por cada llamada sería un anti-patrón: fatiga al usuario y elimina autonomía sin crear una frontera real.

### P2 — Memoria auditable

Empezar con Markdown/JSON local y UI de candidatos; no auto-promover conclusiones del modelo. Recuperar top-k con budget, scope y provenance; permitir corregir/superseder. Redactar antes de persistir, pero asumir que redacción regex es incompleta. El diseño local, acotado y fail-open de `pi-memory` es una referencia útil ([repositorio](https://github.com/chendpoc/pi-memory)).

### P2 — Observabilidad útil, no vigilancia

Panel local por run con objetivo, etapa, children, tool actual, duración, budget, cambios y checks. La política vigente de Zero Telemetry excluye exporters y envío de métricas. `pi-opentelemetry` sigue siendo evidencia útil sobre esquemas, cardinalidad y riesgos de payload ([README](https://github.com/devkade/pi-opentelemetry#why)), y OpenTelemetry documenta la separación de traces, metrics y logs ([especificación OTel](https://opentelemetry.io/docs/specs/otel/overview/)); adoptar cualquier export requeriría una nueva decisión explícita sobre consentimiento, retención y privacidad.

### P3 — Autonomía AFK y autoresearch

Habilitar solo si existen sandbox, worktree efímero, checks obligatorios, presupuesto, stop conditions, keep/revert transaccional y dashboard. Cada loop debe declarar métrica primaria, invariantes, baseline/noise, máximo de iteraciones/tiempo/coste y criterio de aceptación humana. `pi-autoresearch` aporta confidence/noise y checks posteriores al benchmark ([README](https://github.com/davebcn87/pi-autoresearch#ui)); Shopify confirma tanto el potencial como la necesidad de descartar hacks ([Shopify](https://shopify.engineering/autoresearch)).

### P3 — Routing de modelos

No usar siempre el modelo mayor ni enrutar por precio/token. Databricks encontró que modelos más baratos por token podían consumir más y costar más por tarea, y propone task-level benchmarking ([Databricks: Price-per-task](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#price-per-task-vs-price-per-token)). El router futuro debe elegir por tipo/riesgo, calidad histórica, latencia, privacidad y presupuesto, conservando override humano y fallback.

## 5. Patrones y anti-patrones

### Patrones

- **Directo por defecto, orquestado por excepción.** Chalin declara direct execution para trabajo acotado y routing para trabajo amplio/riesgoso ([README de Chalin](https://github.com/0xkuze/pi-chalin#features)).
- **Progressive disclosure.** Brief corto, metadata de skills, referencias on-demand ([Agent Skills](https://agentskills.io/specification#progressive-disclosure)).
- **Fresh review.** Evita que el reviewer herede las mismas suposiciones del implementador ([pi-subagents](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#recommended-orchestration-pattern)).
- **Evidencia ejecutable antes que narrativa.** Tests/checks y diff pesan más que una explicación convincente ([Databricks](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#task-construction)).
- **Estado append-only + proyecciones pequeñas.** Facilita recovery, auditoría y contexto acotado ([pi-autoresearch](https://github.com/davebcn87/pi-autoresearch#the-loop), [pi-subagents observability](https://github.com/nicobailon/pi-subagents/blob/main/docs/observability.md#async-run-artifacts)).
- **Reversibilidad.** Branch/worktree/checkpoint y keep/revert antes de autonomía larga.

### Anti-patrones

- **Mega-prompt permanente:** desperdicia contexto y mezcla políticas irrelevantes.
- **Subagente para cada paso:** añade latencia, tokens, handoffs lossy y fallos distribuidos; las evals de Chalin muestran que routing puede ser más caro y que compararlo justamente es difícil ([reporte](https://github.com/0xkuze/pi-chalin/blob/main/docs/chalin-vs-gentle-eval-results.md#seguimiento-routed)).
- **Escritores paralelos en el mismo árbol:** conflictos y ownership ambiguo; usar un escritor o worktrees ([pi-subagents](https://github.com/nicobailon/pi-subagents/blob/main/docs/workflows.md#worktree-isolation)).
- **Memoria = transcript:** crecimiento, ruido, contradicción y filtración; guardar hechos curados con scope/provenance.
- **LLM judge como único gate:** puede premiar texto plausible sobre behavior; Databricks lo evitó ([fuente](https://www.databricks.com/blog/benchmarking-coding-agents-databricks-multi-million-line-codebase#task-construction)).
- **Project trust confundido con sandbox:** Pi advierte expresamente que trust solo gobierna carga de recursos ([Security](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/security.md#project-trust)).
- **Telemetry externa:** incluso metadata puede revelar actividad y contradice Zero Telemetry; mantener observabilidad local salvo que una nueva decisión sustituya ADR-0016.
- **Loop infinito sin stop/budget:** convierte retry en consumo y puede optimizar una métrica proxy destruyendo invariantes.

## 6. Próximos experimentos priorizados

### E0 — Conformance del runtime (P0)

Construir fixtures falsos de capability/run para validar: transitions, cancel propagado, timeout, truncation, crash/recovery, policy decision, single-writer y artifact integrity. **Éxito:** replay determinista produce el mismo estado terminal y ningún run cancelado sigue ejecutando children.

### E1 — Vertical slice “inspect → edit → verify” (P0/P1)

Una tarea pequeña, un inspector read-only, un worker y `CheckSpec` declarativo. Comparar ruta directa contra ruta con scout. Medir pass rate, tiempo, tokens, tool calls, intervención humana y tamaño de contexto. **Hipótesis:** ruta directa gana en tareas acotadas sin reducir calidad.

### E2 — Fresh reviewer gate (P1)

Corpus de 20–50 bugs/pequeñas features con tests retenidos. A/B: worker solo vs worker + reviewer fresh + una ronda de fix. El scorer principal son checks retenidos; registrar falsos positivos del reviewer. **Stop:** máximo una ronda inicialmente.

### E3 — Single writer vs dos writers (P1/P2)

Mismas tareas compuestas, tres brazos: secuencial, paralelo mismo tree, paralelo con worktrees. Medir conflictos, tiempo, tokens, fallos de integración y esfuerzo humano. **Resultado esperado:** prohibir mismo-tree por defecto; worktrees solo cuando la partición de archivos sea clara.

### E4 — Policy UX y sandbox (P1/P2)

Suite adversarial: lectura de secretos, escritura fuera de cwd, red no aprobada, comando destructivo, dependencia con install script y prompt injection en repositorio. Comparar host con policy hooks frente a backend aislado. **Éxito:** deny/approve correctos, credenciales mínimas y cero acciones fuera de scope. La amenaza es real porque Pi señala que extensiones y built-ins heredan permisos del proceso ([Security](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/security.md#no-built-in-sandbox)).

### E5 — Memoria candidata (P2)

Solo `/remember` explícito + candidatos de decisiones al cerrar; sin retrieval vectorial inicial. Evaluar precisión, contradicción, secretos, tasa de aceptación y tokens añadidos. **Gate para retrieval automático:** alta precisión y UI de provenance/corrección.

### E6 — Observabilidad local privacy-first (P2)

Registrar y mostrar localmente IDs, timestamps, modelo, tool name, status, tokens/coste/duración y hash de artefactos; no prompts/outputs ni exporters. Threat-model y prueba de cardinalidad. **Éxito:** depurar fallos y costes sin contenido sensible ni emisión de telemetry.

### E7 — Loop medible acotado (P3)

Optimizar una métrica no funcional en worktree efímero, máximo 10 iteraciones, checks obligatorios y aprobación de keep. Medir mejora contra baseline repetida, regresiones retenidas, coste y hacks rechazados. Solo ampliar autonomía si supera un run manual con igual presupuesto.

## 7. Riesgos y trade-offs

| Decisión | Beneficio | Coste/riesgo | Mitigación |
|---|---|---|---|
| Núcleo mínimo | Menor contexto, menos acoplamiento | Ecosistema inconsistente | Schemas, conformance suite y versionado semántico |
| Subagentes | Contexto aislado y revisión independiente | Latencia/coste/errores de handoff | Routing selectivo, artifacts, budgets, fresh/fork explícito |
| Verificación automática | Evidencia reproducible | Tests incompletos o proxy gaming | Hidden/behavioral tests, reviewer y diff humano |
| Memoria | Continuidad entre sesiones | Staleness, contradicción, secretos | Candidate/approved, provenance, TTL, redacción, borrado |
| Hooks de policy | UX local y flexible | No son aislamiento fuerte | Backend OS/VM/container para untrusted/AFK |
| Telemetry externa | Potencial mejora y diagnóstico | Contradice Zero Telemetry; privacidad y cardinalidad | Prohibida salvo una nueva decisión explícita |
| Worktrees | Escritores paralelos aislados | Merge e infraestructura | Solo particiones claras; ownership y manifests |
| Autoresearch | Explora trabajo que humanos no priorizan | Optimiza métrica equivocada, gasto abierto | Invariantes, noise, budgets, keep/revert y human gate |

## Decisión recomendada

Para Matty Core v0.1, **no construir todavía “el agente diario”**. Construir la base que permita múltiples agentes diarios sin rehacer seguridad, estado ni observabilidad:

1. capability manifests + policy preflight;
2. run/child-run state machine + event log;
3. budgets/cancel/timeouts/output bounds;
4. artifact/result/verification schemas;
5. single-writer, sin abstraer execution backends hasta tener un segundo adapter real;
6. package provenance y conformance tests.

Después, validar una única experiencia vertical de alta frecuencia: **cambio pequeño verificado y revisable**. Solo tras demostrar calidad/coste en un benchmark propio añadir fresh reviewers, memoria, paralelismo, routing y AFK loops. Esa secuencia conserva la ventaja esencial de Pi —pocas primitivas limpias y extensibilidad selectiva— mientras incorpora de su ecosistema lo que más falta para el trabajo diario: evidencia, reanudación, control, aislamiento y aprendizaje medible.
