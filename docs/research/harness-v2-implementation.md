# Durable AgentHarness v2: investigación de implementación

## Resumen ejecutivo

La fuente primaria separa claramente **diseño v2** de implementación actual. `packages/agent/docs/harness-v2.md` especifica un harness durable con lanes/refs, logs de operaciones, recuperación tras crash, checkpoints y operaciones `resume`; pero `AgentHarness` publicado en `main` sigue siendo el harness de una sola operación/fase, con persistencia de transcript y escrituras pendientes, no el v2 durable completo. En `0.83.0`, el changelog no registra cambios bajo esa versión, por lo que no hay evidencia primaria de que v2 durable esté implementado allí.

Recomendación para Matty: adoptar las APIs existentes de `@earendil-works/pi-agent-core` (AgentHarness, Session y backends de sesión) como integración, y tratar durable-v2 como una dependencia/roadmap upstream. No crear un segundo sistema de operaciones, colas o recuperación dentro de Matty.

## Fuentes primarias

- Diseño v2: [`packages/agent/docs/harness-v2.md`](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md).
- Diseño durable intermedio: [`packages/agent/docs/durable-harness.md`](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/durable-harness.md).
- Estado y TODO de la implementación: [`packages/agent/docs/agent-harness.md`](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/agent-harness.md).
- Tipos y contratos actuales: [`packages/agent/src/harness/types.ts`](https://github.com/earendil-works/pi/blob/main/packages/agent/src/harness/types.ts).
- Implementación actual: [`packages/agent/src/harness/agent-harness.ts`](https://github.com/earendil-works/pi/blob/main/packages/agent/src/harness/agent-harness.ts).
- Exportaciones públicas: [`packages/agent/src/index.ts`](https://github.com/earendil-works/pi/blob/main/packages/agent/src/index.ts).
- Historial de releases: [`packages/agent/CHANGELOG.md`](https://github.com/earendil-works/pi/blob/main/packages/agent/CHANGELOG.md).

## Qué define el diseño Durable v2

`harness-v2.md` define:

- Un `Session` como log append-only con cuatro partes: árbol conversacional, lanes, logs de operación por lane y hechos globales. Cada lane tiene leaf, colas y como máximo una operación abierta; las lanes pueden ejecutarse en paralelo bajo un único writer ([harness-v2.md, “What a session is”, “Lanes”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).
- Operaciones durables `run`, `compaction` y `navigation`; aceptación durable antes de ejecutar y outcomes `completed`, `failed`, `aborted` o `declined` ([“Operations”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).
- Regla de durabilidad: registrar una intención antes del efecto, con IDs provisionados, y escribir después el resultado con esos IDs. El catálogo propuesto incluye `operation_started`, `abort_requested`, `operation_finished`, `task_attempt`, `tool_started`, `queue_enqueued` y `write_deferred` ([“The durability rule”, “Record catalog”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).
- Checkpoints que aplican deferred writes, consumen steering y compactan antes de la siguiente request. Las colas y escrituras se aceptan durablemente; las entradas de árbol aparecen en el punto de consumo/aplicación ([“Queues and deferred writes”, “Checkpoints”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).
- Recuperación bounded por operación/lane, con estado `suspended`; `resume()` continúa una operación abierta, no inicia otra. Streams parciales no se reanudan. Tool calls incompletas sólo se reejecutan si la intención y la declaración actual indican replay seguro; si no, se genera resultado sintético ([“Recovery”, “Tool execution crash sites”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).
- Deferred provider requests: un mensaje assistant con `stopReason: deferred` y handle persistido puede redimirse en vez de pagar una request nueva ([“Deferred provider request”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).

## APIs/tipos que existen hoy en main

Las exportaciones de `packages/agent/src/index.ts` incluyen `AgentHarness`, tipos del harness, `Session`, `SessionStorage`, `JsonlSessionRepo/Storage`, `MemorySessionRepo/Storage`, compaction, tools y `ExecutionEnv`; no exportan una API pública de lanes, operation logs, `resume()` o `HarnessSession` v2.

El `AgentHarness` actual recibe `AgentHarnessOptions` con `session`, `models`, `model`, `tools`, `resources`, `systemPrompt`, `streamOptions`, `retry`, `thinkingLevel`, `activeToolNames`, `steeringMode` y `followUpMode`. Los tipos centrales son:

- `AgentHarness<TContext, TSkill, TPromptTemplate, TTool>` y `AgentHarnessTool`, con `toolContext` de aplicación resuelto por snapshot.
- `AgentHarnessPhase = "idle" | "turn" | "compaction" | "branch_summary" | "retry"`.
- Eventos tipados: `queue_update`, `save_point`, `abort`, `settled`, `before_agent_start`, `context`, hooks de provider, `tool_call`, `tool_result`, compaction/tree, retry, model/thinking/resources/tools updates.
- `SessionTreeEntry` y entradas persistibles existentes: `message`, `thinking_level_change`, `model_change`, `active_tools_change`, `compaction`, `branch_summary`, `custom`, `custom_message`, `label`, `session_info`, `leaf`.
- `SessionStorage`: `getMetadata`, `getLeafId`, `setLeafId`, `createEntryId`, `appendEntry`, `getEntry`, `findEntries`, labels/stats, `getPathToRootOrCompaction` y cursor `getEntries`.
- `PendingSessionWrite` es una forma de entrada sin `id`, `parentId` ni `timestamp`; actualmente las escrituras durante una operación se encolan y se flush-ean en save points, settlement y cleanup.

La implementación fuente confirma que el harness es una clase con un único `phase`, un único `session`, colas `steerQueue`, `followUpQueue`, `nextTurnQueue`, y operaciones estructurales rechazadas si no está idle (`prompt`, `skill`, `promptFromTemplate`, `compact`, `navigateTree`) ([agent-harness.ts](https://github.com/earendil-works/pi/blob/main/packages/agent/src/harness/agent-harness.ts)). Esto no equivale todavía al modelo de múltiples lanes/logs de v2.

## Almacenamiento y recuperación: realidad versus diseño

**Existe hoy:** sesión durable de árbol/transcript en memoria y JSONL; `leaf` se persiste como entrada durable; cambios de modelo, thinking y active tools son entradas de sesión; metadata JSONL opcional; `Session.buildContextEntries()`/`buildContext()` y proyectores/transforms de entradas custom. Las APIs de almacenamiento siguen siendo de árbol/sesión, no de operation log.

**Está diseñado pero no implementado:** `durable-harness.md` describe una meta “semi-durable”: la sesión como fuente de verdad append-only, el host recrea modelos/tools/extensions/resources/auth/hooks, y la recuperación empieza en límites durables. Su TODO explícito propone añadir entradas para colas, pending writes, operaciones, turns, provider requests y tool calls, reducir el log y marcar turns incompletas como interrumpidas por defecto ([durable-harness.md, “Minimum viable spike”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/durable-harness.md)).

El TODO de `agent-harness.md` es concluyente: “Spike semi-durable harness/session recovery” tiene estado **Planned**; quedan por definir las entradas, requisitos de resume, política conservadora y prototipo de recuperación reducer-based. También marca como no implementados auto-compaction y retry decision points en `AgentHarness`, y el generic hook/event extension mechanism ([“Implementation todo”](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/agent-harness.md)). Sí están marcados como hechos el loop directo, snapshots de save point, pending-write flushing, tool registry y stream/provider configuration.

## 0.83.0 y main

- El changelog coloca `## [0.83.0] - 2026-07-29` sin subsecciones Added/Changed/Fixed; por tanto no documenta cambios de AgentHarness en esa release ([CHANGELOG.md](https://github.com/earendil-works/pi/blob/main/packages/agent/CHANGELOG.md)).
- Los cambios documentados inmediatamente anteriores son 0.82.0 (breaking: `ExecutionEnv` y tools context-aware) y 0.81.x (retry policy/usage metadata), no durable-v2 recovery.
- `main` contiene la especificación v2 completa, pero la implementación visible y el TODO la describen como dirección actual con partes planificadas. En particular, la documentación de implementación dice que el `HarnessSession` facade, `getPendingWrites()` público y el generic hooks facade están planificados, no implementados.
- No debe inferirse que la existencia de `harness-v2.md` significa que la API v2 esté disponible: el propio documento de lifecycle y el TODO distinguen comportamiento implementado, provisional y planned.

## Compatibilidad v3

La política explícita de `harness-v2.md` es: “Old coding-agent v3 JSONL sessions must open and restore idle. This is the only backward-compatibility requirement.” El diseño no promete compatibilidad de APIs ni de otros formatos; permite romper formatos/APIs internas de `packages/agent/src/harness` y `packages/storage/sqlite-node`, sin migraciones, versionado de schema ni conversiones adicionales ([compatibility policy](https://github.com/earendil-works/pi/blob/main/packages/agent/docs/harness-v2.md)).

La recomendación de integración es, por tanto, conservar los JSONL v3 de Matty y abrirlos mediante las sesiones oficiales; no depender de tipos internos del futuro log v2 ni escribir un migrador paralelo. La compatibilidad garantizada es abrir/restaurar idle, no reanudar una operación antigua v3.

## Opciones para Matty

1. **Adopción inmediata (recomendada):** usar `AgentHarness` actual con `Session`/`JsonlSessionRepo` o `MemorySessionRepo`, tools context-aware y eventos existentes. Encapsularlo detrás de una pequeña interfaz de Matty que sólo use APIs exportadas. No implementar durable recovery; documentar que un crash puede dejar un turno sin reanudación.
2. **Preparación sin fork:** mantener la capa de integración libre de `SessionStorage` concreto salvo construcción/configuración. Usar `Session` para transcript, `setLeafId`, custom entries y `PendingSessionWrite` sólo donde la API actual lo exponga. Evitar asumir `HarnessSession`, lanes, operation records o `resume()` hasta que upstream los exporte.
3. **Seguimiento upstream:** tratar `harness-v2.md` como contrato de diseño y adoptar cuando existan exportaciones/types/tests de lanes, operation log, recovery y storage v4. Validar primero con los tests oficiales y el requisito v3 “open and restore idle”.
4. **No recomendado:** añadir en Matty su propio journal de operaciones, IDs provisionados, reducers de recuperación o semántica paralela de lanes. Duplicaría exactamente la futura responsabilidad oficial y produciría divergencia de ordering, compaction, tool replay y formato de sesión.

### Criterio de go/no-go

Matty puede integrar la API actual hoy para ejecución y transcript. Debe declarar **no disponible** durable-v2 hasta que el repositorio oficial cambie el estado de “Spike semi-durable harness/session recovery: Planned” y exponga públicamente las primitivas de aceptación durable, operation log, recovery/resume y storage correspondientes.
