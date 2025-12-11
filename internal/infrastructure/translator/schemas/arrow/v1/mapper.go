package v1

import (
	_ "embed"
	"fmt"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/netbridge"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/rabbytesoftware/quiver/internal/utils"
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
		arrowModel.Name = utils.GetStringField(metadata, "name")
		arrowModel.Description = utils.GetStringField(metadata, "description")
		arrowModel.Version = utils.GetStringField(metadata, "version")
		arrowModel.License = utils.GetStringField(metadata, "license")
		arrowModel.QuiverURL = shared.URL(utils.GetStringField(metadata, "quiver"))

		if media, ok := metadata["media"].(map[string]interface{}); ok {
			arrowModel.IconURL = shared.URL(utils.GetStringField(media, "icon"))
			arrowModel.BannerURL = shared.URL(utils.GetStringField(media, "banner"))
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

func mapRequirements(req map[string]interface{}) arrow.Requirement {
	r := arrow.Requirement{
		CpuCores:    utils.GetIntField(req, "cpu_cores"),
		Memory:      utils.GetIntField(req, "ram_gb"),
		Disk:        utils.GetIntField(req, "disk_gb"),
		NetworkMbps: utils.GetIntField(req, "network_mbps"),
	}

	if systems, ok := req["system"].([]interface{}); ok && len(systems) > 0 {
		if firstSys, ok := systems[0].(string); ok {
			r.OS = shared.OS(firstSys)
		}
	}

	return r
}

func mapNetbridge(netbridgeData []interface{}) []netbridge.PortRule {
	rules := []netbridge.PortRule{}
	for _, nb := range netbridgeData {
		if nbMap, ok := nb.(map[string]interface{}); ok {
			rule := netbridge.PortRule{
				ID: utils.GetStringField(nbMap, "name"),
			}
			protocolStr := utils.GetStringField(nbMap, "protocol")
			switch protocolStr {
			case "tcp":
				rule.Protocol = netbridge.ProtocolTCP
			case "udp":
				rule.Protocol = netbridge.ProtocolUDP
			case "tcp/udp":
				rule.Protocol = netbridge.ProtocolTCPUDP
			}
			rules = append(rules, rule)
		}
	}
	return rules
}

func mapVariables(variables []interface{}) []arrow.Variable {
	vars := []arrow.Variable{}
	for _, v := range variables {
		if vMap, ok := v.(map[string]interface{}); ok {
			variable := arrow.Variable{
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

func mapWizards(wizards []interface{}) []arrow.Method {
	var methods []arrow.Method

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

			methods = append(methods, arrow.Method{
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

func mapActions(actionsList []interface{}) []arrow.Action {
	var actions []arrow.Action

	for _, action := range actionsList {
		actionMap, ok := action.(map[string]interface{})
		if !ok {
			continue
		}

		act := arrow.Action{
			Name:          utils.GetStringField(actionMap, "name"),
			To:            utils.GetStringField(actionMap, "to"),
			ExitOnFailure: utils.GetBoolField(actionMap, "exit_on_failure"),
			Timeout:       utils.GetStringField(actionMap, "timeout"),
		}

		if runCmd := utils.GetStringField(actionMap, "run"); runCmd != "" {
			act.Type = arrow.ActionTypeRun
			act.Value = runCmd
		} else if download := utils.GetStringField(actionMap, "download"); download != "" {
			act.Type = arrow.ActionTypeDownload
			act.Value = download
		} else if copy := utils.GetStringField(actionMap, "copy"); copy != "" {
			act.Type = arrow.ActionTypeCopy
			act.Value = copy
		} else if uncompress := utils.GetStringField(actionMap, "uncompress"); uncompress != "" {
			act.Type = arrow.ActionTypeUncompress
			act.Value = uncompress
		}

		actions = append(actions, act)
	}

	return actions
}
