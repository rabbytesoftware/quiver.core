package networking

type Rule struct {
	Protocol         Protocol         `json:"protocol"`
	ForwardingStatus ForwardingStatus `json:"forwarding_status"`
}

func (r *Rule) IsValid() bool {
	return r.Protocol.IsValid() && r.ForwardingStatus.IsValid()
}
