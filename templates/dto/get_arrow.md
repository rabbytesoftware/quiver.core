> GET /v1/arrow/{{namespace}}

### Request Body
Empty

### Response Body

#### Success (2xx)
```json
{
	"status": "executing" | "standby" | "exiting" | "failed",
	"method": [
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
	"arrow": {
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
	"http_cat": "https://http.cat/status/4xx|500x",
	"warnings": [...],
	"error": {
		"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos.",
		"status": "4xx/5xx"
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

#### Validaciones (que venga bien la request)
- Que sea {{namespace}} valido
- Que este en el EventStore.
