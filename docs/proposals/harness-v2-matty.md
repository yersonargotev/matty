# Propuesta: adoptar Durable AgentHarness v2 en Matty

**Estado:** propuesta, sin implementación
**Fecha:** 2026-08-06
**Alcance:** integración de Matty con el Durable AgentHarness de Pi; no reimplementar el harness dentro de Matty.

## Resumen ejecutivo

Matty no debe crear un segundo sistema de sesiones, lanes, journals ni recuperación. El diseño `harness-v2.md` pertenece al runtime de Pi (`pi-agent-core`) y su implementación oficial todavía no expone públicamente todas las primitivas durables descritas.

La propuesta es:

1. Mantener el `Subagent Runtime` y los `Capability Contracts` como propiedad de Matty.
2. Introducir una pequeña interfaz/adaptador Matty para el harness oficial, usando únicamente exportaciones públicas.
3. Adoptar primero el `AgentHarness` y `Session` actuales sin prometer recuperación de operaciones.
4. Activar el modo durable sólo cuando Pi publique lanes, operation logs, storage y `resume()` con pruebas oficiales.
5. Mantener el runner por proceso como adaptación de seguridad para workers; no compartir archivos de sesión entre procesos hasta que Pi defina esa coordinación.

La investigación detallada está en [`docs/research/harness-v2-implementation.md`](../research/harness-v2-implementation.md).

## Situación actual

Matty actualmente:

- ejecuta cada hijo con un proceso Pi independiente mediante `spawn`;
- usa `--no-session` y recibe resultados JSONL;
- ofrece progreso, cancelación, scheduler, guards y Single Writer;
- aplica atomicidad y concurrencia a grupos de delegación;
- no tiene sesión durable propia, operation log, `resume()` ni recuperación crash-safe.

Pi `0.83.0` ya incluye `AgentHarness`, `Session` y backends de memoria/JSONL para el árbol conversacional. Sin embargo, esas APIs son de una sola operación activa: tienen fase, colas y pending writes, pero no exponen todavía lanes ni logs durables de operaciones. La existencia del documento v2 no equivale a que v2 esté disponible.

## Decisión arquitectónica propuesta

### Seam externo

El seam debe estar en la integración de Matty con Pi, no en cada role ni en el scheduler.

```text
Matty tools / roles / contracts
              |
       Matty Harness Adapter
              |
  AgentHarness + Session (Pi)
              |
 storage oficial de Pi
```

El adaptador será un módulo profundo: una interfaz pequeña ocultará selección de sesión, prompts, tools, eventos, cancelación y, posteriormente, recuperación. Los roles sólo conocerán el resultado estructurado del `Subagent Runtime`.

### Responsabilidades

**Pi / `pi-agent-core`:**

- árbol de conversación;
- lanes y serialización por lane;
- aceptación durable de operaciones;
- operation logs y reducers de recuperación;
- storage y compatibilidad JSONL v3;
- semantics de `resume`, abort, deferred writes y tool replay.

**Matty:**

- Capability Contracts y preflight;
- roles y superficies de tools;
- Inspection Guard y Worker Guard;
- Single Writer por repositorio;
- web capability y artifacts de investigación;
- límites de grupos, diagnostics y política de fallback;
- elección de qué operación Matty ejecuta a través del adaptador.

Matty no debe duplicar `operation_started`, `task_attempt`, `tool_started`, IDs provisionados ni reducers de recovery.

## Interfaz propuesta para Matty

La interfaz debe ser deliberadamente pequeña y estable:

```ts
interface MattyHarnessRuntime {
  run(
    request: MattyRunRequest,
    options?: { signal?: AbortSignal; onProgress?: ProgressHandler },
  ): Promise<MattyRunOutcome>;

  abort(operationId: string): Promise<void>;

  inspect(operationId?: string): Promise<MattyOperationSnapshot>;

  resume(operationId: string): Promise<MattyRunOutcome>;
}
```

Detalles importantes:

- En la primera fase, `resume` debe devolver explícitamente `unsupported`, no simular recuperación.
- `operationId`, `laneId` y estado de suspensión sólo se exponen cuando Pi los garantice públicamente.
- El contrato no debe incluir clases concretas de Pi ni `SessionStorage` interno.
- El adaptador debe traducir eventos de Pi a los diagnostics existentes de Matty, sin filtrar payloads sensibles.
- `run` debe preservar los outcomes actuales de Matty: éxito, fallo, cancelación y preflight bloqueado.

Los tipos exactos se definirán durante la implementación, después de confirmar la API pública upstream; no conviene fijarlos contra tipos internos de `main`.

## Plan por fases

### Fase 0 — Preparación y gate upstream

**Objetivo:** dejar Matty listo sin prometer v2.

1. Añadir el adaptador detrás del runtime actual, sin cambiar el comportamiento observable.
2. Encapsular en un solo módulo la creación del child runner/harness.
3. Separar explícitamente:
   - `preflight` de Matty;
   - ejecución del harness;
   - supervisión del proceso;
   - normalización de outcomes.
4. Añadir un capability/status field como `durableHarness: unavailable|transcript-only|durable`.
5. Documentar que un crash durante una delegación puede perder la operación activa.

**Criterio de salida:** todos los tests existentes siguen pasando y no se importa ninguna API interna de Pi.

### Fase 1 — Integración con el harness oficial actual

**Objetivo:** usar el código oficial donde ya aporta valor, sin afirmar durabilidad completa.

1. Consumir sólo exportaciones públicas de `AgentHarness`, `Session` y el backend configurado.
2. Usar `PendingSessionWrite`/save points oficiales para transcript y contexto.
3. Mantener el proceso hijo independiente para aislamiento y guards.
4. Evitar compartir la misma sesión física entre procesos hasta que el modelo de single-writer de Pi lo soporte.
5. Mantener `--no-session` únicamente donde sea necesario por aislamiento; para una ejecución harness-backed usar un directorio de sesión explícito y aislado por operación.

**Criterio de salida:** Matty puede restaurar el transcript/leaf, pero el estado se reporta como `transcript-only`; no existe `resume` de una operación incompleta.

### Fase 2 — Adaptación durable

**Precondición obligatoria:** Pi publica y prueba una API que cubra, como mínimo:

- lanes y leaf por lane;
- operación abierta única por lane;
- operation log persistente;
- aceptación antes del efecto;
- `resume()` y `abort()` después de restart;
- recovery reducer-based;
- tool replay seguro o resultado sintético;
- storage de single writer;
- apertura/restauración de sesiones JSONL v3 en idle.

Implementación:

1. Crear un `PiDurableHarnessAdapter` que implemente `MattyHarnessRuntime`.
2. Mapear cada delegación aceptada a una operación durable.
3. Asociar cada role/delegación a una lane estable sólo si Pi garantiza la identidad y su lifecycle.
4. Persistir en Pi el estado de ejecución, no en entries de conversación creadas por Matty.
5. Hacer que el scheduler reanude operaciones suspendidas sin crear duplicados.
6. Mantener Single Writer de Matty como política de repositorio; es distinta del single writer de una sesión Pi.

**Criterio de salida:** matar el proceso en cada punto de intención/resultado y comprobar que `resume()` termina exactamente una operación, sin perder entradas ni duplicar tool effects.

### Fase 3 — Activación certificada

1. Ejecutar la suite de aceptación sobre el artefacto empaquetado.
2. Probar Pi version/target exactos, como exige la política de certificación de Matty.
3. Activar `durableHarness: durable` sólo para la combinación certificada.
4. Mantener fallback explícito a `transcript-only` cuando la API o el host no estén certificados.
5. Añadir un ADR cuando la integración sea aceptada.

## Matriz de comportamiento

| Situación | Antes de v2 upstream | Con adapter durable |
|---|---|---|
| Pi termina normalmente | outcome actual | operación `completed` |
| abort durante ejecución | cancelación del child | `abort` durable + reconciliación |
| crash antes de ejecutar | operación perdida o fallo reportado | intent record + `resume` |
| crash durante tool | fallo/cancelación | replay seguro o resultado sintético |
| restart con operación abierta | no soportado | lane `suspended` |
| nueva operación en lane ocupada | scheduler local | rechazo durable `busy` |
| transcript existente | restaurable según backend | compatible con JSONL v3 en idle |

## Alternativas rechazadas

### Implementar un journal propio en Matty

Rechazada. Duplica ordering, IDs provisionados, compaction, replay de tools y reglas de recovery de Pi. También crea un formato de sesión incompatible y dos single-writer policies difíciles de coordinar.

### Convertir el scheduler actual en un durable harness

Rechazada. `delegation-scheduler.ts` resuelve cardinalidad, atomicidad y concurrencia de tareas, pero no controla el contexto conversacional, storage, provider requests ni el punto exacto de efectos.

### Compartir la sesión padre con todos los child processes ahora

Rechazada. El diseño v2 exige un único writer por sesión; los procesos actuales de Matty son independientes y no tienen coordinación de storage. Compartir el archivo antes del soporte oficial puede corromper el árbol o producir operaciones intercaladas no recuperables.

### Forkear Pi dentro de Matty

Rechazada salvo bloqueo upstream excepcional. Aumentaría el coste de mantenimiento y rompería la regla de Matty de apoyarse en interfaces públicas certificadas.

## Riesgos y mitigaciones

- **API upstream cambia:** encapsularla en un único adapter y fijar la versión exacta certificada.
- **Falsa sensación de durabilidad:** exponer el estado `transcript-only`/`durable` en diagnostics y no ofrecer `resume` falso.
- **Duplicación de efectos al reanudar:** delegar tool replay a Pi y conservar el ID de operación; workers peligrosos deben declararse no replayables.
- **Confusión entre writers:** documentar por separado el writer de sesión Pi y el writer de repositorio Matty.
- **Coste de migración de child runner:** preservar `DelegatedTaskRunner` como interfaz interna y cambiar sólo su adapter.
- **Compatibilidad v3 incompleta:** no migrar formatos propios; exigir la garantía oficial de abrir y restaurar idle.

## Tests requeridos

### Unitarios

- traducción de eventos/outcomes Pi a diagnostics Matty;
- capability states y fallback explícito;
- rechazo de APIs durable ausentes;
- no duplicación de `operationId`;
- cancelación antes y después de spawn.

### Integración

- sesión de memoria y JSONL;
- restart con operación idle;
- restart con operación abierta;
- abort durante tool;
- steering/follow-up/deferred write en checkpoint;
- dos lanes concurrentes con un único writer;
- tool replay seguro y no seguro;
- compatibilidad de sesión JSONL v3.

### Acceptance

- ejecutar contra el artefacto npm empaquetado, no sólo TypeScript fuente;
- probar el Pi certificado y el target certificado;
- verificar que un fallo de storage deja Matty diagnosable y rechaza nuevos efectos;
- comprobar que no hay red/telemetría adicional fuera de una acción dirigida por el usuario.

## Decisión solicitada

Aceptar esta propuesta como roadmap de integración y **no implementar Durable AgentHarness dentro de Matty ahora**. El siguiente trabajo concreto es la Fase 0: crear el seam/adaptador y los diagnostics de capability, mientras se sigue upstream hasta que la API durable y sus pruebas estén publicadas.
