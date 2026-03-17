package v1

import (
	_ "embed"
	"fmt"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/utils"
)

//go:embed schema.json
var schemaJSON []byte

type ArrowV1Mapper struct{}

func NewMapper() *ArrowV1Mapper {
	return &ArrowV1Mapper{}
}

func (m *ArrowV1Mapper) Map(yamlData map[string]interface{}) (*domain.Arrow, error) {
	arrowModel := &domain.Arrow{
		ID: uuid.New(),
	}

	if metadata, ok := yamlData["metadata"].(map[string]interface{}); ok {
		arrowModel.Name = utils.GetStringField(metadata, "name")
		arrowModel.Description = utils.GetStringField(metadata, "description")
		arrowModel.Version = utils.GetStringField(metadata, "version")
		arrowModel.License = utils.GetStringField(metadata, "license")
		arrowModel.QuiverURL = domain.URL(utils.GetStringField(metadata, "quiver"))

		if media, ok := metadata["media"].(map[string]interface{}); ok {
			arrowModel.IconURL = domain.URL(utils.GetStringField(media, "icon"))
			arrowModel.BannerURL = domain.URL(utils.GetStringField(media, "banner"))
		}

		if credits, ok := metadata["credits"].([]interface{}); ok {
			for _, credit := range credits {
				if c, ok := credit.(map[string]interface{}); ok {
					arrowModel.Credits = append(arrowModel.Credits, utils.GetStringField(c, "name"))
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

	if wizards, ok := yamlData["wizards"].([]interface{}); ok {
		arrowModel.Methods = mapWizards(wizards)
	}

	if err := arrowModel.Validate(); err != nil {
		return nil, fmt.Errorf("arrow validation failed: %w", err)
	}

	return arrowModel, nil
}

func (m *ArrowV1Mapper) GetSchema() ([]byte, error) {
	return schemaJSON, nil
}

func mapRequirements(req map[string]interface{}) domain.Requirement {
	r := domain.Requirement{
		CpuCores:    utils.GetIntField(req, "cpu_cores"),
		Memory:      utils.GetIntField(req, "ram_gb"),
		Disk:        utils.GetIntField(req, "disk_gb"),
		NetworkMbps: utils.GetIntField(req, "network_mbps"),
	}

	if systems, ok := req["system"].([]interface{}); ok && len(systems) > 0 {
		if firstSys, ok := systems[0].(string); ok {
			r.OS = domain.OS(firstSys)
		}
	}

	return r
}

func mapNetbridge(netbridgeData []interface{}) []domain.PortRule {
	rules := []domain.PortRule{}
	for _, nb := range netbridgeData {
		if nbMap, ok := nb.(map[string]interface{}); ok {
			rule := domain.PortRule{
				ID: utils.GetStringField(nbMap, "name"),
			}
			protocolStr := utils.GetStringField(nbMap, "protocol")
			switch protocolStr {
			case "tcp":
				rule.Protocol = domain.ProtocolTCP
			case "udp":
				rule.Protocol = domain.ProtocolUDP
			case "tcp/udp":
				rule.Protocol = domain.ProtocolTCPUDP
			}
			rules = append(rules, rule)
		}
	}
	return rules
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

func mapWizards(wizards []interface{}) []domain.Method {
	var methods []domain.Method

	for _, wizard := range wizards {
		wizardMap, ok := wizard.(map[string]interface{})
		if !ok {
			continue
		}

		platforms := utils.ToStringSlice(utils.GetSliceField(wizardMap, "platforms"))
		dependencies := utils.ToStringSlice(utils.GetSliceField(wizardMap, "dependencies"))
		workdir := utils.GetStringField(wizardMap, "workdir")

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
			actions := mapActions(utils.GetSliceField(methodMap, "actions"))

			methods = append(methods, domain.Method{
				Platforms:    platforms,
				Dependencies: dependencies,
				Workdir:      workdir,
				MethodName:   methodName,
				Actions:      actions,
			})
		}
	}

	return methods
}

func mapActions(actionsList []interface{}) []domain.Action {
	var actions []domain.Action

	for _, action := range actionsList {
		actionMap, ok := action.(map[string]interface{})
		if !ok {
			continue
		}

		act := domain.Action{
			Name:          utils.GetStringField(actionMap, "name"),
			To:            utils.GetStringField(actionMap, "to"),
			ExitOnFailure: utils.GetBoolField(actionMap, "exit_on_failure"),
			Timeout:       utils.GetStringField(actionMap, "timeout"),
		}

		if runCmd := utils.GetStringField(actionMap, "run"); runCmd != "" {
			act.Type = domain.ActionTypeRun
			act.Value = runCmd
		} else if download := utils.GetStringField(actionMap, "download"); download != "" {
			act.Type = domain.ActionTypeDownload
			act.Value = download
		} else if copy := utils.GetStringField(actionMap, "copy"); copy != "" {
			act.Type = domain.ActionTypeCopy
			act.Value = copy
		} else if uncompress := utils.GetStringField(actionMap, "uncompress"); uncompress != "" {
			act.Type = domain.ActionTypeUncompress
			act.Value = uncompress
		}

		actions = append(actions, act)
	}

	return actions
}
