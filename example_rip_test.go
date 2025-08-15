package audiocd_test

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/rabidaudio/audiocd"
)

// Example of ripping a single track
func Example() {
	// open the disk
	cd := audiocd.AudioCD{Device: "/dev/cdrom"}
	err := cd.Open()
	if err != nil {
		panic(err)
	}
	defer cd.Close()

	// show information about the drive
	fmt.Printf("model: %v\n", cd.Model())
	// read the table of contents
	toc := cd.TOC()

	fmt.Printf("track\tstart\tlen\n")
	for _, track := range toc {
		fmt.Printf("%02d\t% 5d\t% 5d\n", track.TrackNum, track.StartSector, track.LengthSectors)
	}

	// seek to the start of track 2
	_, err = cd.SeekToSector(toc[1].StartSector)

	// create a new wave file to stream to
	f, err := os.Create("track2.wav")
	defer f.Close()

	f.Write(CreateWavHeader(uint32(toc[1].LengthSectors * audiocd.BytesPerSector)))

	// stream to file
	data := make([]byte, audiocd.BytesPerSector)
	for range toc[1].LengthSectors {
		_, err = cd.Read(data)
		f.Write(data)
	}
}

func CreateWavHeader(nbytes uint32) []byte {
	b := make([]byte, 44)

	copy(b[0:4], []byte{'R', 'I', 'F', 'F'})
	binary.LittleEndian.PutUint32(b[4:8], nbytes+44-8)
	copy(b[8:12], []byte{'W', 'A', 'V', 'E'})
	copy(b[12:16], []byte{'f', 'm', 't', ' '})
	binary.LittleEndian.PutUint32(b[16:20], 16) // block size
	binary.LittleEndian.PutUint16(b[20:22], 1)  // format
	binary.LittleEndian.PutUint16(b[22:24], audiocd.Channels)
	binary.LittleEndian.PutUint32(b[24:28], audiocd.SampleRate)
	binary.LittleEndian.PutUint32(b[28:32], audiocd.SampleRate*audiocd.Channels*audiocd.BytesPerSample)
	binary.LittleEndian.PutUint16(b[32:34], audiocd.Channels*audiocd.BytesPerSample)
	binary.LittleEndian.PutUint16(b[34:36], audiocd.BitsPerSample)
	copy(b[36:40], []byte{'d', 'a', 't', 'a'})
	binary.LittleEndian.PutUint32(b[40:44], nbytes)
	return b
}
