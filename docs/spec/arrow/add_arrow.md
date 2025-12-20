> POST /v1/arrow/{{namespace}}
> POST /v1/arrow
> Header x-idempotency-key: UUID-v4

### Request Body
```json
{
	"path": "PATH|URL", // Opt.
	"force_add": false  // Default false.
}
```

### Response Body
#### Success (2xx)
```json
{
	"success": true,
	"http_cat": "https://http.cat/status/201",
	"warnings": [
		{
			"code": 429,
			"message": "MISSING_OPTIONAL_DEPENDENCY",
		}
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
		"message": "ARROW_ALREADY_EXISTS"
	}
}
```

### Repository Resp
```go
func (r *Repository) AddArrow(
	ctx context.Context,
	namespace string,
	path string,
	force bool,
) (
	arrow models.Arrow, 
	warnings []errors, 
	err error,
) {
	...
	return arrow, warnings, err
}
```

#### Validaciones de Repository (Request Structure)
- Validar formato del `namespace`
- No enviar {{namespace}} y {{body.path}} al mismo tiempo
- Validar que `body.path` sea URL válida o path absoluto válido
- Validar que `body.path` no esté vacío cuando se provee
- Validar que `force_add` sea boolean
- Validar que `idempotency_key` sea UUID v4 válido (si se provee)

#### Validaciones de Command (Business Logic)
- Verificar que el arrow no exista ya → 409 Conflict
- Validar que el arrow sea accesible y descargable (si es URL)
- Validar que el archivo arrow.yaml sea válido (parsing correcto)
- Validar compatibilidad del sistema operativo con arrow.platforms
- Validar recursos del sistema:
  - CPU cores disponibles >= arrow.requirements.cpu_cores
  - RAM disponible >= arrow.requirements.ram_gb
  - Disk espacio libre >= arrow.requirements.disk_gb
  - Network bandwidth >= arrow.requirements.network_mbps
- Si `force_add == false` y no cumple requirements → Falla con error específico
- Si `force_add == true` → Continuar con warning de requisitos no cumplidos
- Resolver y validar dependencias (prerequisitos)
- Validar que dependencias no formen ciclos circulares

### Eventos Emitidos

#### Evento de Solicitud
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.AddArrow.Requested",        // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 1,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // ID que agrupa todos los eventos de esta operación
	"parent_id": null,                               // ID del evento que causó este (null = evento raíz)
	"timestamp": "2025-12-20T10:30:00.123Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"path": "PATH|URL",
		"force_add": false
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
	"event_type": "arrow.AddArrow.Succeeded",        // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 2,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Requested que causó este
	"timestamp": "2025-12-20T10:30:05.456Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"arrow_data": {
			"name": "arrow-name",
			"version": "1.0.0",
			"description": "Descripción del arrow"
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 5333                          // Duración de la operación en milisegundos -- Inconsistencia Eventual
	}
}
```

#### Evento de Fallo
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.AddArrow.Failed",           // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 2,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Requested que causó este
	"timestamp": "2025-12-20T10:30:05.456Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"error": {
			"code": 409,
			"message": "NOT_ENOUGH_DEDICATED_WAM"
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 5333,
		"retryable": false                          // Indica si la operación puede reintentarse
	}
}
```
