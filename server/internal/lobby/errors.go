package lobby

import "errors"

var (
	errBlindsPositive  = errors.New("smallBlind and bigBlind must be greater than 0")
	errBigBlindGreater = errors.New("bigBlind must be greater than smallBlind")
)
