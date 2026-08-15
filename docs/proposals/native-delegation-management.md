# Propuesta: gestión nativa de Delegations

**Estado:** diseño en grilling, sin implementación

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

El Incremento 1 se considera aceptado únicamente cuando:

- no cambia la frontera de procesos;
- estados, transiciones, límites y desalojo tienen tests;
- cancelar conserva atomicidad y SIGTERM→SIGKILL;
- ninguna vista expone payloads sensibles;
- pasa `npm run check` completo;
- pasa aceptación con el artefacto empaquetado en el Pi certificado;
- una prueba manual confirma que la TUI no se corrompe ni presenta controles ambiguos.

## Incremento 2 — Fleet observable

Este es el siguiente incremento explícito después de superar el gate anterior:

1. widget compacto de actividad global;
2. timeline navegable por Delegated Task;
3. resúmenes seguros de herramientas y actividad;
4. mejor inspección de concurrencia, cola y consumo;
5. controles adicionales sólo cuando no contradigan atomicidad ni autoridad de roles.

Antes de implementarlo se debe definir una política cerrada de redacción para los resúmenes y validar interacción de teclado, modo headless/print, coste de render y convivencia con otras extensiones.

## Incremento 3 — Control avanzado

Backlog preservado, no comprometido por los dos primeros incrementos:

- steering durante ejecución;
- retry y resume;
- visor de conversación;
- historial persistente;
- interoperabilidad RPC opcional con `pi-subagents`.

Estas capacidades requieren decisiones nuevas sobre protocolo bidireccional, identidad de intentos, privacidad, almacenamiento, retención, recuperación y compatibilidad certificada. No se simularán sobre primitivas insuficientes.

## Fuera de alcance actual

- reemplazar Child Executions con `AgentSession` in-process;
- usar `pi-subagents` para ejecutar Matty Roles;
- cancelación individual de tareas;
- persistencia entre sesiones;
- copiar FleetView o el manager completo de `pi-subagents`;
- mostrar payloads o transcripts de hijos.

## Estrategia de entrega

Antes de la implementación definitiva se hará un spike desechable que valide únicamente:

- abrir `/matty delegations` mientras `subagent` ejecuta;
- actualizar la vista sin corrupción visual;
- seleccionar y confirmar cancelación;
- cerrar la consola y devolver correctamente el foco.

El código del spike no se convierte directamente en arquitectura de producción. Superado el spike, el Incremento 1 se implementa con tests sobre seams explícitos.

El issue tracker conservará dos alcances separados:

1. un issue implementable para el spike y el Incremento 1;
2. un issue para el Incremento 2 — Fleet observable, bloqueado por el primero.

El Incremento 3 permanece en esta propuesta hasta contar con evidencia suficiente para dividirlo en trabajo implementable.

## Estado del diseño

Diseño confirmado. El spec y sus tracer bullets están publicados en GitHub:

- [#55 — Specify native Delegation management](https://github.com/yersonargotev/matty/issues/55)
- [#56 — Spike the Delegation Console interaction](https://github.com/yersonargotev/matty/issues/56)
- [#57 — Observe Delegations during the current session](https://github.com/yersonargotev/matty/issues/57)
- [#58 — Cancel Delegations from the Delegation Console](https://github.com/yersonargotev/matty/issues/58)
- [#59 — Show an observable Delegation fleet](https://github.com/yersonargotev/matty/issues/59)
- [#60 — Add redacted Child Execution activity summaries](https://github.com/yersonargotev/matty/issues/60)

Los issues son sub-issues de #55 y forman una cadena de dependencias nativas en el orden listado. #56 es la frontera inicial lista para implementación.
