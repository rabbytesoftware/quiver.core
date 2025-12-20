> GET /v1/arrow/{{namespace}}

### Request Body
Empty

### Response Body

#### Success (2xx)
```json
{
	"success": true,
	"http_cat": "https://http.cat/status/200",
	"warnings": [],
	"error": {},
	"arrow": {
		"status": "executing" | "standby" | "exiting" | "failed",
		"methods": [
			"install": {
				"title": {
					"en": "Install",
					"es": "Instalar"
				}
			},
			"update": {
				"title": {
					"en": "Update",
					"es": "Actualizar"
				}
			},
			...
		],
		"action": { // TODO cambiar nombre.
			"method": "install",
			"title": {
				"en": "Installing",
				"es": "Instalando"
			},
			"step_index": 1,
			"steps": 10,
		},
		"metadata": {
			"name": "quiver.chat",
			"description": "Quiver Chat is a chat client for the Quiver platform.",
			"version": "25.11.0",
			"license": "MIT",
			"quiver": "github.com/rabbytesoftware/quiver.experiments",
			"media": {
				"icon": "https://quiver.ar/quiver/icon.png",
				"banner": "https://quiver.ar/quiver/banner.png"
			},
			"credits": [
				{
					"name": "char2cs",
					"email": "me@char2cs.net",
					"url": "https://char2cs.net"
				}
			]
		},
		"requirements": {
			"cpu_cores": 1,
			"ram_gb": 1,
			"disk_gb": 1,
			"network_mbps": 1
		},
		"variables": [
			{
				"name": "QUIVER_CHAT_HOSTNAME",
				"default": "chat.quiver.ar",
				"sensitive": false
			}
		],
		"netbridge": [
			{
				"name": "QUIVER_CHAT_HOSTNAME",
				"protocol": "tcp"
			}
		]
	}
}
```

#### Error (4xx/5xx)
```json
{
	"success": false,
	"http_cat": "https://http.cat/status/404",
	"warnings": [],
	"error": {
		"code": 404,
		"message": "ARROW_NOT_FOUND",
	}
}
```

### Repository Resp
```go
func (r *Repository) GetArrow(
	ctx context.Context,
	namespace string,
) (warnings []errors, err error) {
	...
	return warnings, err
}
```

#### Validaciones de Repository (Request Structure)
- Validar formato del `namespace`

#### Validaciones de Query (Business Logic)
- Verificar que el arrow exista en el event store → 404 Not Found
- Reconstruir estado actual desde la proyección pre-construida
- Si proyección no existe, reconstruir desde eventos (fallback)
- Retornar estado completo del arrow con metadata
