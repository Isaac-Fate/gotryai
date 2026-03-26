package structured

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

func GetSchemaString[T any](t T) string {
	schema := jsonschema.Reflect(t)
	schemaBytes, _ := json.MarshalIndent(schema, "", "  ")
	return string(schemaBytes)
}
