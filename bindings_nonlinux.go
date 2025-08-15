//go:build !linux

package audiocd

import (
	"crypto/rand"
	"fmt"
	"os"
	"unsafe"
)

func init() {
	fmt.Fprintln(os.Stderr, "NOTE: audiocd is only supported on linux. You are operating on a mock implementation for testing which returns white noise.")
}

func openDrive(cd *AudioCD) error {
	// pretend to be open by keeping a pointer to self
	cd.drive = unsafe.Pointer(cd)
	return nil
}

func model(drive unsafe.Pointer) string {
	return "Mock AudioCD implementation"
}

func driveType(drive unsafe.Pointer) DriveType {
	return 0
}

func interfaceType(drive unsafe.Pointer) InterfaceType {
	return 0
}

func trackCount(d unsafe.Pointer) int {
	return 10
}

func firstAudioSector(d unsafe.Pointer) int {
	return 0
}

func toc(d unsafe.Pointer, ntracks int) []TrackPosition {
	tp := make([]TrackPosition, 10)
	len := SectorsPerSecond * 3 * 60
	pos := 0
	for i, t := range tp {
		t.TrackNum = i + 1
		t.Flags = 0
		t.StartSector = pos
		t.LengthSectors = len
		pos += len
	}
	return tp
}

func lengthSectors(d unsafe.Pointer) int {
	return SectorsPerSecond * 3 * 60 * 10
}

func opened(d unsafe.Pointer) bool {
	return true
}

func setParanoia(cd *AudioCD, flags ParanoiaFlags) {}

func overlapSet(cd *AudioCD, sectors int) {}

func setSpeed(cd *AudioCD, x int) error {
	return nil
}

func seekSector(cd *AudioCD, sector int) error {
	return nil
}

func readLimited(cd *AudioCD, p []byte, retries int) error {
	_, err := rand.Read(p)
	return err
}

func closeDrive(d unsafe.Pointer) {}

func paranoiaFree(p unsafe.Pointer) {}

func version() string {
	return "mock"
}
