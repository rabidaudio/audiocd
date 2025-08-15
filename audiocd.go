// Package audiocd allows reading PCM audio data from a CD-DA disk
// in the cd drive.
//
// It's a cgo wrapper for [CDParanoia], which means it only runs on Linux
// and requires libcdparanoia and headers to be installed, for example:
//
//	sudo apt install cdparanoia libcdparanoia-dev
//
// It also means it has really powerful error correction capabilities.
//
// [CDParanoia]: https://xiph.org/paranoia/index.html
package audiocd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"
)

// LogMode configures the destination for debug logs.
type LogMode int

const (
	LogModeSilent LogMode = 0 // disable logs
	LogModeStdErr LogMode = 1 // log to stderr
	LogModeLogger LogMode = 2 // log to the supplied log.Logger instance
)

// ParanoiaFlags enable specific error checking features.
type ParanoiaFlags int

const (
	ParanoiaModeFull    ParanoiaFlags = 0xFF // enable all error checking features
	ParanoiaModeDisable ParanoiaFlags = 0x00 // disable all error checking features

	ParanoiaVerify    ParanoiaFlags = (1 << 0)
	ParanoiaFragment  ParanoiaFlags = (1 << 1)
	ParanoiaOverlap   ParanoiaFlags = (1 << 2)
	ParanoiaScratch   ParanoiaFlags = (1 << 3)
	ParanoiaRepair    ParanoiaFlags = (1 << 4)
	ParanoiaNeverSkip ParanoiaFlags = (1 << 5)
)

// InterfaceType represents the driver implementation used
// to access the drive.
type InterfaceType int

// DriveType represents the underlying driver/protocol used
// by the drive.
//
// These definitions come from [linux/major.h], with
// descriptions from [scan_devices.c].
//
// [linux/major.h]: https://github.com/torvalds/linux/blob/0e39a731820ad26533eb988cef27ad2506063b5b/include/uapi/linux/major.h#L27
// [scan_devices.c]: https://github.com/opus47/cdparanoia/blob/bc6a412bca35b03413b88e7ba2cb78c911f1d8f3/interface/scan_devices.c#L177
type DriveType int

// FullSpeed can be passed to [SetSpeed] to run the drive at its fastest speed.
const FullSpeed = -1

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
// Note that this definition of frame is interchangeable with sector.
// It is distinct from a 33-byte channel data frame, which this package does
// not concern itself with.
//
// For more information, see [Wikipedia].
//
// [Wikipedia]: https://en.wikipedia.org/wiki/Compact_Disc_Digital_Audio#Frames_and_timecode_frames
const SectorsPerSecond = 75

// SamplesPerFrame is the number of 16-bit audio samples per channel
// that appear within one frame of data (294).
const SamplesPerFrame = SampleRate / SectorsPerSecond / Channels

// BytesPerSector is the number of bytes of audio contained in one sector of
// CD data (and equivalently in one frame of samples), 2352 bytes.
//
// Sectors are the unit of interest when reading data from CDs. AudioCD reads
// data in units of sectors.
const BytesPerSector = SampleRate * Channels * BytesPerSample / SectorsPerSecond

// TrackPosition reports the offset information for tracks
// from the table of contents.
type TrackPosition struct {
	Flags         byte // bitflag parameters
	TrackNum      int  // index of the track, starting at 1
	StartSector   int  // address of the sector where the data starts
	LengthSectors int  // total number of sectors the track covers

	// TODO: handle pregap?
}

func (t TrackPosition) IsPreemphasisEnabled() bool {
	return (t.Flags & 0x01) != 0
}

func (t TrackPosition) IsCopyProtected() bool {
	return (t.Flags & 0x02) != 0
}

// IsAudio reports whether the track is an audio track.
// Mixed-mode disks can have data tracks in addition to audio tracks.
func (t TrackPosition) IsAudio() bool {
	return (t.Flags & 0x04) == 0
}

// ContainsSector reports whether the given sector is within the track bounds
func (t TrackPosition) ContainsSector(sector int) bool {
	return sector >= t.StartSector && sector < (t.StartSector+t.LengthSectors)
}

// AudioCD reads data from a CD-DR format cd in the disk drive.
// If Device is specified, AudioCD will read from the specified block device.
// Otherwise it will try to read from the first detected disk drive device.
// An AudioCD must be [Open]ed before use. The zero value for AudioCD is ready to be opened.
//
// AudioCD implements [io.ReadSeekCloser].
//
// Debug logging can be enabled by specifying LogMode. For [LogModeLogger],
// supply a [log.Logger] instance to Logger.
type AudioCD struct {
	Device     string      // the path to the cdrom device, e.g. /dev/cdrom
	MaxRetries int         // number of repeated reads on failed sectors. Set to -1 to disable retries. If 0, the default of 20 will be used
	LogMode    LogMode     // direct the library logs
	Logger     *log.Logger // if LogMode == LogModeLogger, the log.Logger to use

	buf            bytes.Buffer
	bufferedOffset int64
	trueOffset     int64

	drive    unsafe.Pointer // *C.cdrom_drive
	paranoia unsafe.Pointer // *C.cdrom_paranoia
}

// ensure interface conformation
var _ io.ReadSeekCloser = (*AudioCD)(nil)

// Open determines the properties of the drive and detects
// the audio cd. This method must be called before information
// about the drive and cd can be accessed and before data can
// be read from the disk.
//
// If one of [ErrReadTOCLeadOut], [ErrIllegalNumberOfTracks],
// [ErrReadTOCHeader], or [ErrReadTOCEntry] is returned,
// it's likely that no cd is in the drive or the cd is not
// an audio cd.
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
	err = cd.SetSpeed(FullSpeed)
	if err != nil {
		return err
	}

	cd.buf.Truncate(0)
	cd.buf.Grow(BytesPerSector)
	cd.bufferedOffset = 0
	cd.trueOffset = 0
	err = seekSector(cd, 0)
	if err != nil {
		return err
	}

	cd.SetParanoiaMode(ParanoiaModeFull)
	return nil
}

// Model returns information about the cd drive's manufacturer and model number.
func (cd *AudioCD) Model() string {
	if !cd.IsOpen() {
		return ""
	}
	return model(cd.drive)
}

func (cd *AudioCD) DriveType() DriveType {
	if !cd.IsOpen() {
		return -1
	}
	return driveType(cd.drive)
}

func (cd *AudioCD) InterfaceType() InterfaceType {
	if !cd.IsOpen() {
		return -1
	}
	return interfaceType(cd.drive)
}

// TrackCount returns number of audio tracks on the disk.
// The CD-DA format supports a maximum of 99 tracks.
func (cd *AudioCD) TrackCount() int {
	if !cd.IsOpen() {
		return -1
	}
	return trackCount(cd.drive)
}

// FirstAudioSector returns the sector index of the first track.
func (cd *AudioCD) FirstAudioSector() int {
	if !cd.IsOpen() {
		return -1
	}
	return firstAudioSector(cd.drive)
}

// TOC returns the table of contents from the disk.
//
// The table of contents lists the tracks on the disk
// and the sector offsets they can be found at.
// It will have length of [TrackCount].
func (cd *AudioCD) TOC() []TrackPosition {
	if !cd.IsOpen() {
		return nil
	}
	return toc(cd.drive, cd.TrackCount())
}

// LengthSectors returns the total number of sectors on the disk
// with audio data. This is the sector after the last track.
func (cd *AudioCD) LengthSectors() int {
	if !cd.IsOpen() {
		return -1
	}
	return lengthSectors(cd.drive)
}

// TrackAtSector returns the number of the track that
// contains the given sector, if any. Track numbers
// start at 1.
//
// If the CD isn't open, returns -1. If the sector
// is outside the track bounds, returns 0.
func (cd *AudioCD) TrackAtSector(sector int) int {
	if !cd.IsOpen() {
		return -1
	}

	toc := cd.TOC()
	for _, t := range toc {
		if t.ContainsSector(sector) {
			return t.TrackNum
		}
	}
	return 0
}

// IsOpen reports whether the instance has been initialized
// and checked for audio tracks.
//
// IsOpen does not refer to the state of the drive tray.
func (cd *AudioCD) IsOpen() bool {
	if cd.drive == nil {
		return false
	}
	return opened(cd.drive)
}

// SetParanoiaMode sets how "paranoid" audiocd will be about error
// checking and correcting. [ParanoiaModeFull] (the default)
// enables all the correction features. [ParanoiaModeDisable] (0)
// disables all checks. Individual checks can be enabled, e.g.
// ParanoiaRepair|ParanoiaNeverSkip.
func (cd *AudioCD) SetParanoiaMode(flags ParanoiaFlags) {
	setParanoia(cd, flags)
}

// ForceSearchOverlap sets the minimum number of sectors to search
// when detecting overlap issues during data verification.
func (cd *AudioCD) ForceSearchOverlap(sectors int) error {
	if !cd.IsOpen() {
		return os.ErrClosed
	}
	if sectors < 0 || sectors > 75 {
		return fmt.Errorf("audiocd: search overlap sectors must be 0 <= n <= 75")
	}

	overlapSet(cd, sectors)
	return nil
}

// SetSpeed sets the data read speed multiplier.
// 1x reads at real-time audio speed, 75 sectors/second.
// Use [FullSpeed] (the default) to read as fast as possible.
func (cd *AudioCD) SetSpeed(x int) error {
	if !cd.IsOpen() {
		return os.ErrClosed
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
		end := int64(cd.LengthSectors()) * BytesPerSector
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

	err := seekSector(cd, int(secoffset/BytesPerSector))
	if err != nil {
		cd.trueOffset = cd.bufferedOffset
		return cd.trueOffset, err
	}
	err = cd.bufferSectors(1)
	cd.trueOffset = cd.bufferedOffset
	if err != nil {
		return cd.trueOffset, err
	}
	// seek buffer ahead to sub-sector offset
	_ = cd.buf.Next(int(newoffset - secoffset))
	cd.trueOffset = newoffset
	return cd.trueOffset, nil
}

// SeekToSector seeks the cd to the specfied sector index.
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

	if int(len(p)) > BytesPerSector {
		// read one sector
		n, err := cd.readSectors(p[:BytesPerSector])
		if err != nil {
			return n, err
		}
		// read remaining sectors
		nn, err := cd.readSectors(p[BytesPerSector:])
		return n + nn, err
	}

	retries := cd.MaxRetries
	if retries < 0 {
		retries = 0 // disable
	} else if retries == 0 {
		retries = 20 // default value
	}
	readLimited(cd, p, retries)
	return BytesPerSector, nil
}

func (cd *AudioCD) bufferSectors(nsectors int) error {
	p := make([]byte, nsectors*BytesPerSector)
	n, err := cd.readSectors(p)
	cd.bufferedOffset += n
	cd.buf.Write(p[:n])
	return err
}

// Close releases access to the cd drive. Data can no longer be accessed
// unless [Open]ed again.
//
// Close this does not refer to controlling the drive tray.
func (cd *AudioCD) Close() error {
	if cd.IsOpen() {
		closeDrive(cd.drive)
	}
	if cd.paranoia != nil {
		paranoiaFree(cd.paranoia)
	}

	cd.paranoia = nil
	cd.drive = nil
	cd.buf.Truncate(0)
	return nil
}

// Version returns the libcdparanoia version string.
func Version() string {
	return version()
}
