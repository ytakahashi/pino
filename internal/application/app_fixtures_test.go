package application

// otherAction is an Action this layer has no handling for.
type otherAction struct{}

func (otherAction) isAction() {}
