package audiocd

import (
	"fmt"
	"io/fs"
)

// ErrNoDrive is returned when no valid cd drive was found.
var ErrNoDrive = fs.ErrNotExist

// Errors returned while reading audio data.
type AudioCDError int

const (
	ErrSetReadAudioMode      AudioCDError = 1
	ErrReadTOCLeadOut        AudioCDError = 2
	ErrIllegalNumberOfTracks AudioCDError = 3
	ErrReadTOCHeader         AudioCDError = 4
	ErrReadTOCEntry          AudioCDError = 5
	ErrNoData                AudioCDError = 6
	ErrUnknownReadError      AudioCDError = 7
	ErrUnableToIdentifyModel AudioCDError = 8
	ErrIllegalTOC            AudioCDError = 9
	ErrInterfaceNotSupported AudioCDError = 100
	ErrPermissionDenied      AudioCDError = 102
	ErrKernelMemory          AudioCDError = 300
	ErrNotOpen               AudioCDError = 400
	ErrInvalidTrackNumber    AudioCDError = 401
	ErrNoAudioTracks         AudioCDError = 403
	ErrNoMediumPresent       AudioCDError = 404
	ErrOperationNotSupported AudioCDError = 405
)

func (pe AudioCDError) Error() string {
	return fmt.Sprintf("audiocd: %v", pe.name())
}

func (pe AudioCDError) name() string {
	switch pe {
	case ErrSetReadAudioMode:
		return "unable to set CDROM to read audio mode"
	case ErrReadTOCLeadOut:
		return "unable to read table of contents lead-out"
	case ErrIllegalNumberOfTracks:
		return "cdrom reporting illegal number of tracks"
	case ErrReadTOCHeader:
		return "unable to read table of contents header"
	case ErrReadTOCEntry:
		return "unable to read table of contents entry"
	case ErrNoData:
		return "could not read any data from drive"
	case ErrUnknownReadError:
		return "unknown, unrecoverable error reading data"
	case ErrUnableToIdentifyModel:
		return "unable to identify CDROM model"
	case ErrIllegalTOC:
		return "cdrom reporting illegal table of contents"

	case ErrInterfaceNotSupported:
		return "interface not supported"
	case ErrPermissionDenied:
		return "permision denied on cdrom (ioctl) device"

	case ErrKernelMemory:
		return "kernel memory error"

	case ErrNotOpen:
		return "device not open"
	case ErrInvalidTrackNumber:
		return "invalid track number"
	case ErrNoAudioTracks:
		return "no audio tracks on disc"
	case ErrNoMediumPresent:
		return "no medium present"
	case ErrOperationNotSupported:
		return "option not supported by drive"
	default:
		return fmt.Sprintf("unknown error code: %v", int(pe))
	}
}
