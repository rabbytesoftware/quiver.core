> POST /v1/arrow/{{namespace}}/execute/{{method}}
> Header x-idempotency-key: UUID-v4

### Request Body
```json
{
	"variables": {
		"VARIABLE_NAME": "value",
		"ANOTHER_VAR": "another-value"
	}
}
```

#### Success (2xx)
```json
{
	"success": true,
	"http_cat": "https://http.cat/status/202",
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
		"message": "METHOD_ALREADY_EXECUTING",
	}
}
```

### Repository Resp
```go
func (r *Repository) ExecuteMethod(
	ctx context.Context,
	namespace string,
	method string,
	variables map[string]string,
) (
	warnings []errors, 
	err error,
) {
	...
	return warnings, err
}

func (r *Repository) StopMethod(
	ctx context.Context,
	namespace string,
	method string,
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
- Validar que `variables` sea un objeto/map válido
- Validar longitud máxima de cada variable: key <= 128 chars, value <= 4096 chars
- Validar caracteres permitidos en nombres de variables: `^[A-Z][A-Z0-9_]*$`
- Validar que `idempotency_key` sea UUID v4 válido (si se provee)

#### Validaciones de Command (Business Logic)
- Verificar que el arrow exista en el event store → 404 Not Found
- Verificar que el método exista en arrow.methods → 404 Not Found
- Verificar estado actual del arrow:
  - Si status == "executing" → 409 Conflict (ya hay método ejecutándose)
  - Si status == "exiting" → 409 Conflict (arrow apagándose)
  - Si status == "standby" → Permitir métodos disponibles
- Validar `state-machine` de métodos (transiciones válidas).
- Validar variables requeridas:
  - Todas las variables con `required: true` deben estar presentes
  - Variables no presentes usan valores default (si existen)
  - Variables extra no definidas → Warning
- Validar tipos de variables:
  - `type: "string"` → validar es string
  - `type: "number"` → validar es número válido
  - `type: "boolean"` → validar es "true" o "false"
  - `type: "path"` → validar es path válido del sistema
- Validar constraints de variables:
  - `min/max` para números
  - `min_length/max_length` para strings
  - `pattern` (regex) para validación custom
  - `enum` para valores permitidos específicos
- Validar prerequisitos del método:
  - Dependencias instaladas
  - Puertos disponibles (si el método los requiere)
  - Recursos suficientes (CPU, RAM)
- Adquirir lock distribuido del arrow aggregate
- Generar execution_id único (UUID v4)
- Aplicar control de concurrencia optimista (aggregate version)

### Eventos Emitidos

#### Evento de Solicitud
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.ExecuteMethod.Requested",   // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 7,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // ID que agrupa todos los eventos de esta operación
	"parent_id": null,                               // ID del evento que causó este (null = evento raíz)
	"timestamp": "2025-12-20T10:30:00.123Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"method": "{{method}}",
		"execution_id": "uuid-v4-string",
		"variables": {
			"VARIABLE": "value"
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",                  // IP del cliente que hizo la petición
		"idempotency_key": "uuid-v4-string"          // Clave para prevenir duplicados
	}
}
```

#### Evento de Inicio
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.ExecuteMethod.Started",     // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 8,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Requested que causó este
	"timestamp": "2025-12-20T10:30:00.456Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"method": "{{method}}",
		"execution_id": "uuid-v4-string",
		"total_steps": 10
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string"
	}
}
```

#### Evento de Progreso (emitido periódicamente)
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.ExecuteMethod.ProgressUpdated", // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 9,                          // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Started que causó este
	"timestamp": "2025-12-20T10:30:15.789Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"method": "{{method}}",
		"execution_id": "uuid-v4-string",
		"current_step": 3,
		"total_steps": 10,
		"action": "{{action}}",
		"action_description": {
			"en": "Installing SteamCMD",
			"es": "Instalando SteamCMD"
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1"
	}
}
```

#### Evento de Éxito
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.ExecuteMethod.Succeeded",   // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 10,                         // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Started que causó este
	"timestamp": "2025-12-20T10:35:00.123Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"method": "{{method}}",
		"execution_id": "uuid-v4-string",
		"result": {
			"exit_code": 0
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 299667                        // Duración de la operación en milisegundos
	}
}
```

#### Evento de Fallo
```json
{
	"event_id": "uuid-v4-string",                    // ID único del evento
	"event_type": "arrow.ExecuteMethod.Failed",      // Tipo de evento
	"event_version": "1.0",                          // Versión del esquema del evento
	"aggregate_id": "{{namespace}}",                 // ID del aggregate (namespace del arrow)
	"aggregate_type": "arrow",                       // Tipo de aggregate
	"aggregate_version": 10,                         // Versión del aggregate después de este evento
	"correlation_id": "uuid-v4-string",              // Mismo ID de la operación completa
	"parent_id": "uuid-v4-string",                   // ID del evento Started que causó este
	"timestamp": "2025-12-20T10:32:30.456Z",         // Timestamp ISO8601 del evento
	"payload": {
		"namespace": "{{namespace}}",
		"method": "{{method}}",
		"execution_id": "uuid-v4-string",
		"current_step": 3,
		"total_steps": 10,
		"error": {
			"code": 500,
			"message": "EXECUTION_FAILED_AT_STEP",
		}
	},
	"metadata": {
		"client_ip": "192.168.1.1",
		"idempotency_key": "uuid-v4-string",
		"duration_ms": 150333,
	}
}
```
