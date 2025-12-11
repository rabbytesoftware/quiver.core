package netbridge

type ForwardingStatus string

const (
	ForwardingStatusEnabled  ForwardingStatus = "enabled"
	ForwardingStatusDisabled ForwardingStatus = "disabled"
	ForwardingStatusError    ForwardingStatus = "error"
)

func (f ForwardingStatus) String() string {
	return string(f)
}

func (f ForwardingStatus) IsEnabled() bool {
	return f == ForwardingStatusEnabled
}

func (f ForwardingStatus) IsDisabled() bool {
	return f == ForwardingStatusDisabled
}

func (f ForwardingStatus) IsError() bool {
	return f == ForwardingStatusError
}
