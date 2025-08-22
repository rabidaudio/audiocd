// Package audiocd allows reading PCM audio data from a CD-DA disk
// in the cd drive.
//
// It's a cgo wrapper for [libcdio], which means it requires libcdio
// and headers to be installed, for example:
//
//	sudo apt install libcdio libcdio-dev
//
// NOTE: while this library is cross-platform, libcdio has not fully
// implemented features for MacOS, so this package will not be able to
// read audio CDs on Mac. Windows is untested.
//
// [libcdio]: https://www.gnu.org/software/libcdio/
package audiocd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"unsafe"
)

// SampleRate is the number of samples per second. All Redbook audio
// CDs use at 44.1KHz.
const SampleRate = 44100

// Samples are signed 16-bit
const BitsPerSample = 16
const BytesPerSample = BitsPerSample / 8

// Channels is the number of audio channels in the data. All Redbook
// audio CDs are stereo.
//
// CDParanoia source code detects 4-cannel audio on bit 8 of table of contents
// flags. [Wikipedia] notes that four-channel audio support was planned but never
// implemented and no known drives support it.
//
// [Wikipedia]: https://en.wikipedia.org/wiki/Compact_Disc_Digital_Audio#Audio_format
const Channels = 2

// SectorsPerSecond is the number of audio frames in one second of audio.
// An audio frame is the smallest valid unit of length for a track, defined
// as 1/75th of a second. Redbook track offsets are specified in MM:SS:FF.
//
// Note that this definition of frame is interchangeable with the term "sector".
// It is distinct from a 33-byte channel data frame, which this package does
// not concern itself with.
//
// For more information, see [Wikipedia].
//
// [Wikipedia]: https://en.wikipedia.org/wiki/Compact_Disc_Digital_Audio#Frames_and_timecode_frames
const SectorsPerSecond = 75

// SamplesPerSector is the number of 16-bit audio samples (including both channels)
// that appear within one frame of data (588).
const SamplesPerSector = SampleRate / SectorsPerSecond

// BytesPerSector is the number of bytes of audio contained in one sector of
// CD data (and equivalently in one frame of samples), 2352 bytes.
//
// Sectors are the unit of interest when reading data from CDs. AudioCD reads
// data in units of sectors.
const BytesPerSector = SampleRate * Channels * BytesPerSample / SectorsPerSecond

// TrackPosition reports the offset information for tracks
// from the table of contents.
type TrackPosition struct {
	NumChannels          int  // the number of audio channels. Spec defines 2 or 4 but no known audio cds support 4
	IsAudio              bool // mixed-mode disks can have data tracks in addition to audio tracks
	IsCopyPermitted      bool
	IsPreemphasisEnabled bool

	TrackNum      int // index of the track, starting at 1
	StartSector   int // address of the sector where the data starts
	LengthSectors int // total number of sectors the track covers
}

// ContainsSector reports whether the given sector is within the track bounds
func (t TrackPosition) ContainsSector(sector int) bool {
	return sector >= t.StartSector && sector < (t.StartSector+t.LengthSectors)
}

// AudioCD reads data from a CD-DA format cd in the disk drive.
// If Device is specified, AudioCD will read from the specified block device.
// Otherwise it will try to read from the first detected disk drive device.
// An AudioCD must be opened with [*AudioCD.Open] before use. The zero value
// for AudioCD is ready to be opened.
//
// AudioCD implements [io.ReadSeekCloser].
//
// Debug logging can be enabled by specifying LogMode. For [LogModeLogger],
// supply a [log.Logger] instance to Logger.
type AudioCD struct {
	Device string // the path to the cdrom device, e.g. /dev/cdrom

	buf            bytes.Buffer
	sbuf           []byte
	bufferedOffset int64
	trueOffset     int64

	cdio unsafe.Pointer // CdIo_t*
}

// ensure interface conformation
var _ io.ReadSeekCloser = (*AudioCD)(nil)

// Open determines the properties of the drive and detects
// the audio cd. This method must be called before information
// about the drive and cd can be accessed and before data can
// be read from the disk.
//
// Open this does not refer to controlling the drive tray.
func (cd *AudioCD) Open() error {
	if cd.IsOpen() {
		return nil
	}

	err := openDrive(cd)
	if err != nil {
		return err
	}
	err = cd.SetSpeed(-1)
	if err != nil {
		return err
	}

	cd.buf.Truncate(0)
	cd.buf.Grow(BytesPerSector)
	cd.bufferedOffset = 0
	cd.trueOffset = 0

	return nil
}

// Model returns information about the cd drive's manufacturer and model number.
func (cd *AudioCD) Model() string {
	if !cd.IsOpen() {
		return ""
	}
	return model(cd.cdio)
}

// TrackCount returns number of audio tracks on the disk.
// The CD-DA format supports a maximum of 99 tracks.
func (cd *AudioCD) TrackCount() int {
	if !cd.IsOpen() {
		return -1
	}
	return trackCount(cd.cdio)
}

// TOC returns the table of contents from the disk.
//
// The table of contents lists the tracks on the disk
// and the sector offsets they can be found at.
// It will have length of [*AudioCD.TrackCount].
func (cd *AudioCD) TOC() ([]TrackPosition, error) {
	if !cd.IsOpen() {
		return nil, ErrNotOpen
	}
	return toc(cd.cdio, cd.TrackCount())
}

// LengthSectors returns the total number of sectors on the disk
// with audio data. This is the sector after the last track.
func (cd *AudioCD) LengthSectors() (int, error) {
	if !cd.IsOpen() {
		return -1, ErrNotOpen
	}
	toc, err := cd.TOC()
	if err != nil {
		return -1, err
	}
	last := toc[len(toc)-1]
	return last.StartSector + last.LengthSectors, nil
}

// IsOpen reports whether the instance has been initialized
// and checked for audio tracks.
//
// IsOpen does not refer to the state of the drive tray.
func (cd *AudioCD) IsOpen() bool {
	if cd.cdio == nil {
		return false
	}
	return true
}

// SetSpeed sets the data read speed multiplier.
// 1x reads at real-time audio speed, 75 sectors/second.
// Use -1 (the default) to read as fast as possible.
func (cd *AudioCD) SetSpeed(x int) error {
	if !cd.IsOpen() {
		return os.ErrClosed
	}
	if x == -1 {
		x = int(0xffff)
	}
	return setSpeed(cd, x)
}

// Seek provides access to the cursor position for reading audio data.
// It allows seeking to arbitrary sub-sector byte offsets.
func (cd *AudioCD) Seek(offset int64, whence int) (int64, error) {
	if !cd.IsOpen() {
		return cd.trueOffset, os.ErrClosed
	}

	var newoffset int64
	switch whence {
	case io.SeekCurrent:
		newoffset = cd.trueOffset + offset
	case io.SeekEnd:
		l, err := cd.LengthSectors()
		if err != nil {
			return 0, err
		}
		end := int64(l) * BytesPerSector
		newoffset = end + offset
	default:
		newoffset = offset
	}

	if newoffset == cd.trueOffset {
		// nothing to do
		return cd.trueOffset, nil
	}

	if newoffset > cd.trueOffset && newoffset < cd.bufferedOffset {
		// can use data already in buffer
		_ = cd.buf.Next(int(newoffset - cd.trueOffset)) // empty the buffer up to current point
		cd.trueOffset = newoffset
		return cd.trueOffset, nil
	}

	// otherwise we're going to need to wipe buffer and seek
	cd.buf.Truncate(0) // wipe buffered data
	cd.trueOffset = cd.bufferedOffset
	secoffset := newoffset - (newoffset % BytesPerSector)

	err := cd.bufferSectors(1)
	cd.trueOffset = cd.bufferedOffset
	if err != nil {
		return cd.trueOffset, err
	}
	// seek buffer ahead to sub-sector offset
	_ = cd.buf.Next(int(newoffset - secoffset))
	cd.trueOffset = newoffset
	return cd.trueOffset, nil
}

// SeekToSector seeks the cd to the specified sector index.
// This is useful for going to the start of a track.
func (cd *AudioCD) SeekToSector(sector int) (int64, error) {
	return cd.Seek(int64(sector)*BytesPerSector, io.SeekStart)
}

// Read reads PCM audio data from the disk.
//
// Read only supports reading complete sectors, and will error
// for partial reads.
//
// PCM data is signed 16-bit samples. Data will be in host byte order,
// regardless of drive endianness.
func (cd *AudioCD) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	// if there's data available in the buffer, return just that
	if cd.buf.Len() > 0 {
		n = len(p)
		if n > cd.buf.Len() {
			n = cd.buf.Len()
		}
		copy(p[:n], cd.buf.Next(n))
		cd.trueOffset += int64(n)

		// if more was requested, continue reading
		nn, err := cd.Read(p[n:])
		return n + nn, err
	}

	// otherwise load data into the buffer
	nsectors := (len(p) / BytesPerSector) + 1
	err = cd.bufferSectors(int(nsectors))
	if err != nil {
		return 0, err
	}
	// recurse to load said data from buffer
	return cd.Read(p)
}

func (cd *AudioCD) bufferSectors(nsectors int) error {
	if cd.sbuf == nil {
		cd.sbuf = make([]byte, nsectors*BytesPerSector)
	}
	if len(cd.sbuf) < nsectors*BytesPerSector {
		cd.sbuf = make([]byte, nsectors*BytesPerSector)
	}
	n, err := cd.readSectors(cd.sbuf)
	cd.bufferedOffset += n
	cd.buf.Write(cd.sbuf[:n])
	return err
}

func (cd *AudioCD) readSectors(p []byte) (int64, error) {
	if !cd.IsOpen() {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}

	if int(len(p))%BytesPerSector != 0 {
		return 0, fmt.Errorf("audiocd: must read complete sectors")
	}

	nsectors := len(p) / BytesPerSector
	err := readAudioSectors(cd, p, nsectors)
	if err != nil {
		return 0, err
	}
	return int64(len(p)), nil
}

// CloseTray closes the cd drive tray. Not all drives/drivers support this.
func CloseTray(device string) error {
	return closeTray(device)
}

// EjectMedia stops reading and opens the CD tray. This also
// closes the AudioCD.
func (cd *AudioCD) EjectMedia() error {
	err := ejectMedia(cd)
	if err != nil {
		return err
	}
	return cd.Close()
}

// Close releases access to the cd drive. Data can no longer be accessed
// unless opened again.
//
// Close this does not refer to controlling the drive tray.
func (cd *AudioCD) Close() error {
	if cd.IsOpen() {
		closeDrive(cd.cdio)
	}
	cd.cdio = nil
	cd.buf.Truncate(0)
	return nil
}

// Version returns the libcdio version string.
func Version() string {
	return version()
}
