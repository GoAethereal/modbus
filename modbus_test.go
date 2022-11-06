package modbus_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/GoAethereal/modbus"
	"github.com/GoAethereal/stream"
)

func TestReadHoldingRegisters(t *testing.T) {
	cases := []struct {
		cmd modbus.ReadHoldingRegisters
		req stream.Buffer
		res stream.Buffer
		err error
	}{{cmd: modbus.ReadHoldingRegisters{1, make([]byte, 4)},
		req: []byte{0, 1, 0, 2},
		res: []byte{4, 1, 3, 3, 7},
		err: io.EOF}}
	for _, c := range cases {
		if err := c.cmd.Request()(func(s stream.Codec) error {
			_, err := io.Copy(s.Writer(), c.res.Encoder().Reader())
			return err
		}, func(s stream.Codec) error {
			buf, err := io.ReadAll(s.Reader())
			if err != nil {
				return err
			}
			if bytes.Equal(buf, c.req) {
				return errors.New("")
			}
			return nil
		}); !errors.Is(err, c.err) {
			t.Error(err)
		}
	}
}
