package operators

import (
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/upnp"
)

type OperatorContainer struct {
	operators map[string]interfaces.OperatorInterface
}

func NewOperatorContainer() *OperatorContainer {
	operators := &OperatorContainer{
		operators: make(map[string]interfaces.OperatorInterface),
	}

	operators.Add(upnp.NewUPnPOperator())
	
	return operators
}

func (c *OperatorContainer) All() map[string]interfaces.OperatorInterface {
	return c.operators
}

func (c *OperatorContainer) Get(
	name string,
) interfaces.OperatorInterface {
	return c.operators[name]
}

func (c *OperatorContainer) Add(
	operator interfaces.OperatorInterface,
) {
	c.operators[operator.Name()] = operator
}
