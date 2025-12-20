> DELETE /v1/arrow/{{namespace}}
> Header x-idempotency-key: UUID-v4

### Request Body
```json
{
	"force": false  // Default false. Si true, fuerza eliminación aunque esté ejecutando
}
```

### Response Body
#### Success (2xx)
```json
{
	"success": true,
	"http_cat": "https://http.cat/status/200",
	"warnings": [
		...
	],
	"error": {}
}
```

#### Error (4xx/5xx)
```json
{
	"success": false,
	"http_cat": "https://http.cat/status/409",
	"warnings": [],
	"error": {
		"code": 409,
		"message": "ARROW_CURRENTLY_EXECUTING"
	}
}
```

### Repository Resp
```go
func (r *Repository) DeleteArrow(
	ctx context.Context,
	namespace string,
	force bool,
) (
	warnings []errors, 
	err error,
) {
	...
	return warnings, err
}
```

#### Validaciones de Repository (Request Structure)
- Validar formato del `namespace`
- Validar que `force` sea boolean
- Validar que `idempotency_key` sea UUID v4 válido (si se provee)

#### Validaciones de Command (Business Logic)
- Verificar que el arrow exista en el event store → 404 Not Found
- Verificar estado actual del arrow desde la proyección:
  - Si status == "executing" y force == false → 409 Conflict
  - Si status == "exiting" → 409 Conflict (no remover mientras se apaga)
  - Si status == "standby" → Permitir eliminación
  - Si status == "failed" → Permitir eliminación (cleanup)
- Si force == true:
  - Intentar graceful shutdown del arrow (SIGTERM)
  - Esperar timeout configurable (ej: 30 segundos)
  - Si no responde, forzar kill (SIGKILL)
  - Emitir evento de forced shutdown
  - Elimina Arrow sin importar dependencias
- Verificar que no haya otros arrows que dependan de este (si `force == false`)
- Archivar configuración y datos del arrow antes de eliminar
- Aplicar control de concurrencia optimista (aggregate version)

### Eventos Emitidos

#### Evento de Solicitud
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.RemoveArrow.Requested",     // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 5,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // ID que agrupa todos los eventos de esta operación
	"parent_id": null,                               // ID del evento que causó este (null = evento raíz)
	"timestamp": "2025-12-20T10:30:00.123Z",        // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"force": false
	},
	"metadata": {
		"client_ip": "192.168.1.1",                  // IP del cliente que hizo la petición
		"idempotency_key": "uuid-v4-string"          // Clave para prevenir duplicados
	}
}
```

#### Evento de Éxito
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.RemoveArrow.Succeeded",     // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 6,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Requested que causó este
	"timestamp": "2025-12-20T10:30:02.456Z",        // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}"
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 2333                          // Duración de la operación en milisegundos
	}
}
```

#### Evento de Fallo
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.RemoveArrow.Failed",        // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 6,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Requested que causó este
	"timestamp": "2025-12-20T10:30:02.456Z",        // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"dependency_children": [
			"{{namespace}}"
		],
		"error": {
			"code": 409,
			"message": "ARROW_CURRENTLY_EXECUTING"
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 2333,
		"retryable": true,                           // Indica si la operación puede reintentarse
		"retry_after_seconds": 300                   // Sugerencia de cuándo reintentar
	}
}
```
