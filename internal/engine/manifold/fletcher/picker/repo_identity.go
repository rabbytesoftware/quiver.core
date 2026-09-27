package picker

import (
	"slices"
	"strings"
)

type repoIdentity struct {
	name    string
	product string
	tokens  []string
}

func (p *picker) identify(
	repo string,
) repoIdentity {
	name := strings.ToLower(repo[strings.LastIndex(repo, "/")+1:])
	return repoIdentity{
		name:    name,
		product: p.normalize(name),
		tokens:  p.separator.Split(name, -1),
	}
}

func (id repoIdentity) accepts(
	c classification,
) bool {
	for _, companion := range c.companions {
		if !slices.Contains(id.tokens, companion) {
			return false
		}
	}
	return true
}

func (id repoIdentity) owns(
	c classification,
) bool {
	return id.product != "" && c.product == id.product
}

func (id repoIdentity) extends(
	c classification,
) bool {
	return id.product != "" && strings.HasPrefix(c.product, id.product)
}

func (id repoIdentity) mentionedIn(
	c classification,
) bool {
	return strings.Contains(strings.ToLower(c.asset.Name), id.name)
}

func (id repoIdentity) extras(
	c classification,
) int {
	return len(c.product) - len(id.product)
}
