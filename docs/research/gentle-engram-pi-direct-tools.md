# Engram en Pi sin MCP: herramientas nativas con `pi.registerTool()`

## Resumen ejecutivo

El reemplazo de MCP **ya existe en la rama `main` de Engram**: `plugin/pi/index.ts` registra directamente 19 herramientas `mem_*` con `pi.registerTool()` y las traduce a llamadas JSON al API local de `engram serve`. Por tanto, Pi no necesita `pi-mcp-adapter`, `mcp.json` ni el subproceso `engram mcp` para usar esas herramientas. La página oficial lo confirma: las herramientas Pi-nativas vienen de `gentle-engram`; `pi-mcp-adapter` sólo es necesario para el gateway MCP opcional o para otras integraciones MCP ([pi.dev: Requirements](https://pi.dev/packages/gentle-engram#requirements), [README del paquete: What gets installed](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/README.md#what-gets-installed)).

Arquitectura objetivo:

```text
Pi
  ├─ eventos de sesión
  └─ herramientas mem_* registradas con pi.registerTool()
               │ HTTP JSON
               ▼
       ENGRAM_URL / engram serve
               │
               ▼
          SQLite + FTS5
```

Se elimina únicamente este camino:

```text
Pi → pi-mcp-adapter → stdio → engram mcp
```

## Archivos exactos y responsabilidades

| Archivo | Función en la solución nativa |
|---|---|
| [`plugin/pi/index.ts`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/index.ts) | Implementación principal: variables de entorno, esquemas TypeBox, cliente HTTP, `pi.registerTool()`, renderizado y hooks de ciclo de vida. |
| [`plugin/pi/package.json`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/package.json) | Declara `pi.extensions: ["./index.ts"]`, dependencia `typebox`, UI y `pi-mcp-adapter` como *peer* opcional. La versión del snapshot de `main` es `0.1.10`. |
| [`plugin/pi/memory-tool-chrome.js`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/memory-tool-chrome.js) | Etiquetas y render compacto de llamadas/resultados. |
| [`plugin/pi/private-redaction.js`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/private-redaction.js) | Redacción recursiva de bloques `<private>` y valores enviados por HTTP. |
| [`plugin/pi/compaction-recovery.js`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/compaction-recovery.js) | Extracción de resumen de compactación y aviso de recuperación. |
| [`internal/server/server.go`](https://github.com/Gentleman-Programming/engram/blob/main/internal/server/server.go) | API REST local consumida por la extensión; `routes()` contiene el contrato efectivo. `engram serve` escucha en `127.0.0.1:<puerto>`. |
| [`internal/mcp/mcp.go`](https://github.com/Gentleman-Programming/engram/blob/main/internal/mcp/mcp.go) | Contrato MCP de referencia para comparar nombres, parámetros, perfiles y semántica; no participa en ejecución nativa. |
| [`plugin/pi/cli.js`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/cli.js) y [`mcp-template.json`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/mcp-template.json) | Setup heredado de doble camino. Hoy agregan `pi-mcp-adapter` y `mcpServers.engram`; deben cambiarse o evitarse para un modo realmente sin MCP. |
| [`plugin/pi/test/`](https://github.com/Gentleman-Programming/engram/tree/main/plugin/pi/test) | Pruebas Node del paquete; `package.json` ejecuta `node --test test/*.test.mjs`. |

El repositorio establece además el principio de “adaptador delgado”: plugins pueden iniciar/encontrar `engram serve`, crear sesiones, inyectar protocolo y registrar integración, pero la semántica central debe permanecer en Go ([Integrations: Thin plugin principle](https://github.com/Gentleman-Programming/engram/blob/main/docs/codebase/integrations.md#thin-plugin-principle)).

## API exacta de Pi utilizada

La implementación recorre `ENGRAM_TOOLS` y registra cada herramienta así ([`registerMemoryTools`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/index.ts)):

```ts
pi.registerTool({
  name: toolName,
  label: `Engram: ${humanToolName(toolName)}`,
  description: `Engram memory tool: ...`,
  promptSnippet: `Engram memory: ...`,
  parameters: MEMORY_TOOL_SCHEMAS[toolName], // Type.Object(...)
  renderShell: "self",
  async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
    return executeMemoryTool(toolName, params, ctx);
  },
  renderCall(args) { return new Text(...); },
  renderResult(result, options, _theme, context) { return new Text(...); },
});
```

`executeMemoryTool()` inicializa/autoinicia Engram, resuelve el proyecto, actualiza `ctx.ui.setStatus("engram", ...)` y devuelve el contrato Pi:

```ts
{
  content: [{ type: "text", text: textResult(data) }],
  details: { data }
}
```

En error devuelve el mismo sobre con `isError: true`. Los parámetros se definen con TypeBox (`Type.Object`, `Type.String`, `Type.Number`, `Type.Boolean`, `Type.Optional`) en `MEMORY_TOOL_SCHEMAS` ([fuente primaria](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/index.ts)).

La extensión también registra estos eventos mediante `pi.on(...)`:

- `session_start`: inicializa y, si corresponde, arranca `engram serve`.
- `session_shutdown`: limpia contadores, sesiones y estado de autorreparación.
- `session_compact`: persiste un `session_summary` de recuperación y carga contexto.
- `before_agent_start`: inyecta el Memory Protocol y captura el prompt.
- `tool_execution_end`: captura pasivamente aprendizajes de resultados `Task`, ignorando sus propias `mem_*`.

## Herramientas Pi-nativas y traducción HTTP exacta

La lista efectiva de `plugin/pi/index.ts` contiene **19 herramientas**:

| Herramienta | Parámetros del esquema Pi | HTTP/acción |
|---|---|---|
| `mem_search` | `query` requerido; `type`, `project`, `scope`, `limit`, `all_projects`, `match_mode` | `GET /search?q=...` |
| `mem_save` | `title`, `content` requeridos; `type`, `session_id`, `scope`, `topic_key`, `project`, `capture_prompt` | asegura `POST /sessions`; luego `POST /observations` |
| `mem_update` | `id` requerido; `title`, `content`, `type`, `scope`, `topic_key` | `PATCH /observations/{id}` |
| `mem_delete` | `id` requerido; `hard_delete` | `DELETE /observations/{id}?hard=...` |
| `mem_suggest_topic_key` | `type`, `title`, `content` | cálculo local `slugifyTopicKey()`; no llama al servidor |
| `mem_save_prompt` | `content` requerido; `session_id`, `project` | asegura sesión; `POST /prompts` |
| `mem_session_summary` | `content` requerido; `session_id`, `project` | asegura sesión; `POST /observations` con `type=session_summary` |
| `mem_context` | `project`, `scope` | `GET /context` |
| `mem_stats` | `project` | `GET /stats` (`project` sólo sirve al chrome) |
| `mem_timeline` | `observation_id` requerido; `before`, `after`, `project` | `GET /timeline` |
| `mem_get_observation` | `id` requerido | `GET /observations/{id}` |
| `mem_session_start` | `id` requerido; `directory` | `POST /sessions` |
| `mem_session_end` | `id` requerido; `summary` | `POST /sessions/{id}/end` |
| `mem_current_project` | `cwd` | `GET /project/current?cwd=...` |
| `mem_doctor` | `check`, `project` | `GET /doctor` |
| `mem_capture_passive` | `content` requerido; `session_id`, `source` | asegura sesión; `POST /observations/passive` |
| `mem_review` | `action` requerido; `project`, `limit`, `observation_id`, `id` | `GET /review` o `POST /review/mark_reviewed` |
| `mem_judge` | `judgment_id`, `relation` requeridos; `reason`, `evidence`, `confidence`, `session_id` | `POST /conflicts/judge` |
| `mem_compare` | `memory_id_a`, `memory_id_b`, `relation`, `confidence`, `reasoning` requeridos; `model` | `POST /conflicts/compare` |

La correspondencia anterior está implementada en `MEMORY_TOOL_SCHEMAS` y `callMemoryTool()` ([`plugin/pi/index.ts`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/index.ts)); las rutas están declaradas por el servidor Go y documentadas en la referencia técnica ([`internal/server/server.go`](https://github.com/Gentleman-Programming/engram/blob/main/internal/server/server.go), [DOCS: HTTP API Endpoints](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#http-api-endpoints)).

## Configuración exacta sin MCP

### Instalación operativa mínima

La documentación publicada en pi.dev fija actualmente este comando:

```bash
pi install npm:gentle-engram@0.1.8
```

Después, reiniciar Pi. **No instalar** `npm:pi-mcp-adapter` y **no crear** `mcpServers.engram`. La extensión se carga porque el paquete declara `pi.extensions` ([package.json](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/package.json)). La versión de la rama `main` (`0.1.10`) y la versión fijada por pi.dev/CLI (`0.1.8`) no coinciden; conviene alinear publicación, documentación y `PACKAGE_NAME` antes de cambiar el instalador.

Requisitos:

- binario `engram` en `PATH`, o `ENGRAM_BIN=/ruta/engram`;
- por defecto la extensión intenta iniciar `engram serve` y usa `http://127.0.0.1:7437`;
- alternativamente, servidor ya gestionado:

```bash
ENGRAM_URL=http://127.0.0.1:7437 pi
```

- puerto alternativo para autoarranque: `ENGRAM_PORT=<puerto>`;
- proyecto determinista, especialmente en monorepos:

```json
// <repo>/.engram/config.json
{
  "project_name": "mi-proyecto"
}
```

Estas variables y el orden de detección están documentados por el paquete ([pi.dev: Configuration y Project detection](https://pi.dev/packages/gentle-engram#configuration)). `ENGRAM_URL` es REST, **no MCP**; el README principal confirma que MCP sólo existe por stdio y que Pi usa HTTP para captura y herramientas nativas ([README: Setup FAQ](https://github.com/Gentleman-Programming/engram/blob/main/README.md#setup-faq)).

### Qué hacer con el setup actual

No usar `engram setup pi` ni `pi-engram init` sin modificaciones si el objetivo es “cero MCP”: actualmente ambos instalan/declaran `pi-mcp-adapter` y escriben `mcp.json`; el template ejecuta `engram mcp --tools=agent` con `directTools: false` ([`cli.js`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/cli.js), [`mcp-template.json`](https://github.com/Gentleman-Programming/engram/blob/main/plugin/pi/mcp-template.json), [pi.dev: Install command details](https://pi.dev/packages/gentle-engram#install-command-details)).

Para convertir el flujo oficial:

1. Hacer que `engram setup pi` instale sólo `gentle-engram` por defecto.
2. No agregar `npm:pi-mcp-adapter` a `settings.json`.
3. No escribir `mcpServers.engram` en `mcp.json`; retirar una entrada Engram previamente gestionada, conservando otros servidores MCP.
4. Ofrecer un flag explícito, por ejemplo `--with-mcp`, sólo para compatibilidad/depuración u otras integraciones.
5. Mantener `ENGRAM_BIN` porque sigue siendo necesario para autoarrancar `engram serve`, no para MCP.
6. Actualizar `plugin/pi/README.md`, `docs/AGENT-SETUP.md`, pi.dev y pruebas de setup.

## Brechas y riesgos antes de declarar paridad total

1. **Cobertura desigual.** El README enumera 20 herramientas MCP e incluye `mem_merge_projects`, pero la extensión Pi registra 19 y no la incluye ([README: MCP Tools](https://github.com/Gentleman-Programming/engram/blob/main/README.md#mcp-tools-20)). Además, `internal/mcp/mcp.go` de `main` ya incluye `mem_pin` y `mem_unpin` en `ProfileAgent`; tampoco existen en `ENGRAM_TOOLS`. Hay deriva entre documentación, MCP y Pi que debe resolverse explícitamente.
2. **Semántica duplicada.** `mem_suggest_topic_key` se implementa en TypeScript con un slug simple, mientras MCP delega al store Go. Esto contradice parcialmente el principio de adaptador delgado y puede producir claves distintas ([`internal/mcp/mcp.go`](https://github.com/Gentleman-Programming/engram/blob/main/internal/mcp/mcp.go), [integrations.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/codebase/integrations.md#thin-plugin-principle)). Debe exponerse una ruta HTTP canónica o compartir exactamente la regla Go.
3. **Parámetros anunciados pero no aplicados.** En la ruta Pi, `mem_save.capture_prompt` aparece en el esquema pero `callMemoryTool()` no lo usa; `mem_stats.project` tampoco filtra la consulta. Deben eliminarse o implementarse.
4. **Resolución de proyecto no equivalente.** MCP documenta validación fuerte de `project`, recuperación por `ambiguous_project`, `project_choice_reason` y `recovery_token`; el esquema Pi no expone los dos últimos y acepta el `project` explícito directamente para varias escrituras. La paridad requiere mover la resolución canónica a HTTP/Go y devolver los mismos sobres de error ([Agent Setup: Ambiguous-project recovery rules](https://github.com/Gentleman-Programming/engram/blob/main/docs/AGENT-SETUP.md#ambiguous-project-recovery-rules)).
5. **Conflictos al guardar.** MCP puede devolver candidatos y exige resolverlos con `mem_judge`; `POST /observations` responde esencialmente `{id,status}` en el handler HTTP actual. El camino nativo no debe prometer paridad de conflict surfacing hasta que el API HTTP entregue el mismo sobre.
6. **Autenticación HTTP.** `ENGRAM_HTTP_TOKEN` protege, entre otras, `DELETE /observations/{id}` y `POST /projects/migrate`, pero `engramFetch()` no envía `Authorization`. Con token activo, `mem_delete` fallará y la migración automática será ignorada. Añadir soporte explícito de token sin registrarlo en logs/render ([`requireAuth` y `routes()`](https://github.com/Gentleman-Programming/engram/blob/main/internal/server/server.go), [DOCS: Environment Variables](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#environment-variables)).
7. **Topología remota.** `engram serve` enlaza sólo `127.0.0.1`; `ENGRAM_URL` puede apuntar a otro servidor, pero el binario estándar no ofrece todavía bind no-loopback. No se debe presentar como API de red lista para producción ([README: Docker/remote agents](https://github.com/Gentleman-Programming/engram/blob/main/README.md#setup-faq)).
8. **Versionado inconsistente.** `plugin/pi/package.json` dice `0.1.10`, mientras pi.dev, `cli.js` y los ejemplos fijan `0.1.8`. El modo sin MCP debe publicar y documentar una única versión verificable.

## Recomendación de implementación

La opción de menor riesgo es consolidar la implementación existente, no crear otra extensión:

1. Mantener `plugin/pi/index.ts` como adaptador Pi-nativo y `internal/server/server.go` como frontera.
2. Completar rutas HTTP en Go para toda semántica que hoy sólo vive en MCP: sugerencia de topic key, validación/recovery de proyecto, candidatos de conflicto, pin/unpin y, si se desea paridad administrativa, merge.
3. Registrar cada nueva ruta con el mismo patrón `pi.registerTool()` y esquemas TypeBox, manteniendo `renderCall`/`renderResult` compactos.
4. Marcar/describir correctamente herramientas destructivas en Pi y añadir soporte de cancelación (`AbortSignal`) e idempotencia para escrituras; hoy el código evita reintentar timeouts de escritura porque el resultado puede ser ambiguo.
5. Añadir pruebas de contrato que comparen nombres, parámetros y sobres Pi ↔ HTTP ↔ MCP, evitando nueva deriva.
6. Cambiar setup a Pi-nativo por defecto y MCP sólo opt-in.

## Criterios de aceptación

- Pi muestra una sola fila por herramienta `mem_*`, sin prefijos/duplicados MCP.
- No existe proceso `engram mcp`; sí existe `engram serve` local o `ENGRAM_URL` accesible.
- `settings.json` carga `gentle-engram`; no requiere `pi-mcp-adapter` para Engram.
- No hay `mcpServers.engram` gestionado en `mcp.json`.
- Las 19 herramientas actuales pasan pruebas de éxito, error, timeout y proyecto ambiguo.
- Se decide y prueba explícitamente el destino de `mem_merge_projects`, `mem_pin` y `mem_unpin`.
- `mem_save`, proyecto, conflictos, topic keys y autenticación tienen semántica equivalente al core Go.
- Compactación, captura de prompts, redacción `<private>` y captura pasiva siguen funcionando sin MCP.

## Conclusión

La sustitución es viable y, para las 19 herramientas actuales, ya está materializada en el paquete oficial. El cambio operativo inmediato es cargar sólo `gentle-engram` y retirar la configuración MCP. El trabajo restante es principalmente de **consolidación de setup y paridad contractual**, no de inventar un nuevo transporte: Pi llama herramientas nativas; la extensión adapta a HTTP; Engram Go conserva persistencia y semántica central.
