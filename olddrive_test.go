package audiocd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParanoiaVersion(t *testing.T) {
	assert.Equal(t, "10.2", Version())
}

func failIfErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

// NOTE: this test is setup for a specific test device
// with a specific CD plugged in and is not portable
func TestOldDriveInfo(t *testing.T) {
	drive := AudioCD{Device: "/dev/sr1"}
	err := drive.Open()
	failIfErr(t, err)
	defer drive.Close()

	assert.Equal(t, "MATSHITA UJDA775 DVD/CDRW 1.00 ", drive.Model())
	assert.Equal(t, DriveType(11) /*SCSI_CDROM_MAJOR*/, drive.DriveType())
	assert.Equal(t, InterfaceType(3) /*SGIO_SCSI*/, drive.InterfaceType())
	assert.Equal(t, 5, drive.TrackCount())
	assert.Equal(t, int32(0), drive.FirstAudioSector())

	toc := drive.TOC()

	assert.Equal(t, drive.TrackCount(), len(toc))

	assert.Equal(t, uint8(1), toc[0].TrackNum, "tracks are 1 indexed")
	assert.Equal(t, uint8(2), toc[1].TrackNum)
	assert.Equal(t, uint8(3), toc[2].TrackNum)
	assert.Equal(t, uint8(4), toc[3].TrackNum)
	assert.Equal(t, uint8(5), toc[4].TrackNum)

	for i := range toc {
		assert.True(t, toc[i].IsAudio())
	}

	assert.Equal(t, int32(0), toc[0].StartSector)
	assert.Equal(t, int32(44988), toc[4].StartSector)

	assert.Equal(t, int32(6290), toc[0].LengthSectors)
	assert.Equal(t, int32(17021), toc[1].LengthSectors)
	assert.Equal(t, int32(7763), toc[2].LengthSectors)
	assert.Equal(t, int32(13914), toc[3].LengthSectors)
	assert.Equal(t, int32(11903), toc[4].LengthSectors)

	len := drive.LengthSectors()
	assert.Equal(t, int32(56891), len)
}

func TestRead(t *testing.T) {
	drive := AudioCD{Device: "/dev/sr1"}
	err := drive.Open()
	failIfErr(t, err)
	defer drive.Close()

	buf := make([]byte, BytesPerSector)
	n, err := drive.Read(buf)
	failIfErr(t, err)
	assert.Equal(t, len(buf), n)

	for _, v := range buf[:16] {
		if v != 0 {
			return
		}
	}
	t.Fatalf("expected data but found none: %v", buf[:64])
}

func TestRipTrack1(t *testing.T) {
	drive := AudioCD{Device: "/dev/sr1"}
	err := drive.Open()
	failIfErr(t, err)
	defer drive.Close()

	toc := drive.TOC()

	start := toc[0].StartSector
	end := toc[1].StartSector

	buf := make([]byte, (end-start)*BytesPerSector)
	read := 0
	for read < len(buf) {
		n, err := drive.Read(buf[read:])
		failIfErr(t, err)
		read += n
	}

	assert.True(t, read%int(BytesPerSector) == 0)
	assert.Equal(t, len(buf), read)

	err = os.WriteFile("track1.cdda", buf, 0777)
	failIfErr(t, err)
}

func TestRipTrack5(t *testing.T) {
	drive := AudioCD{Device: "/dev/sr1"}
	err := drive.Open()
	failIfErr(t, err)
	defer drive.Close()

	toc := drive.TOC()

	start := toc[4].StartSector
	end := start + toc[4].LengthSectors

	n, err := drive.Seek(int64(start*BytesPerSector), io.SeekStart)
	failIfErr(t, err)
	assert.Equal(t, int64(start*BytesPerSector), n)

	buf := make([]byte, (end-start)*BytesPerSector)
	read := 0
	for read < len(buf) {
		n, err := drive.Read(buf[read:])
		failIfErr(t, err)
		read += n
	}

	assert.True(t, read%int(BytesPerSector) == 0)
	assert.Equal(t, len(buf), read)

	err = os.WriteFile("track5.cdda", buf, 0777)
	failIfErr(t, err)
}

func TestRipEvery10s(t *testing.T) {
	drive := AudioCD{Device: "/dev/sr1"}
	err := drive.Open()
	failIfErr(t, err)
	defer drive.Close()

	buf := bytes.Buffer{}

	step := 10 * SectorsPerSecond
	len := SectorsPerSecond * BytesPerSector
	for i := range drive.LengthSectors() / step {
		_, err := drive.SeekToSector(i * step)
		failIfErr(t, err)

		buf.Grow(int(len))
		b := buf.AvailableBuffer()
		_, err = drive.Read(b[:len])
		failIfErr(t, err)

		buf.Write(b[:len])
	}

	err = os.WriteFile("steps.cdda", buf.Bytes(), 0777)
	failIfErr(t, err)
}
