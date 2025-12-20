> GET /v1/arrow

### Request Body

Empty

### Response Body

#### Success (2xx)
```json
{ 	// TODO Paginacion.
	"success": true,
	"http_cat": "https://http.cat/status/500",
	"warnings": [...],
	"error": {},
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
	"http_cat": "https://http.cat/status/500",
	"warnings": [],
	"error": {
		"code": 500,
		"message": "FAILED_TO_RETRIEVE_ARROWS",
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

#### Validaciones de Repository (Request Structure)

#### Validaciones para cuando se implemente Paginación (TODO)
- Validar `page` >= 1
- Validar `limit` entre 1 y 100
- Validar `status` filter es uno de: "standby", "executing", "exiting", "failed"
- Validar `sort_by` es uno de: "name", "namespace", "added_at", "last_execution", "status"
- Validar `sort_order` es "asc" o "desc"
- Validar encoding de query parameters

#### Validaciones de Query (Business Logic)
- Leer desde proyección pre-construida optimizada para listado
- Aplicar filtros a nivel de base de datos
- Retornar resultados paginados con metadata de paginación
- Si proyección está desactualizada, trigger rebuild asíncrono
