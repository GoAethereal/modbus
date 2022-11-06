package modbus

import (
	"encoding/binary"
	"fmt"

	. "github.com/GoAethereal/stream"
)

const (
	// The function code received in the query is not an allowable action for the server (or slave). This
	// may be because the function code is only applicable to newer devices, and was not
	// implemented in the unit selected. It could also indicate that the server (or slave) is in the wrong
	// state to process a request of this type, for example because it is not configured and is being
	// asked to return register values.
	IllegalFunction Exception = 0x01
	// The data address received in the query is not an allowable address for the server (or slave). More
	// specifically, the combination of reference number and transfer length is invalid. For a controller with
	// 100 registers, the PDU addresses the first register as 0, and the last one as 99. If a request
	// is submitted with a starting register address of 96 and a quantity of registers of 4, then this request
	// will successfully operate (address-wise at least) on registers 96, 97, 98, 99. If a request is
	// submitted with a starting register address of 96 and a quantity of registers of 5, then this request
	// will fail with Exception Code 0x02 “Illegal Data Address” since it attempts to operate on registers
	// 96, 97, 98, 99 and 100, and there is no register with address 100.
	IllegalDataAddress Exception = 0x02
	// A value contained in the query data field is not an allowable value for server (or slave). This
	// indicates a fault in the structure of the remainder of a complex request, such as that the implied
	// length is incorrect. It specifically does NOT mean that a data item submitted for storage in a register
	// has a value outside the expectation of the application program, since the MODBUS protocol
	// is unaware of the significance of any particular value of any particular register.
	IllegalDataValue Exception = 0x03
	// An unrecoverable error occurred while the server (or slave) was attempting to perform the
	// requested action.
	SlaveDeviceFailure Exception = 0x04
	// Specialized use in conjunction with programming commands. The server (or slave) has accepted the request
	// and is processing it, but a long duration of time will be required to do so. This response is
	// returned to prevent a timeout error from occurring in the client (or master). The client (or master)
	// can next issue a Poll Program Complete message to determine if processing is completed.
	Acknowledge Exception = 0x05
	// Specialized use in conjunction with programming commands. The server (or slave) is engaged in processing a
	// long–duration program command. The client (or master) should retransmit the message later when
	// the server (or slave) is free
	SlaveDeviceBusy Exception = 0x06
	// Specialized use in conjunction with function codes 20 and 21 and reference type 6, to indicate that
	// the extended file area failed to pass a consistency check. The server (or slave) attempted to read record
	// file, but detected a parity error in the memory. The client (or master) can retry the request, but
	// service may be required on the server (or slave) device.
	MemoryParityError Exception = 0x08
	// Specialized use in conjunction with gateways, indicates that the gateway was unable to allocate
	// an internal communication path from the input port to the output port for processing the request.
	// Usually means that the gateway is misconfigured or overloaded.
	GatewayPathUnavailable Exception = 0x0A
	// Specialized use in conjunction with gateways, indicates that no response was obtained from the
	// target device. Usually means that the device is not present on the network.
	GatewayTargetDeviceFailedToRespond Exception = 0x0B
)

// Exception represents a modbus exception as defined by the specification.
type Exception byte

// Error implements the builtin error interface,
// returning a human readable string representing the underlying exception.
func (ex Exception) Error() string {
	prefix := "modbus: "
	switch ex {
	case 0:
		return prefix
	case IllegalFunction:
		return prefix + "illegal function"
	case IllegalDataAddress:
		return prefix + "illegal data address"
	case IllegalDataValue:
		return prefix + "illegal data value"
	case SlaveDeviceFailure:
		return prefix + "slave device failure"
	case Acknowledge:
		return prefix + "acknowledge"
	case SlaveDeviceBusy:
		return prefix + "slave device busy"
	case MemoryParityError:
		return prefix + "memory parity error"
	case GatewayPathUnavailable:
		return prefix + "gateway path unavailable"
	case GatewayTargetDeviceFailedToRespond:
		return prefix + "gateway target device failed to respond"
	}
	return prefix + fmt.Sprintf("exception %v", byte(ex))
}

// Broadcast is the numerical value defined by the modbus specification for sending broadcast request to all slaves on the same bus.
const Broadcast = 0

// Common holds generic information of a modbus command frame.
type Common struct {
	ID byte // ID defines the slave identifier addressed by the command.
	FC byte // FC defines the function code for the command.
}

func (com *Common) Request(cmd Command) Command {
	return func(rx, tx Handler) error {
		if com.FC == 0 || (com.FC&0x80) != 0 {
			return IllegalFunction
		}
		buf := Buffer{com.ID, com.FC}
		return cmd(rx.Hook(buf.Decoder().Hook(Validate(func() error {
			if buf[1] != com.FC {
				return Exception(buf[1] & 0x80)
			}
			return nil
		}))), tx.Hook(buf.Encoder()))
	}
}

func (com *Common) Respond(cmd Command) Command {
	return func(rx, tx Handler) error {
		buf := Buffer{0, 0}
		return cmd(rx.Hook(buf.Decoder().Hook(Validate(func() error {
			com.ID = buf[0]
			com.FC = buf[1]
			return nil
		}))), tx.Hook(buf.Encoder()))
	}
}

// TCP ToDo.
type TCP struct{}

// Register is a helper type.
type Register struct {
	Address uint16 // Address defines the numerical starting point for the register.
	Payload []byte // Payload contains the data value for the Register.
}

// Quantity returns the total number of individual register values contained.
func (r Register) Quantity() uint16 {
	return uint16(len(r.Payload) / 2)
}

// ReadHoldingRegisters is used to read the contents of a contiguous block of holding registers in a remote device.
// The Request PDU specifies the starting register address and the number of registers.
// In the PDU Registers are addressed starting at zero.
// Therefore registers numbered 1-16 are addressed as 0-15.
// The register data in the response message are packed as two bytes per register, with the binary contents right justified within each byte.
// For each register, the first byte contains the high order bits and the second contains the low order bits.
type ReadHoldingRegisters = Register

// Code returns the numerical value of the function code.
func (ReadHoldingRegisters) Code() byte {
	return 0x03
}

func (cmd *ReadHoldingRegisters) Request() Command {
	return func(rx, tx Handler) error {
		if q := len(cmd.Payload); q < 2 || q > 126 || q%2 != 0 {
			return IllegalDataValue
		}
		buf := Buffer{0, 0, 0, 0}
		binary.BigEndian.PutUint16(buf[0:], cmd.Address)
		binary.BigEndian.PutUint16(buf[2:], uint16(len(cmd.Payload))/2)
		if err := tx(buf.Encoder()); err != nil {
			return err
		}
		return rx(buf[:1].Decoder().Hook(Validate(func() error {
			if buf[0] != byte(len(cmd.Payload)) {
				return SlaveDeviceFailure
			}
			return nil
		})).Hook(Buffer(cmd.Payload).Decoder()))
	}
}

func (cmd *ReadHoldingRegisters) Respond() Command {
	return func(rx, tx Handler) error {
		buf := Buffer{0, 0, 0, 0}
		if err := rx(buf.Decoder().Hook(Validate(func() error {
			cmd.Address = binary.BigEndian.Uint16(buf[0:])
			cmd.Payload = make([]byte, 2*binary.BigEndian.Uint16(buf[2:]))
			return nil
		}))); err != nil {
			return err
		}
		return tx(Buffer{byte(len(cmd.Payload))}.Encoder().Hook(Buffer(cmd.Payload).Encoder()))
	}
}

// WriteMultipleRegisters is used to write a block of contiguous registers (1 to 123 registers) in a remote device.
// The requested written values are specified in the request data field.
// Data is packed as two bytes per register.
// The normal response returns the function code, starting address, and quantity of registers written.
type WriteMultipleRegisters Register

// Code returns the numerical value of the function code.
func (WriteMultipleRegisters) Code() byte {
	return 0x10
}

func (cmd *WriteMultipleRegisters) Request() Command {
	return func(rx, tx Handler) error {
		if q := len(cmd.Payload); q < 2 || q > 126 || q%2 != 0 {
			return IllegalDataValue
		}
		buf := Buffer{0, 0, 0, 0, 0}
		binary.BigEndian.PutUint16(buf[0:], cmd.Address)
		binary.BigEndian.PutUint16(buf[2:], uint16(len(cmd.Payload)))
		buf[5] = byte(len(cmd.Payload))
		if err := tx(buf.Encoder().Hook(Buffer(cmd.Payload).Encoder())); err != nil {
			return err
		}
		return rx(buf[:4].Decoder().Hook(Validate(func() error {
			switch {
			case binary.BigEndian.Uint16(buf[0:]) != cmd.Address:
				return SlaveDeviceFailure
			case binary.BigEndian.Uint16(buf[2:]) != uint16(len(cmd.Payload)):
				return SlaveDeviceFailure
			}
			return nil
		})))
	}
}

func (cmd *WriteMultipleRegisters) Respond() Command {
	return func(rx, tx Handler) error {
		buf := Buffer{0, 0, 0, 0, 0}
		var dec Codec
		if err := rx(buf.Decoder().Hook(Validate(func() error {
			cmd.Address = binary.BigEndian.Uint16(buf[0:])
			cmd.Payload = make([]byte, buf[5])
			dec = Buffer(cmd.Payload).Decoder()
			return nil
		})).Hook(dec)); err != nil {
			return err
		}
		return tx(buf[:4].Encoder())
	}
}
