# audiocd

[![GoDoc](https://godoc.org/github.com/rabidaudio/audiocd?status.svg)](https://godoc.org/github.com/rabidaudio/audiocd)

`audiocd` connects to your disk drive and reads/rips PCM audio data from standard CD-DA disks.

```bash
go get -u github.com/rabidaudio/audiocd
```

It's a `cgo` wrapper around [CDParanoia](https://xiph.org/paranoia/index.html). This means that it only works on Linux. It will build on non-Linux platforms with a mock implementation which returns white noise.

Building requires the development files for `libcdparanoia`, e.g. on Ubuntu/Debian/etc:

```bash
sudo apt install cdparanoia libcdparanoia libcdparanoia-dev
```

See [GoDoc](https://godoc.org/github.com/rabidaudio/audiocd) for more details.
