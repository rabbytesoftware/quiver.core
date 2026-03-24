package v1

import (
	_ "embed"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainnetbridge "github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/utils"
)

//go:embed schema.json
var schemaJSON []byte

type ArrowV1Mapper struct{}

func NewMapper() *ArrowV1Mapper {
	return &ArrowV1Mapper{}
}

func (m *ArrowV1Mapper) Map(yamlData map[string]interface{}) (*domain.Arrow, error) {
	arrowModel := &domain.Arrow{}

	if metadata, ok := yamlData["metadata"].(map[string]interface{}); ok {
		arrowModel.Manifest.Name = utils.GetStringField(metadata, "name")
		arrowModel.Manifest.Description = utils.GetStringField(metadata, "description")
		arrowModel.Manifest.Version = utils.GetStringField(metadata, "version")
		arrowModel.Manifest.License = utils.GetStringField(metadata, "license")
		arrowModel.Manifest.URL = utils.GetStringField(metadata, "quiver")

		if credits, ok := metadata["credits"].([]interface{}); ok {
			for _, credit := range credits {
				if c, ok := credit.(map[string]interface{}); ok {
					arrowModel.Manifest.Credits = append(arrowModel.Manifest.Credits, utils.GetStringField(c, "name"))
				}
			}
		}
	}

	if req, ok := yamlData["requirements"].(map[string]interface{}); ok {
		arrowModel.Manifest.Requirements = mapRequirements(req)
	}

	if netbridge, ok := yamlData["netbridge"].([]interface{}); ok {
		arrowModel.Manifest.Netbridge = mapNetbridge(netbridge)
	}

	if variables, ok := yamlData["variables"].([]interface{}); ok {
		arrowModel.Manifest.Variables = mapVariables(variables)
	}

	if wizards, ok := yamlData["wizards"].([]interface{}); ok {
		arrowModel.Manifest.Lifecycle, arrowModel.Manifest.Methods = mapWizards(wizards)
	}

	if err := validateArrow(arrowModel); err != nil {
		return nil, fmt.Errorf("arrow validation failed: %w", err)
	}

	return arrowModel, nil
}

func (m *ArrowV1Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}

func validateArrow(arrow *domain.Arrow) error {
	if arrow.Manifest.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if arrow.Manifest.Version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	for _, port := range arrow.Manifest.Netbridge {
		if port.Name == "" {
			return fmt.Errorf("netbridge port missing name")
		}
	}
	return nil
}

func mapRequirements(req map[string]interface{}) domain.Requirement {
	r := domain.Requirement{
		CpuCores: utils.GetIntField(req, "cpu_cores"),
		MemoryGB: utils.GetIntField(req, "ram_gb"),
		DiskGB:   utils.GetIntField(req, "disk_gb"),
	}

	if systems, ok := req["system"].([]interface{}); ok {
		for _, sys := range systems {
			if s, ok := sys.(string); ok {
				r.OS = append(r.OS, domain.OS(s))
			}
		}
	}

	return r
}

func mapNetbridge(netbridgeData []interface{}) []domainnetbridge.PortDef {
	portDefs := []domainnetbridge.PortDef{}
	for _, nb := range netbridgeData {
		if nbMap, ok := nb.(map[string]interface{}); ok {
			portDef := domainnetbridge.PortDef{
				Name:     utils.GetStringField(nbMap, "name"),
				Required: utils.GetBoolField(nbMap, "required"),
				Default:  utils.GetIntField(nbMap, "default"),
			}
			protocolStr := utils.GetStringField(nbMap, "protocol")
			switch protocolStr {
			case "tcp":
				portDef.Protocol = domainnetbridge.ProtocolTCP
			case "udp":
				portDef.Protocol = domainnetbridge.ProtocolUDP
			case "tcp/udp":
				portDef.Protocol = domainnetbridge.ProtocolTCPUDP
			}
			portDefs = append(portDefs, portDef)
		}
	}
	return portDefs
}

func mapVariables(variables []interface{}) []domain.Variable {
	vars := []domain.Variable{}
	for _, v := range variables {
		if vMap, ok := v.(map[string]interface{}); ok {
			variable := domain.Variable{
				Name:      utils.GetStringField(vMap, "name"),
				Default:   utils.GetStringField(vMap, "default"),
				Sensitive: utils.GetBoolField(vMap, "sensitive"),
				Min:       utils.GetIntField(vMap, "min"),
				Max:       utils.GetIntField(vMap, "max"),
			}

			if values, ok := vMap["values"].([]interface{}); ok {
				variable.Values = utils.ToStringSlice(values)
			}

			vars = append(vars, variable)
		}
	}
	return vars
}

func mapWizards(wizards []interface{}) (domain.Lifecycle, map[string]domain.Method) {
	lifecycle := domain.Lifecycle{}
	methods := map[string]domain.Method{}

	for _, wizard := range wizards {
		wizardMap, ok := wizard.(map[string]interface{})
		if !ok {
			continue
		}

		wizardMethods, ok := wizardMap["methods"].([]interface{})
		if !ok {
			continue
		}

		for _, method := range wizardMethods {
			methodMap, ok := method.(map[string]interface{})
			if !ok {
				continue
			}

			methodName := utils.GetStringField(methodMap, "method")
			steps := mapActions(utils.GetSliceField(methodMap, "actions"))

			switch methodName {
			case "install":
				lifecycle.Install = append(lifecycle.Install, steps...)
			case "execute", "run":
				lifecycle.Execute = append(lifecycle.Execute, steps...)
			case "stop":
				lifecycle.Stop = append(lifecycle.Stop, steps...)
			case "uninstall":
				lifecycle.Uninstall = append(lifecycle.Uninstall, steps...)
			default:
				methods[methodName] = domain.Method{Steps: steps}
			}
		}
	}

	return lifecycle, methods
}

func mapActions(actionsList []interface{}) []step.Step {
	var steps []step.Step

	for _, action := range actionsList {
		actionMap, ok := action.(map[string]interface{})
		if !ok {
			continue
		}

		name := utils.GetStringField(actionMap, "name")
		exitOnFailure := utils.GetBoolField(actionMap, "exit_on_failure")
		to := utils.GetStringField(actionMap, "to")

		if runCmd := utils.GetStringField(actionMap, "run"); runCmd != "" {
			steps = append(steps, step.NewRunStep(name, runCmd, 0, exitOnFailure))
		} else if download := utils.GetStringField(actionMap, "download"); download != "" {
			steps = append(steps, step.NewFetchStep(name, download, to, 0, exitOnFailure))
		} else if copySrc := utils.GetStringField(actionMap, "copy"); copySrc != "" {
			cmd := fmt.Sprintf("cp %s %s", copySrc, to)
			steps = append(steps, step.NewRunStep(name, cmd, 0, exitOnFailure))
		} else if uncompress := utils.GetStringField(actionMap, "uncompress"); uncompress != "" {
			cmd := fmt.Sprintf("tar -xf %s -C %s", uncompress, to)
			steps = append(steps, step.NewRunStep(name, cmd, 0, exitOnFailure))
		} else {
			steps = append(steps, step.NewRunStep(name, "", 0, exitOnFailure))
		}
	}

	return steps
}
