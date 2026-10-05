package dto

// ConsoleCommandsDTO is the response of the console's command list: every
// command the console may run and no other, in the order cobra lists them.
type ConsoleCommandsDTO struct {
	Commands []ConsoleCommandDTO `json:"commands" yaml:"commands"`
}
