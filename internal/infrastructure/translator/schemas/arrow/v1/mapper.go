package v1

import (
	_ "embed"
	"fmt"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/port"
	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/runtime"
	"github.com/rabbytesoftware/quiver/internal/models/system"
	"github.com/rabbytesoftware/quiver/internal/models/variable"
)

//go:embed schema.json
var schemaJSON []byte

type ArrowV1Mapper struct{}

func NewMapper() *ArrowV1Mapper {
	return &ArrowV1Mapper{}
}

func (m *ArrowV1Mapper) Map(yamlData map[string]interface{}) (*arrow.Arrow, error) {
	arrowModel := &arrow.Arrow{
		ID: uuid.New(),
	}

	if metadata, ok := yamlData["metadata"].(map[string]interface{}); ok {
		arrowModel.Name = getStringField(metadata, "name")
		arrowModel.Description = getStringField(metadata, "description")
		arrowModel.Version = getStringField(metadata, "version")
		arrowModel.License = getStringField(metadata, "license")
		arrowModel.URL = system.URL(getStringField(metadata, "quiver_url"))

		if credits, ok := metadata["credits"].([]interface{}); ok {
			for _, credit := range credits {
				if c, ok := credit.(map[string]interface{}); ok {
					arrowModel.Credits = append(arrowModel.Credits, getStringField(c, "name"))
				}
			}
		}
	}

	if req, ok := yamlData["requirements"].(map[string]interface{}); ok {
		arrowModel.Requirements = mapRequirements(req)
	}

	if netbridge, ok := yamlData["netbridge"].([]interface{}); ok {
		arrowModel.Netbridge = mapNetbridge(netbridge)
	}

	if variables, ok := yamlData["variables"].([]interface{}); ok {
		arrowModel.Variables = mapVariables(variables)
	}

	if methods, ok := yamlData["methods"].(map[string]interface{}); ok {
		arrowModel.Methods = mapMethods(methods)
	}

	return arrowModel, nil
}

func (m *ArrowV1Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}

func mapRequirements(req map[string]interface{}) requirement.Requirement {
	r := requirement.Requirement{
		CpuCores: getIntField(req, "cpu_cores"),
		Memory:   getIntField(req, "ram_gb"),
		Disk:     getIntField(req, "disk_gb"),
	}

	if systems, ok := req["system"].([]interface{}); ok && len(systems) > 0 {
		if firstSys, ok := systems[0].(string); ok {
			r.OS = system.OS(firstSys)
		}
	}

	return r
}

func mapNetbridge(netbridge []interface{}) []port.PortRule {
	rules := []port.PortRule{}
	for _, nb := range netbridge {
		if nbMap, ok := nb.(map[string]interface{}); ok {
			rule := port.PortRule{
				ID: getStringField(nbMap, "name"),
			}
			protocolStr := getStringField(nbMap, "protocol")
			switch protocolStr {
			case "tcp":
				rule.Protocol = port.ProtocolTCP
			case "udp":
				rule.Protocol = port.ProtocolUDP
			case "tcp/udp":
				rule.Protocol = port.ProtocolTCPUDP
			}
			rules = append(rules, rule)
		}
	}
	return rules
}

func mapVariables(variables []interface{}) []variable.Variable {
	vars := []variable.Variable{}
	for _, v := range variables {
		if vMap, ok := v.(map[string]interface{}); ok {
			vars = append(vars, variable.Variable{
				Name:      getStringField(vMap, "name"),
				Default:   getStringField(vMap, "default"),
				Sensitive: getBoolField(vMap, "sensitive"),
			})
		}
	}
	return vars
}

func mapMethods(methods map[string]interface{}) []runtime.Method {
	runtimeMethods := []runtime.Method{}

	for osName, osData := range methods {
		if osMap, ok := osData.(map[string]interface{}); ok {
			for archName, archData := range osMap {
				if archMap, ok := archData.(map[string]interface{}); ok {
					osArch := fmt.Sprintf("%s/%s", osName, archName)

					if install, ok := archMap["install"].([]interface{}); ok {
						runtimeMethods = append(runtimeMethods, runtime.Method{
							OS:      system.OS(osArch),
							Command: toStringSlice(install),
						})
					}
				}
			}
		}
	}

	return runtimeMethods
}

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntField(m map[string]interface{}, key string) int {
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getBoolField(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func toStringSlice(data []interface{}) []string {
	result := []string{}
	for _, item := range data {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
