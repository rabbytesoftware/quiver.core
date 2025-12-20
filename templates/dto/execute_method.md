> POST /v1/arrow/{{namespace}}/execute/{{method}}

### Request Body
```json
{
	"VARIABLE": "asdad" // <-- Arrow's Method Variables
	...
}
```

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
func (r *Repository) ExecuteMethod(
	ctx context.Context,
	namespace string,
	params map[string]string,
) (
	warnings []errors, 
	err error,
) {
	...
	return warnings, err
}
```

#### Validaciones
- Validar arrow.
- Validar metodo.
- Validar variables.

#### Command
- Validar estado arrow
  - Que no se este ejecutando nada.
  - Que el metodo se pueda ejecutar.
  - Mapeo de variables y ver si estan presentes todos los prerequisitos.  
- Devolver tipo de repuesta y status code.

### Event Emitted
```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.ExecuteMethod.Request",
	"payload": {
		"method": "{{method}}",
		"variables": {
			"VARIABLE": "asdad" // O default.
		}
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```

```json
{
	"aggregate_id": "{{namespace}}",
	"aggregate_type": "arrow",
	"event_type": "arrow.ExecuteMethod.Success",
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
	"event_type": "arrow.ExecuteMethod.Fail",
	"payload": {
		"namespace": "{{namespace}}",
		"status": 500, // Internal Server Error
		"reason": "Lorem ipsum dolor sit amet consectetur adipiscing elit. Quisque faucibus ex sapien vitae pellentesque sem placerat. In id cursus mi pretium tellus duis convallis. Tempus leo eu aenean sed diam urna tempor. Pulvinar vivamus fringilla lacus nec metus bibendum egestas. Iaculis massa nisl malesuada lacinia integer nunc posuere. Ut hendrerit semper vel class aptent taciti sociosqu. Ad litora torquent per conubia nostra inceptos himenaeos."
	},
	"version": "v1",
	"timestamp": 2025-12-19
}
```
