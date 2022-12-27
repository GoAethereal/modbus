package modbus

import (
	"encoding/binary"
	"errors"
	"sync/atomic"
)

// framer represents the modbus mode
type framer interface {
	buffer() []byte
	encode(uid, code byte, data []byte) (adu []byte, err error)
	decode(adu []byte) (uid, code byte, data []byte, err error)
	verify(req, res []byte) (err error)
	reply(uid, code byte, data, req []byte) (res []byte, err error)
}

var _ framer = (*tcp)(nil)

type tcp struct {
	transId uint32
}

func (s *tcp) buffer() []byte {
	return make([]byte, 260)
}

func (s *tcp) encode(uid, code byte, data []byte) (adu []byte, err error) {
	if len(data) > 252 {
		return nil, ErrDataSizeExceeded
	}
	adu = s.buffer()
	binary.BigEndian.PutUint16(adu[0:], uint16(atomic.AddUint32(&s.transId, 1)))
	binary.BigEndian.PutUint16(adu[4:], 2+uint16(len(data)))
	adu[6], adu[7] = uid, code
	return adu[:8+copy(adu[8:], data)], nil
}

func (s *tcp) decode(adu []byte) (uid, code byte, data []byte, err error) {
	if len(adu) < 8 {
		return 0, 0, nil, errors.New("modbus: invalid request")
	}
	if adu[7] >= 0x80 {
		return 0, 0, nil, Exception(adu[8])
	}
	return adu[6], adu[7], adu[8:], nil
}

func (s *tcp) verify(req, res []byte) error {
	switch {
	case req[0] != res[0] || req[1] != res[1]:
		return ErrMismatchedTransactionId
	case req[2] != res[2] || req[3] != res[3]:
		return ErrMismatchedProtocolId
	case req[6] != 0 && req[6] != res[6]:
		return ErrMismatchedUnitId
	}
	return nil
}

func (s *tcp) reply(uid, code byte, data, req []byte) (res []byte, err error) {
	if res, err = s.encode(uid, code, data); err != nil {
		return nil, err
	}
	// copy transaction id from request
	res[0], res[1] = req[0], req[1]
	return res, nil
}
