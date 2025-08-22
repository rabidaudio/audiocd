package audiocd

import (
	"fmt"
	"io/fs"
)

// ErrNoDrive is returned when no valid cd drive was found.
var ErrNoDrive = fs.ErrNotExist
var ErrNotOpen = fs.ErrClosed

// Errors returned while reading audio data.
type AudioCDError int

const (
	ErrDriverOperationFailed       AudioCDError = -1
	ErrDriverOperationUnsupported  AudioCDError = -2
	ErrDriverUninitialized         AudioCDError = -3
	ErrDriverOperationNotPermitted AudioCDError = -4
	ErrDriverBadParameter          AudioCDError = -5
	ErrDriverBadPointer            AudioCDError = -6
	ErrDriverInvalidDriver         AudioCDError = -7
	ErrDriverMMCSenseData          AudioCDError = -8
)

func (pe AudioCDError) Error() string {
	return fmt.Sprintf("audiocd: %v", driverErrMsg(pe))
}
