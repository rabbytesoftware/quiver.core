> DELETE /v1/arrow/{{namespace}}

### Request Body
Empty

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
func (r *Repository) RemoveArrow(
	ctx context.Context,
	namespace string,
) (
	warnings []errors, 
	err error,
) {
	...
	return warnings, err
}
```

#### Validaciones (que venga bien la request)
- Verificar que {{namespace}} sea valido.

#### Command
- Si no existe la arrow -> Falla
- Que no este running nada.

### Event Emitted
```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.RemoveArrow.Request",
	"payload": {
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
	"event_type": "arrow.RemoveArrow.Success",
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
	"event_type": "arrow.RemoveArrow.Fail",
	"payload": {
		"namespace": "{{namespace}}",
		"status": 500, // Internal Server Error
		"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos."
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```
