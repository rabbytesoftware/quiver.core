package operators

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/holepunch"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/natpmp"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/pcp"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/upnp"
)

type OperatorContainer struct {
	operators map[string]interfaces.OperatorInterface
	order     []string
}

func NewOperatorContainer() *OperatorContainer {
	operators := &OperatorContainer{
		operators: make(map[string]interfaces.OperatorInterface),
		order:     []string{},
	}

	operators.Add(upnp.NewUPnPOperator())
	operators.Add(natpmp.NewNATPMP())
	operators.Add(pcp.NewPCP())
	operators.Add(holepunch.NewHolePunch())

	return operators
}

func NewEmptyOperatorContainer() *OperatorContainer {
	return &OperatorContainer{
		operators: make(map[string]interfaces.OperatorInterface),
		order:     []string{},
	}
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
	name := operator.Name()
	if _, exists := c.operators[name]; exists {
		c.operators[name] = operator
		return
	}

	c.operators[name] = operator
	c.order = append(c.order, name)
}

func (c *OperatorContainer) Obtain(
	ctx context.Context,
) interfaces.OperatorInterface {
	return c.ObtainFrom(ctx, nil)
}

func (c *OperatorContainer) ObtainFrom(
	ctx context.Context,
	priority []string,
) interfaces.OperatorInterface {
	for _, operator := range c.orderedOperators(priority) {
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

func (c *OperatorContainer) AvailableNames(
	ctx context.Context,
) []string {
	available := []string{}
	for _, operator := range c.orderedOperators(nil) {
		isAvailable, err := operator.IsAvailable(ctx)
		if err != nil {
			continue
		}

		if isAvailable {
			available = append(available, operator.Name())
		}
	}

	return available
}

func (c *OperatorContainer) orderedOperators(
	priority []string,
) []interfaces.OperatorInterface {
	if len(priority) > 0 {
		ordered := make([]interfaces.OperatorInterface, 0, len(priority))
		for _, name := range priority {
			if operator, ok := c.operators[name]; ok {
				ordered = append(ordered, operator)
			}
		}
		return ordered
	}

	ordered := make([]interfaces.OperatorInterface, 0, len(c.order))
	for _, name := range c.order {
		operator, ok := c.operators[name]
		if ok {
			ordered = append(ordered, operator)
		}
	}
	return ordered
}
