# Propuesta: gestión nativa de Delegations

**Estado:** Incrementos 1 y 2 implementados; aceptación PTY empaquetada y validación subjetiva en terminal real completadas

**Fecha:** 2026-08-15

**Alcance:** observación y control de Delegations sobre el Subagent Runtime existente; no sustituirlo por `pi-subagents`.

## Decisión arquitectónica

Matty conservará sus procesos Pi aislados y construirá la gestión de Delegations de forma nativa. `pi-subagents` sirve como referencia de producto, pero no será backend ni dependencia de Matty Core. Una interoperabilidad opcional puede evaluarse posteriormente sin ejecutar los cinco Matty Roles.

Esta decisión preserva Capability Contracts, preflight, atomicidad de grupos, cancelación SIGTERM→SIGKILL, Bounded Concurrency y Single Writer. La evidencia está en [`docs/research/pi-subagents-evaluation.md`](../research/pi-subagents-evaluation.md) y la decisión se registra en [ADR-0023](../adr/0023-manage-delegations-within-matty.md).

## Modelo acordado

- **Delegation:** una invocación de `subagent`, raíz de observación, control y resultado.
- **Delegation ID:** UUID opaco asignado antes del preflight; la UI muestra una forma corta `D-<8 hex>` cuya unicidad se verifica dentro del registro.
- **Delegated Task:** una unidad de trabajo con Matty Role dentro de la Delegation.
- **Child Execution:** el proceso Pi que intenta completar una tarea; PID y `runId` son diagnósticos.
- **Delegation Registry:** registro en memoria durante la sesión, con todas las Delegations activas y las 50 terminales más recientes.
- **Delegation Console:** vista navegable del registro mediante `/matty delegations`.

Las acciones se aplican a la Delegation. Las tareas se pueden inspeccionar, pero el primer incremento no permite cancelarlas individualmente porque eso sería ambiguo para grupos `required` atómicos.

## Incremento 1 — Gestión esencial

### Capacidades

1. Crear una entrada de registro al aceptar cada Delegation.
2. Mostrar una tarjeta inline con progreso resumido.
3. Exponer `/matty delegations` como Delegation Console navegable.
4. Mostrar por Delegation y tarea:
   - rol;
   - posición en cola;
   - estado;
   - tiempos y duración;
   - resultado terminal resumido.
5. Permitir cancelar una Delegation completa después de confirmación explícita.
6. Indicar en la confirmación cuántas tareas están activas y en cola.
7. Conservar todas las activas y las 50 terminales más recientes; desalojar primero la terminal más antigua.
8. Registrar como `blocked` las Delegations rechazadas durante preflight, con una razón redactada.

### Estados de Delegation

```text
queued → running → cancelling → cancelled
```

Estados terminales alternativos:

```text
blocked | succeeded | partial | failed
```

El primer estado terminal gana. Cancelar una Delegation terminal devuelve `already-finished`; repetir una petición mientras está `cancelling` es un no-op; la cancelación nunca sobrescribe un resultado terminal.

### Lifecycle del registro

El Delegation Registry pertenece a la instancia actual de Matty y no se reconstruye desde la conversación:

- `/new` comienza con un registro vacío;
- `/resume` comienza con un registro vacío;
- `/reload` comienza con un registro vacío aunque continúe la conversación;
- `session_shutdown` cancela defensivamente cualquier controller restante y libera referencias de UI.

La Delegation Console debe explicar este límite. Reconstrucción e historial persistente quedan fuera del Incremento 1.

### Interacción

En TUI, `/matty delegations` abre una vista custom con este mapa mínimo:

- `↑/↓`: seleccionar Delegation;
- `Enter`: expandir sus Delegated Tasks;
- `c`: solicitar cancelación de la Delegation completa;
- `Esc` o `q`: cerrar.

La confirmación de cancelación muestra cuántas tareas están activas y en cola. No se registra un shortcut global en este incremento. La tarjeta inline es informativa y no contiene controles porque los tool renderers no tienen foco fiable.

La consola agrupa y ordena:

1. `running` y `cancelling`;
2. `queued`;
3. terminales de más reciente a más antigua.

La selección permanece ligada al Delegation ID aunque una transición cambie la posición de la fila. El estado siempre aparece como texto completo además de icono o color. La vista compacta muestra rol, conteo de tareas y tiempo transcurrido o duración final; los timestamps exactos aparecen sólo al expandir.

En modos `print`, `json` y RPC, el comando devuelve un snapshot determinista textual o JSON y no ofrece cancelación manual sin confirmación interactiva. La propagación normal de `AbortSignal` permanece disponible.

### Privacidad y datos

El registro y la UI guardan sólo metadatos de lifecycle. El primer incremento no incorpora prompts, argumentos de herramientas, comandos, respuestas del modelo ni transcripts del hijo. El resultado completo que Pi ya conserva en la conversación no se duplica en el registro.

### Gate de salida

La implementación productiva conserva la frontera de procesos, cubre estados, transiciones, retención, redacción y cancelación atómica, y forma parte de `npm run check`. La aceptación automatizada empaqueta el candidato y ejercita la TUI sobre Pi `0.84.2` en `darwin/arm64`.

El gate subjetivo se validó por separado en un terminal real: el operador confirmó ausencia de corrupción visual, controles claros, cursor y foco correctos al cerrar con `q` y `Esc`, y entrada inmediata de `/matty status`. La evidencia conserva artefacto, SHA-256, versión de Pi, target, fecha y resultado en [`docs/acceptance/delegation-tui-manual.md`](../acceptance/delegation-tui-manual.md). El procedimiento reproducible permanece en [`scripts/acceptance/manual-delegation-tui.sh`](../../scripts/acceptance/manual-delegation-tui.sh).

## Incremento 2 — Fleet observable (implementado)

La implementación productiva incluye:

1. widget compacto de actividad global;
2. timeline navegable por Delegated Task;
3. resúmenes cerrados y redactados de herramientas y actividad;
4. inspección de concurrencia y cola;
5. ausencia deliberada de controles que contradigan atomicidad o autoridad de roles.

La redacción usa un allowlist cerrado y las vistas comparten snapshots deterministas. La interacción, truncación, lifecycle del widget y modos headless/print están cubiertos en los seams públicos de aplicación y registro Pi.

## Incremento 3 — Child Sessions interactivas y persistentes

**Estado:** diseño acordado; implementación pendiente.

El Incremento 3 reemplaza el child runtime de una sola respuesta con la arquitectura final: una Child Session Pi bidireccional por Delegated Task, ejecutada exclusivamente mediante procesos aislados `pi --mode rpc`. No conserva el renderer/runtime anterior como fallback, no usa `AgentSession` in-process y no añade compatibilidad ni migraciones para shapes obsoletos. La decisión se registra en [ADR-0024](../adr/0024-manage-interactive-child-sessions-over-isolated-pi-rpc.md).

### Identidad, ejecución y resultados

- Cada Delegated Task recibe un UUID opaco y una etiqueta corta `T-<hex>`, estable entre sus Child Executions.
- La Delegated Task es la unidad visible; PID y `runId` identifican únicamente el intento actual.
- Sólo existen cuatro Child Executions concurrentes. Un proceso RPC permanece abierto mientras procesa su turn y mensajes ya encolados, y termina al alcanzar `agent_settled`; no existe una flota idle ni un scheduler secundario.
- Una interacción aceptada usa el Child Execution abierto o, si éste ya terminó antes del freeze, crea otro sobre la misma Child Session. Durante `freezing`, nuevas interacciones reciben `delegation-closing`; después del freeze se ofrecen como Continuation.
- Cada respuesta settled vuelve a pasar por el parser del Matty Role. Sólo una respuesta válida reemplaza el Candidate Result; una inválida permanece en el Child Transcript con diagnóstico cerrado.
- El cierre espera interacciones ya aceptadas, congela el resultado una sola vez y produce un único tool result. Trabajo posterior crea una Continuation enlazada, con Delegated Task, Delegation, preflight y resultado propios.

Estados de Child Session:

```text
starting → working → settled
                  ↘ waiting-for-input
                  ↘ waiting-for-capability
working → aborting → aborted
any open state → interrupted | failed | protocol-failed
```

La Child Session persiste como conversación aunque no exista un proceso hijo. Crash, reload o restart nunca intentan reattach al mismo proceso; Continue crea una nueva Delegated Task y Child Execution después de preflight.

### Interacción y autoridad

- La consola permite transcript, búsqueda, filtros, steering, follow-up, abort y Continue.
- `Steer` y `Follow up` son acciones explícitas. `Esc` cancela composición o vuelve un nivel; abort requiere confirmación separada.
- Toda interacción conserva rol, herramientas, guards, working tree, web, Review Scope Contract y límites de modelo. Una ampliación de capacidades exige otra Delegated Task con preflight propio.
- Solicitudes `extension_ui_request` no toman el foco: se registran, marcan `waiting-for-input` y se responden desde la consola. Se respeta el timeout del hijo o se aplica un default fijo de cinco minutos; la consola puede ampliarlo explícitamente.
- Al expirar, Matty responde cancelación. Si el hijo no puede producir Candidate Result, la tarea falla mediante su contrato normal.
- Abortar una tarea `required` aplica la cancelación atómica a hermanas abiertas. En una Delegation `optional`, sólo se cancela la tarea elegida. Una tarea cancelada nunca se presenta como failed.
- Un worker libera Single Writer al producir Candidate Result. Otra interacción debe readquirirlo y espera como `waiting-for-capability` mientras no esté disponible.
- Continue repite preflight, advierte drift de Git HEAD/working tree, bloquea Reviewer si sus commits ya no existen y nunca restaura autoridad desde metadata antigua.

### Protocolo y backpressure

- El cliente RPC usa framing JSONL estricto, límites por frame y buffer, request IDs correlacionados y comandos allowlisted.
- Un frame inválido termina sólo ese Child Execution como `protocol-failed`; stderr se limita y se reduce a diagnóstico cerrado. No existe fallback al modo print.
- Eventos desconocidos no se ejecutan. ANSI y contenido de terminal se neutralizan antes de renderizarse.
- Los deltas se ensamblan por `contentIndex`; `message_end` es la autoridad final. Updates acumulativos reemplazan estado en vez de duplicarlo.
- La lectura de stdout nunca espera a la TUI. Los rerenders se coalescen y la memoria conserva sólo el parcial necesario; el store persiste entradas terminales, no frames visuales.

### Child Session Store y privacidad

El store mínimo vive fuera del repositorio y de `/resume`:

```text
<agent-dir>/matty/child-sessions/
└── <delegated-task-id>/
    ├── manifest.json
    └── session.jsonl
```

- No hay índice global: Matty descubre sesiones escaneando manifests.
- El JSONL de Pi es la fuente del Child Transcript y el manifest usa únicamente el schema actual. Una versión incompatible falla explícitamente; no se migra ni se abre read-only.
- La persistencia está habilitada por defecto. Cada Delegation sólo puede elegir `persistent` o `ephemeral`.
- La retención fija es siete días o cien sesiones, con límite global de 1 GiB. Se desalojan primero sesiones terminales antiguas y nunca una activa.
- Una sesión ephemeral conserva transcript en memoria durante la sesión padre y no puede continuar después de reload o restart.
- El transcript incluye mensajes retenidos, reasoning, tool calls, argumentos, resultados, usage, interacción y errores, pero no promete recuperar contenido truncado antes de entrar a Pi.
- No hay redacción heurística. El store usa permisos restrictivos y nunca incluye transcripts en snapshots seguros, tool results del padre, logs, status, doctor, crash output o telemetría.

### Presentación y API

La Delegation Console conserva la jerarquía `Delegations → Delegated Tasks → Child Session`.

- La tarjeta inline muestra una sección viva y compacta por tarea; el widget fleet resume actividad y cola por tarea.
- La vista Child Session ofrece transcript con scroll, búsqueda y filtros, costo, usage, contexto y controles. Texto user/assistant aparece expandido; reasoning, argumentos y resultados aparecen colapsados.
- `/matty task T-<id>` abre directamente una tarea. La UI publica hints para `Enter`, `Tab`, flechas/`j`/`k`, `/`, `f`, `s`, `u`, `a`, `Esc` y `q` usando las capacidades actuales de keybindings Pi.
- Headless mantiene `/matty delegations --json` con el nuevo shape breaking y añade lectura explícita de tarea/transcript más steer/follow-up por ID exacto.
- Abort, delete y Continue son TUI-only en este incremento. Ningún control se expone como tool al modelo padre.
- La consola muestra tokens, costo, turns y contexto, sin introducir presupuestos configurables.

### Entrega

La arquitectura final se entrega en tres incrementos end-to-end, cada uno integrado en `npm run check` y aceptación empaquetada:

1. Delegated Task ID, controlador RPC final, stream, transcript en memoria y resultados de grupo;
2. tarjetas por tarea y visor TUI final con búsqueda, steer, follow-up y abort;
3. manifest + JSONL, modo ephemeral, retención fija y Continue.

Cada incremento mantiene el producto funcionando, pero ninguno introduce un runtime temporal que el siguiente deba reemplazar. No habrá feature flag permanente ni fallback al child runtime obsoleto.

## Fuera de alcance

- reemplazar Child Executions con `AgentSession` in-process;
- usar `pi-subagents` para ejecutar Matty Roles o copiar su manager;
- ampliar herramientas, rol o autoridad mediante steering, follow-up o Continuation;
- retry fresh, navegación/fork de ramas, recovery automático o reattach durable al mismo proceso;
- presupuestos configurables, exportación especial o gestión de attachments propia;
- mutaciones headless como abort, delete o Continue;
- compatibilidad, fallback o migración para el child runtime y schemas reemplazados;
- reescribir resultados terminales o tool results de la conversación padre;
- guardar Child Transcripts dentro del proyecto o mezclarlos con `/resume`;
- incluir Child Transcripts en snapshots seguros o exponer controles como tools al modelo padre;
- prometer recuperación de output que Pi o una herramienta ya truncó.

## Estrategia de entrega

Antes de la implementación definitiva se hizo un spike desechable que validó únicamente:

- abrir `/matty delegations` mientras `subagent` ejecuta;
- actualizar la vista sin corrupción visual;
- seleccionar y confirmar cancelación;
- cerrar la consola y devolver correctamente el foco.

El código del spike no se convierte directamente en arquitectura de producción. Superado el spike, el Incremento 1 se implementa con tests sobre seams explícitos.

El issue tracker separó las fronteras ya implementadas:

1. #56 conserva el spike desechable validado;
2. #57 y #58 entregaron observación y cancelación como gestión esencial;
3. #59 entregó el Fleet observable;
4. #60 entregó actividad redactada.

El Incremento 3 queda dividido en los tres incrementos finales definidos arriba. Cada uno requiere issue/spec propio, tests de protocolo y presentación, aceptación empaquetada y validación manual de foco, navegación y ausencia de corrupción visual.

## Resultado del spike de interacción (#56)

**Veredicto:** viable con Pi `0.83.0` en `darwin/arm64`. El spike fue instrumentación desechable en el adaptador Pi, se ejercitó en TUI y se eliminó por completo antes de conservar estos resultados.

### Evidencia observada

- Una Delegation `required` de cinco Delegated Tasks abrió `/matty delegations` mientras cuatro Child Executions estaban activos y una Delegated Task permanecía en cola.
- La vista se actualizó en vivo de `running` (`active:4`, `queued:1`) a `cancelling` y `cancelled` sin contenido duplicado o corrupción visible. La selección y la expansión permanecieron ligadas al mismo `D-<8 hex>` durante las transiciones.
- Una ejecución adicional inició dos Delegations raíz en paralelo. La seleccionada pasó de la segunda fila a la primera cuando la otra Delegation cambió de `running` a `succeeded`; el marcador permaneció en el mismo Delegation ID, demostrando que el reordenamiento no retargetea la selección.
- `c` abrió una confirmación interna. `Esc` la rechazó sin cancelar; una segunda solicitud confirmada con `y` alcanzó la ruta activa: los cuatro Child Executions emitieron progreso `terminating` con `SIGTERM`, y la Delegated Task en cola terminó cancelada en fase `before-spawn`.
- `q` cerró la consola; `/matty status` se ejecutó inmediatamente después, demostrando que el editor recuperó el foco.
- Una segunda ejecución cerró con `Esc` mientras una Delegation seguía activa; `/matty status` volvió a ejecutarse inmediatamente y reportó un Child Execution activo, confirmando restauración de foco sin esperar al tool result.
- Las dos ejecuciones usaron el binario local exacto `@earendil-works/pi-coding-agent@0.83.0`. El modelo activo del ensayo fue `openai-codex/gpt-5.4`; la validación demuestra compatibilidad del host TUI, no certifica otro Reference Model Path.

### Hallazgos y límites

- Los comandos de extensión se despachan durante streaming, por lo que no es necesario esperar a que termine `subagent`.
- `ctx.ui.custom()` no-overlay restaura el editor y su foco al invocar `done()`. Es una base más conservadora para Incremento 1 que el overlay experimental.
- La confirmación debe ser un estado dentro del componente. Abrir `ctx.ui.confirm()` desde una vista custom no-overlay sustituye el contenedor y puede restaurar el editor en lugar de la consola.
- Cada cambio de snapshot debe invalidar la presentación y llamar `tui.requestRender()`; las líneas deben truncarse al ancho recibido.
- La selección debe almacenar Delegation ID, nunca índice de fila. El ensayo con dos Delegations confirmó identidad estable durante reordenamiento; el Incremento 1 debe conservar esta conducta en sus tests de presentación.
- La validación fue sobre una extensión fuente instrumentada, no sobre el artefacto npm empaquetado. El gate del Incremento 1 sigue requiriendo aceptación packed y revisión manual.
- La ausencia de corrupción se evaluó en la TUI y en el stream ANSI capturado; no existe todavía un emulador de terminal automatizado que compare frames visuales.

### Seam recomendado para producción

No promover los maps, controllers ni el componente del spike. Crear un módulo profundo y session-scoped **Delegation Registry** que oculte identidad, transiciones, retención, resolución de carreras y controllers detrás de una interfaz pequeña equivalente a:

```text
accept(declaration) -> Delegation ID
snapshot() -> lifecycle metadata only
subscribe(listener) -> unsubscribe
record(id, lifecycle event)
cancel(id) -> cancelling | already-cancelling | already-finished
finish(id, result)
shutdown()
```

El adaptador Pi debe limitarse a conectar el tool público y lifecycle del host con ese registro. Una presentación pura conserva `selectedId`/expansión y produce filas ordenadas; una capa TUI delgada maneja teclado, render e invalidación. Los modos no interactivos consumen el mismo snapshot determinista sin exponer controllers ni ofrecer cancelación sin confirmación.

## Aceptación productiva empaquetada

`scripts/acceptance/t10-delegation-tui.mjs`, integrado en `npm run check`, exige exactamente Pi `0.84.2`, `darwin/arm64` y `/usr/bin/expect`; una precondición ausente falla con un diagnóstico de target certificado. En un HOME y proyecto aislados instala el artefacto npm empaquetado y una extensión fixture determinista junto a la extensión productiva. El PTY:

- inicia una Delegation viva de cinco tareas con cuatro activas y una en cola;
- abre `/matty delegations` durante streaming y observa texto de estado y conteos;
- rechaza una confirmación, vuelve a solicitarla y confirma la cancelación completa;
- observa rerender `cancelling` y `cancelled`;
- cierra con `q` y ejecuta `/matty status` para probar restauración de foco e input;
- usa matches por fase y adjunta el transcript PTY al diagnóstico de fallo.

Esta aceptación automatiza comportamiento observable, no la apreciación humana de corrupción visual o cursor en un emulador de terminal real. Para esa revisión, `scripts/acceptance/manual-delegation-tui.sh [evidence.md]` ejecuta el mismo candidato empaquetado y fixture en el target certificado y registra nombre y SHA-256 del artefacto, Pi, target y fecha. La revisión completada se conserva en [`docs/acceptance/delegation-tui-manual.md`](../acceptance/delegation-tui-manual.md), con resultado satisfactorio para render, controles, cursor, foco y entrada posterior al cierre.

## Estado del diseño

Diseño confirmado e Incrementos 1 y 2 implementados. El spec y sus fronteras están publicados en GitHub:

- [#55 — Specify native Delegation management](https://github.com/yersonargotev/matty/issues/55)
- [#56 — Spike the Delegation Console interaction](https://github.com/yersonargotev/matty/issues/56)
- [#57 — Observe Delegations during the current session](https://github.com/yersonargotev/matty/issues/57)
- [#58 — Cancel Delegations from the Delegation Console](https://github.com/yersonargotev/matty/issues/58)
- [#59 — Show an observable Delegation fleet](https://github.com/yersonargotev/matty/issues/59)
- [#60 — Add redacted Child Execution activity summaries](https://github.com/yersonargotev/matty/issues/60)

Los issues son sub-issues de #55 y formaron la cadena de entrega nativa. #56 conserva el spike histórico; #57, #58, #59 y #60 delimitan la implementación productiva completada. El target certificado actual es Pi `0.84.2` en `darwin/arm64`; el resultado histórico del spike sobre `0.83.0` permanece documentado como antecedente, no como certificación actual.
