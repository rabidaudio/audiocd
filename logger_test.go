package audiocd

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	buf := bytes.Buffer{}
	logger := log.New(&buf, "cdda:", 0)
	cd := AudioCD{LogMode: LogModeLogger, Logger: logger}
	err := cd.Open()
	failIfErr(t, err)
	defer cd.Close()

	assert.Greater(t, buf.Len(), 0)
	str := buf.String()

	t.Log(str)
	assert.True(t, strings.HasPrefix(str, "cdda:"))
}
