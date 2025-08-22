package audiocd

// TODO: should we link statically instead??

// #cgo LDFLAGS: -lcdio
// #include <stdint.h>
// #include <stdlib.h>
// #include <cdio/cdio.h>
import "C"
import (
	"fmt"
	"strings"
	"unsafe"
)

func openDrive(cd *AudioCD) error {
	var p_cdio *C.CdIo_t

	if cd.Device == "" {
		p_cdio = C.cdio_open(nil, C.cdio_os_driver)
	} else {
		str := C.CString(cd.Device)
		// defer C.free(unsafe.Pointer(str)) // handled by lib
		p_cdio = C.cdio_open(str, C.cdio_os_driver)
	}

	if p_cdio == nil {
		// TODO: is it really a no-drive error or something else?
		return ErrNoDrive
	}
	cd.cdio = unsafe.Pointer(p_cdio)
	return nil
}

func model(cdio unsafe.Pointer) string {
	var hwinfo C.cdio_hwinfo_t
	ok := bool(C.cdio_get_hwinfo((*C.CdIo_t)(cdio), &hwinfo))
	if !ok {
		return ""
	}
	return strings.Join([]string{
		C.GoStringN(&hwinfo.psz_vendor[0], C.int(unsafe.Sizeof(hwinfo.psz_vendor))),
		C.GoStringN(&hwinfo.psz_model[0], C.int(unsafe.Sizeof(hwinfo.psz_model))),
		C.GoStringN(&hwinfo.psz_revision[0], C.int(unsafe.Sizeof(hwinfo.psz_revision))),
	}, " ")
}

func trackCount(cdio unsafe.Pointer) int {
	return int(C.cdio_get_num_tracks((*C.CdIo_t)(cdio)))
}

func toc(cdio unsafe.Pointer, ntracks int) ([]TrackPosition, error) {
	p := (*C.CdIo_t)(cdio)

	toc := make([]TrackPosition, ntracks)

	t1 := int(C.cdio_get_first_track_num(p))
	if t1 == C.CDIO_INVALID_TRACK {
		// TODO: should we assume "1" instead?
		return nil, fmt.Errorf("libcdio: CDIO_INVALID_TRACK")
	}

	for i := range ntracks {
		tt := C.track_t(t1 + i)
		toc[i].TrackNum = t1 + i
		toc[i].NumChannels = int(C.cdio_get_track_channels(p, tt))
		toc[i].IsCopyPermitted = C.cdio_get_track_copy_permit(p, tt) == C.CDIO_TRACK_FLAG_TRUE
		toc[i].IsPreemphasisEnabled = C.cdio_get_track_preemphasis(p, tt) == C.CDIO_TRACK_FLAG_TRUE
		toc[i].IsAudio = C.cdio_get_track_format(p, tt) == C.TRACK_FORMAT_AUDIO

		toc[i].StartSector = int(C.cdio_get_track_lsn(p, tt))
		if toc[i].StartSector == C.CDIO_INVALID_LSN {
			return nil, fmt.Errorf("libcdio: CDIO_INVALID_LSN")
		}
		// includes pregap at end
		toc[i].LengthSectors = int(C.cdio_get_track_sec_count(p, tt))
		if toc[i].LengthSectors == 0 {
			return nil, fmt.Errorf("libcdio: CDIO_INVALID_LSN")
		}

		//cdio_get_track_isrc
	}
	return toc, nil
}

func setSpeed(cd *AudioCD, x int) error {
	cdio := (*C.CdIo_t)(cd.cdio)
	err := int(C.cdio_set_speed(cdio, C.int(x)))
	if err != 0 {
		return AudioCDError(err)
	}
	return nil
}

func driverErrMsg(err AudioCDError) string {
	return C.GoString(C.cdio_driver_errmsg(C.driver_return_code_t(err)))
}

func readAudioSectors(cd *AudioCD, p []byte, nsectors int) error {
	if len(p) != nsectors*BytesPerSector {
		return fmt.Errorf("audiocd: invalid buffer size")
	}
	start := uint32(cd.trueOffset / BytesPerSector)
	err := C.cdio_read_audio_sectors((*C.CdIo_t)(cd.cdio), unsafe.Pointer(&p[0]), C.lsn_t(start), C.uint(nsectors))
	if err != 0 {
		return AudioCDError(err)
	}
	return nil
}

func closeTray(device string) error {
	var err C.driver_return_code_t
	if device == "" {
		err = C.cdio_close_tray(nil, nil)
	} else {
		str := C.CString(device)
		// defer C.free(unsafe.Pointer(str)) // handled by lib
		err = C.cdio_close_tray(str, nil)
	}
	if err != 0 {
		return AudioCDError(err)
	}
	return nil
}

func ejectMedia(cd *AudioCD) error {
	cdio := (*C.CdIo_t)(cd.cdio)
	err := C.cdio_eject_media(&cdio)
	if err != 0 {
		return AudioCDError(err)
	}
	cd.cdio = nil
	return nil
}

func closeDrive(cdio unsafe.Pointer) {
	C.cdio_destroy((*C.CdIo_t)(cdio))
}

func version() string {
	return C.CDIO_VERSION
}
