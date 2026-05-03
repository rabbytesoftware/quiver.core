package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

type ServiceConsumerLifecycleRule struct{}

func (ServiceConsumerLifecycleRule) Name() string { return "service_consumer_lifecycle" }

func (ServiceConsumerLifecycleRule) Validate(m *domain.Arrow) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	for os, target := range m.Targets {
		errs = append(errs, checkServiceConsumerLifecycle(string(os), target)...)
	}
	return errs
}

func checkServiceConsumerLifecycle(key string, target domain.Target) aerrors.RuleErrors {
	if len(target.Services) == 0 {
		return nil
	}
	hasExecute := len(target.Lifecycle.Execute) > 0
	hasStop := len(target.Lifecycle.Stop) > 0
	if hasExecute && hasStop {
		return nil
	}
	return aerrors.RuleErrors{aerrors.RuleError{
		Field:   fmt.Sprintf("targets[%s].lifecycle", key),
		Rule:    "service_consumer_missing_lifecycle",
		Message: "arrows that declare service dependencies must define execute and stop lifecycle steps",
	}}
}
