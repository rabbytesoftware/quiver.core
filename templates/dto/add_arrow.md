> POST /v1/arrow/{{namespace}}
> POST /v1/arrow

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
	"http_cat": "https://http.cat/status/2xx",
	"warnings": [
		...
	],
	"error": {},
}
```

#### Error (4xx/5xx)
```json
{
	"success": false,
	"http_cat": "https://http.cat/status/4xx|500x",
	"warnings": [
		{
			"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos.",
			"status": "4xx/5xx"
		}
	],
	"error": {
		"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos.",
		"status": "4xx/5xx"
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

#### Validaciones (que venga bien la request)
- No enviar {{namespace}} y {{body.path}} al mismo tiempo.
- Verificar que {{body.path}} (`fns.Validate()`)

#### Command
- Si existe la arrow -> Falla
- Si se encuentra visible y usable
- Validar si el OS es compatible
- Validar si cumple los requirements (`SRV`) o `body.force_add == true`
- Prerequisitos (dependencias -> Cap. faltante `dependency-engine`)

### Event Emitted
```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.AddArrow.Request",
	"payload": {
		"path": "PATH|URL",
		"namespace": "{{namespace}}",
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```

```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.AddArrow.Success",
	"payload": {
		"namespace": "{{namespace}}"
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```

```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.AddArrow.Fail",
	"payload": {
		"namespace": "{{namespace}}",
		"status": 500, // Internal Server Error
		"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos."
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```
