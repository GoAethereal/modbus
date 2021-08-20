package modbus

import (
	"context"
	"encoding/binary"
)

// Handler is firstly and foremost used by the modbus.Server.
// The Handle method describes how incoming messages are managed.
type Handler interface {
	Handle(ctx context.Context, code byte, req []byte) (res []byte, ex Exception)
}

var _ Handler = (*Mux)(nil)

// Mux implements the modbus.Handler interface and is intended to be used as a server side request
// multiplexer. When called by the server it will redirect the inbound message to the given function.
// If the callback is not set the Mux will return the modbus.ExIllegalFunction exception to the server.
// In case of an unknown function code the Fallback function, if set, will be executed.
// All given functions must be safe for use by multiple go routines.
type Mux struct {
	Fallback                   func(ctx context.Context, code byte, req []byte) (res []byte, ex Exception)
	ReadCoils                  func(ctx context.Context, address, quantity uint16) (res []bool, ex Exception)
	ReadDiscreteInputs         func(ctx context.Context, address, quantity uint16) (res []bool, ex Exception)
	ReadHoldingRegisters       func(ctx context.Context, address, quantity uint16) (res []byte, ex Exception)
	ReadInputRegisters         func(ctx context.Context, address, quantity uint16) (res []byte, ex Exception)
	WriteSingleCoil            func(ctx context.Context, address uint16, status bool) (ex Exception)
	WriteSingleRegister        func(ctx context.Context, address, value uint16) (ex Exception)
	WriteMultipleCoils         func(ctx context.Context, address uint16, status []bool) (ex Exception)
	WriteMultipleRegisters     func(ctx context.Context, address uint16, values []byte) (ex Exception)
	ReadWriteMultipleRegisters func(ctx context.Context, rAddress, rQuantity, wAddress uint16, values []byte) (res []byte, ex Exception)
}

// Handle dispatches incoming requests depending on their function code to the correlating callbacks
// as defined inside the Mux.
func (h *Mux) Handle(ctx context.Context, code byte, req []byte) (res []byte, ex Exception) {
	switch code {
	case 0x01:
		return h.readCoils(ctx, req)
	case 0x02:
		return h.readDiscreteInputs(ctx, req)
	case 0x03:
		return h.readHoldingRegisters(ctx, req)
	case 0x04:
		return h.readInputRegisters(ctx, req)
	case 0x05:
		return h.writeSingleCoil(ctx, req)
	case 0x06:
		return h.writeSingleRegister(ctx, req)
	case 0x0F:
		return h.writeMultipleCoils(ctx, req)
	case 0x10:
		return h.writeMultipleRegisters(ctx, req)
	case 0x17:
		return h.readWriteMultipleRegisters(ctx, req)
	}
	return h.fallback(ctx, code, req)
}

func (h *Mux) fallback(ctx context.Context, code byte, req []byte) (res []byte, ex Exception) {
	if h.Fallback == nil {
		return nil, ExIllegalFunction
	}
	return h.Fallback(ctx, code, req)
}

func (h *Mux) readCoils(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.ReadCoils == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 2000:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	status, ex := h.ReadCoils(ctx, address, quantity)
	switch {
	case ex != nil:
		return nil, ex
	case len(status) != int(quantity):
		return nil, ExSlaveDeviceFailure
	}
	return put(1+int(byteCount(quantity)), byte(byteCount(quantity)), status), nil
}

func (h *Mux) readDiscreteInputs(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.ReadDiscreteInputs == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 2000:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	status, ex := h.ReadDiscreteInputs(ctx, address, quantity)
	switch {
	case ex != nil:
		return nil, ex
	case len(status) != int(quantity):
		return nil, ExSlaveDeviceFailure
	}
	return put(1+int(byteCount(quantity)), byte(byteCount(quantity)), status), nil
}

func (h *Mux) readHoldingRegisters(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.ReadHoldingRegisters == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 125:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	values, ex := h.ReadHoldingRegisters(ctx, address, quantity)
	switch {
	case ex != nil:
		return nil, ex
	case len(values) != 2*int(quantity):
		return nil, ExSlaveDeviceFailure
	}
	return put(1+int(quantity)*2, byte(quantity*2), values), nil
}

func (h *Mux) readInputRegisters(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.ReadInputRegisters == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 125:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	values, ex := h.ReadInputRegisters(ctx, address, quantity)
	switch {
	case ex != nil:
		return nil, ex
	case len(values) != 2*int(quantity):
		return nil, ExSlaveDeviceFailure
	}
	return put(1+int(quantity)*2, byte(quantity*2), values), nil
}

func (h *Mux) writeSingleCoil(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.WriteSingleCoil == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	status := false
	switch binary.BigEndian.Uint16(req[2:]) {
	case 0x0000:
	case 0xFF00:
		status = true
	default:
		return nil, ExIllegalDataValue
	}
	if ex = h.WriteSingleCoil(ctx, address, status); ex != nil {
		return nil, ex
	}
	return req, nil
}

func (h *Mux) writeSingleRegister(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.WriteSingleRegister == nil:
		return nil, ExIllegalFunction
	case len(req) != 4:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	value := binary.BigEndian.Uint16(req[2:])
	if ex = h.WriteSingleRegister(ctx, address, value); ex != nil {
		return nil, ex
	}
	return req, nil
}

func (h *Mux) writeMultipleCoils(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.WriteMultipleCoils == nil:
		return nil, ExIllegalFunction
	case len(req) < 6:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 1968 || len(req[5:]) != int(req[4]):
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	if ex = h.WriteMultipleCoils(ctx, address, bytesToBools(quantity, req[5:])); ex != nil {
		return nil, ex
	}
	return req[:4], nil
}

func (h *Mux) writeMultipleRegisters(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.WriteMultipleRegisters == nil:
		return nil, ExIllegalFunction
	case len(req) < 6:
		return nil, ExIllegalDataAddress
	}
	address := binary.BigEndian.Uint16(req[0:])
	quantity := binary.BigEndian.Uint16(req[2:])
	switch {
	case quantity < 1 || quantity > 123 || 2*quantity != uint16(req[4]) || int(req[4]) != len(req[5:]):
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	if ex = h.WriteMultipleRegisters(ctx, address, req[5:]); ex != nil {
		return nil, ex
	}
	return req[:4], nil
}

func (h *Mux) readWriteMultipleRegisters(ctx context.Context, req []byte) (res []byte, ex Exception) {
	switch {
	case h.ReadWriteMultipleRegisters == nil:
		return nil, ExIllegalFunction
	case len(req) < 11:
		return nil, ExIllegalDataAddress
	}
	rAddress := binary.BigEndian.Uint16(req[0:])
	rQuantity := binary.BigEndian.Uint16(req[2:])
	wAddress := binary.BigEndian.Uint16(req[4:])
	wQuantity := binary.BigEndian.Uint16(req[6:])
	switch {
	case rQuantity < 1 || rQuantity > 125 || wQuantity < 1 || wQuantity > 121 || rQuantity*2 != uint16(req[8]) || int(req[8]) != len(req[9:]):
		return nil, ExIllegalDataValue
	case int(rAddress)+int(rQuantity) > 0xFFFF || int(wAddress)+int(wQuantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	res, ex = h.ReadWriteMultipleRegisters(ctx, rAddress, rQuantity, wAddress, req[9:])
	switch {
	case ex != nil:
		return nil, ex
	case len(res) != int(rQuantity)*2:
		return nil, ExSlaveDeviceFailure
	}
	return put(1+len(res), byte(len(res)), res), nil
}
