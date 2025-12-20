> GET /v1/arrow

### Request Body

Empty

### Response Body

#### Success (2xx)
```json
{ 	// TODO Paginacion.
	"success": true,
	"http_cat": "https://http.cat/status/2xx",
	"warnings": [...],
	"arrows": [
		"{{namespace}}": {
			"status": "executing" | "standby" | "exiting" | "failed",
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
			},
		}
	]
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
func (r *Repository) ListArrows(
	ctx context.Context,
) (
	arrows map[string]models.Arrows, 
	warnings []errors, 
	err error,
) {
	...
	return arrows, warnings, err
}
```

#### Validaciones (que venga bien la request)
- Ninguna.
