package action

// Action represents an action to be executed
type Action struct {
	ActionSequenceID int    `json:"actionSequenceID"`
	Action           string `json:"action"`
	Coordinates      struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"coordinates,omitempty"`
	Duration     int                           `json:"duration,omitempty"`
	InputString  string                        `json:"inputString,omitempty"`
	KeyTapString string                        `json:"keyTapString,omitempty"`
	KeyString    string                        `json:"keyString,omitempty"`
	ActionsRange []int                         `json:"actionsRange,omitempty"`
	RepeatTimes  int                           `json:"repeatTimes,omitempty"`
	Parameters   interface{}                   `json:"parameters,omitempty"`
	Description  string                        `json:"description,omitempty"`
	Execute      func(*Action, ...interface{}) `json:"-"`
}
