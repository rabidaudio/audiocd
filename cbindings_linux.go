//go:build linux

package audiocd

// TODO: should we link statically instead??

// #cgo LDFLAGS: -lcdda_interface -lcdda_paranoia
// #include <stdint.h>
// #include <stdlib.h>
// #include <linux/major.h>
// #include <cdda_interface.h>
// #include <cdda_paranoia.h>
//
// /* Calling C function pointers from Go is not supported,
//    but this is a workaround. See https://pkg.go.dev/cmd/cgo */
// typedef int (*set_speed_fn) (struct cdrom_drive *d, int speed);
// int bridge_set_speed(set_speed_fn f, struct cdrom_drive *d, int speed) {
//   return f(d, speed);
// }
import "C"

import (
	"fmt"
	"io"
	"log"
	"strings"
	"unsafe"
)

const (
	GENERIC_SCSI     InterfaceType = C.GENERIC_SCSI
	COOKED_IOCTL     InterfaceType = C.COOKED_IOCTL
	TEST_INTERFACE   InterfaceType = C.TEST_INTERFACE
	SGIO_SCSI        InterfaceType = C.SGIO_SCSI
	SGIO_SCSI_BUGGY1 InterfaceType = C.SGIO_SCSI_BUGGY1
)

const (
	IDE0_MAJOR DriveType = C.IDE0_MAJOR
	IDE1_MAJOR DriveType = C.IDE1_MAJOR
	IDE2_MAJOR DriveType = C.IDE2_MAJOR
	IDE3_MAJOR DriveType = C.IDE3_MAJOR
	IDE4_MAJOR DriveType = C.IDE4_MAJOR
	IDE5_MAJOR DriveType = C.IDE5_MAJOR
	IDE6_MAJOR DriveType = C.IDE6_MAJOR
	IDE7_MAJOR DriveType = C.IDE7_MAJOR
	IDE8_MAJOR DriveType = C.IDE8_MAJOR
	IDE9_MAJOR DriveType = C.IDE9_MAJOR

	CDU31A_CDROM_MAJOR DriveType = C.CDU31A_CDROM_MAJOR

	CDU535_CDROM_MAJOR DriveType = C.CDU535_CDROM_MAJOR

	MATSUSHITA_CDROM_MAJOR  DriveType = C.MATSUSHITA_CDROM_MAJOR
	MATSUSHITA_CDROM2_MAJOR DriveType = C.MATSUSHITA_CDROM2_MAJOR
	MATSUSHITA_CDROM3_MAJOR DriveType = C.MATSUSHITA_CDROM3_MAJOR
	MATSUSHITA_CDROM4_MAJOR DriveType = C.MATSUSHITA_CDROM4_MAJOR

	SANYO_CDROM_MAJOR DriveType = C.SANYO_CDROM_MAJOR

	MITSUMI_CDROM_MAJOR   DriveType = C.MITSUMI_CDROM_MAJOR
	MITSUMI_X_CDROM_MAJOR DriveType = C.MITSUMI_X_CDROM_MAJOR

	OPTICS_CDROM_MAJOR DriveType = C.OPTICS_CDROM_MAJOR

	AZTECH_CDROM_MAJOR DriveType = C.AZTECH_CDROM_MAJOR

	GOLDSTAR_CDROM_MAJOR DriveType = C.GOLDSTAR_CDROM_MAJOR

	CM206_CDROM_MAJOR DriveType = C.CM206_CDROM_MAJOR

	SCSI_CDROM_MAJOR   DriveType = C.SCSI_CDROM_MAJOR
	SCSI_GENERIC_MAJOR DriveType = C.SCSI_GENERIC_MAJOR
)

func openDrive(cd *AudioCD) error {
	logLevel, logFlush := prepareLogs(cd.LogMode, cd.Logger)
	var p *C.char
	defer logFlush(unsafe.Pointer(p))

	var drive *C.cdrom_drive
	if cd.Device == "" {
		drive = C.cdda_find_a_cdrom(logLevel, &p)
	} else {
		str := C.CString(cd.Device)
		defer C.free(unsafe.Pointer(str))
		drive = C.cdda_identify(str, logLevel, &p)
	}

	if drive == nil {
		return ErrNoDrive
	}

	if err, ok := parseError(C.cdda_open(drive)); !ok {
		return err
	}
	cd.drive = unsafe.Pointer(drive)
	cd.paranoia = C.paranoia_init(drive)
	return nil
}

func model(drive unsafe.Pointer) string {
	return C.GoString((*C.cdrom_drive)(drive).drive_model)
}

func driveType(drive unsafe.Pointer) DriveType {
	return DriveType((*C.cdrom_drive)(drive).drive_type)
}

func interfaceType(drive unsafe.Pointer) InterfaceType {
	return InterfaceType((*C.cdrom_drive)(drive)._interface)
}

func trackCount(d unsafe.Pointer) int {
	return int((*C.cdrom_drive)(d).tracks)
}

func firstAudioSector(d unsafe.Pointer) int {
	return int((*C.cdrom_drive)(d).audio_first_sector)
}

func toc(d unsafe.Pointer, ntracks int) []TrackPosition {
	drive := (*C.cdrom_drive)(d)
	ctoc := drive.disc_toc

	// NOTE: the end of the last track is the first sector
	// of the imaginary track after
	toc := make([]TrackPosition, ntracks+1)
	audiolen := toc[len(toc)-1].StartSector

	for i := range toc {
		toc[i].Flags = byte(ctoc[i].bFlags)
		toc[i].TrackNum = int(ctoc[i].bTrack)
		toc[i].StartSector = int(ctoc[i].dwStartSector)
	}

	// compute lengths
	if len(toc) == 1 {
		toc[0].LengthSectors = audiolen
	} else {
		for i := range toc[1:] {
			toc[i].LengthSectors = toc[i+1].StartSector - toc[i].StartSector
		}
	}
	return toc[:ntracks]
}

func lengthSectors(d unsafe.Pointer) int {
	drive := (*C.cdrom_drive)(d)
	return int(drive.disc_toc[int(drive.tracks)].dwStartSector)
}

func opened(d unsafe.Pointer) bool {
	return int((*C.cdrom_drive)(d).opened) != 0
}

func setParanoia(cd *AudioCD, flags ParanoiaFlags) {
	defer flushLogs(cd)
	C.paranoia_modeset(cd.paranoia, C.int(flags))
}

func overlapSet(cd *AudioCD, sectors int) {
	defer flushLogs(cd)
	C.paranoia_overlapset(cd.paranoia, C.long(sectors))
}

func setSpeed(cd *AudioCD, x int) error {
	defer flushLogs(cd)
	drive := (*C.cdrom_drive)(cd.drive)
	err, _ := parseError(C.bridge_set_speed(drive.set_speed, drive, C.int(x)))
	return err
}

func seekSector(cd *AudioCD, sector int) error {
	defer flushLogs(cd)

	res := int64(C.paranoia_seek(cd.paranoia, C.long(sector), C.int(io.SeekStart)))
	if res < 0 {
		return AudioCDError(-1 * res)
	}
	return nil
}

func readLimited(cd *AudioCD, p []byte, retries int) error {
	// TODO: expose callback? may not be possible
	buf := unsafe.Pointer(C.paranoia_read_limited(cd.paranoia, nil, C.int(retries)))
	// run logs and check for errors
	err := flushLogs(cd)
	if err != nil {
		return err
	}
	if buf == nil {
		return fmt.Errorf("audiocd: unknown error")
	}

	res := C.GoBytes(buf, C.int(BytesPerSector))
	// copy data into provided buffer, since paranoia will reclaim buffer
	copy(p, res)
	return nil
}

func closeDrive(d unsafe.Pointer) {
	C.cdda_close((*C.cdrom_drive)(d))
}

func paranoiaFree(p unsafe.Pointer) {
	C.paranoia_free(p)
}

func version() string {
	return C.GoString(C.paranoia_version())
}

func parseError(retval C.int) (err error, ok bool) {
	if retval == 0 {
		return nil, true
	}
	i := int(retval)
	if i < 0 {
		i = -1 * i
	}
	return AudioCDError(i), false
}

func prepareLogs(lm LogMode, logger *log.Logger) (C.int, func(unsafe.Pointer)) {
	nopLogFlush := func(p unsafe.Pointer) {}
	switch lm {
	case LogModeStdErr:
		return C.int(LogModeStdErr), nopLogFlush
	case LogModeLogger:
		if logger != nil {
			return C.int(LogModeLogger), func(p unsafe.Pointer) {
				if logger != nil && p != nil {
					str := C.GoString((*C.char)(p))
					for line := range strings.Lines(str) {
						logger.Print(line)
					}
					C.free(p)
				}
			}
		}
	}
	return C.int(LogModeSilent), nopLogFlush
}

func flushLogs(cd *AudioCD) (err error) {
	drive := (*C.cdrom_drive)(cd.drive)

	errstring := C.cdda_errors(drive)
	if errstring != nil {
		err = fmt.Errorf("audiocd: %v", C.GoString(errstring))
	}

	logger := cd.Logger
	if cd.LogMode != LogModeLogger || logger == nil {
		return
	}

	if errstring != nil {
		for line := range strings.Lines(C.GoString(errstring)) {
			logger.Print(line)
		}
	}

	msgstring := C.cdda_messages(drive)
	if msgstring != nil {
		for line := range strings.Lines(C.GoString(msgstring)) {
			logger.Print(line)
		}
	}
	return
}

func (dt DriveType) String() string {
	switch dt {
	case IDE0_MAJOR, IDE1_MAJOR, IDE2_MAJOR, IDE3_MAJOR, IDE4_MAJOR, IDE5_MAJOR, IDE6_MAJOR, IDE7_MAJOR, IDE8_MAJOR, IDE9_MAJOR:
		return "ATAPI"

	case CDU31A_CDROM_MAJOR:
		return "Sony CDU31A or compatible"

	case CDU535_CDROM_MAJOR:
		return "Sony CDU535 or compatible"

	case MATSUSHITA_CDROM_MAJOR, MATSUSHITA_CDROM2_MAJOR, MATSUSHITA_CDROM3_MAJOR, MATSUSHITA_CDROM4_MAJOR:
		return "non-ATAPI IDE-style Matsushita/Panasonic CR-5xx or compatible"

	case SANYO_CDROM_MAJOR:
		return "Sanyo proprietary or compatible: NOT CDDA CAPABLE"

	case MITSUMI_CDROM_MAJOR, MITSUMI_X_CDROM_MAJOR:
		return "Mitsumi proprietary or compatible: NOT CDDA CAPABLE"

	case OPTICS_CDROM_MAJOR:
		return "Optics Dolphin or compatible: NOT CDDA CAPABLE"

	case AZTECH_CDROM_MAJOR:
		return "Aztech proprietary or compatible: NOT CDDA CAPABLE"

	case GOLDSTAR_CDROM_MAJOR:
		return "Goldstar proprietary: NOT CDDA CAPABLE"

	case CM206_CDROM_MAJOR:
		return "Philips/LMS CM206 proprietary: NOT CDDA CAPABLE"

	case SCSI_CDROM_MAJOR, SCSI_GENERIC_MAJOR:
		return "SCSI CDROM"

	default:
		return "unknown"
	}
}
