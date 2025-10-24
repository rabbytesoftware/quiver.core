package operators

import (
	"context"

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

func (c *OperatorContainer) Obtain(
	ctx context.Context, 
) interfaces.OperatorInterface {
	for _, operator := range c.operators {
		available, err := operator.IsAvailable(ctx)
		if err != nil {
			continue
		}

		if available {
			return operator
		}
	}

	return nil
}
