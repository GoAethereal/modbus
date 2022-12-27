package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/GoAethereal/cancel"
)

// Client is the go implementation of a modbus master.
// Generally the intended use is as follows:
//
//	c := modbus.Client{Config: modbus.Config{
//		Mode:     "tcp",
//		Kind:     "tcp",
//		Endpoint: "localhost:502",
//	}}
//	defer c.Disconnect()
//
//	//use the client`s read/write methods like c.ReadCoils, etc
type Client struct {
	Config
	mtx sync.Mutex
	c   connection
	f   framer
}

func (c *Client) Ready() bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.c != nil {
		return c.c.ready()
	}
	return false
}

// Disconnect shuts down the connection.
// All running requests will be canceled as a result.
func (c *Client) Disconnect() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.c != nil {
		c.c.close()
	}
}

func (c *Client) init(ctx cancel.Context) (_ connection, _ framer, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.c == nil || !c.c.ready() {
		if c.c, err = c.Config.connection(ctx); err != nil {
			return nil, nil, err
		}
	}
	if c.f == nil {
		if c.f, err = c.Config.framer(ctx); err != nil {
			return nil, nil, err
		}
	}
	return c.c, c.f, nil
}

// Request encodes the request into a valid application data unit and sends it to the clients endpoint.
// Only function codes below 0x80 are accepted.
// The method will return a nil response and an error if something went wrong.
func (c *Client) Request(ctx cancel.Context, uid, code byte, req []byte) (res []byte, err error) {
	if code == 0 || code >= 0x80 {
		return nil, IllegalFunction
	}

	con, f, err := c.init(ctx)
	if err != nil {
		return nil, err
	}

	if req, err = f.encode(uid, code, req); err != nil {
		return nil, err
	}

	sig := cancel.New().Propagate(ctx)
	defer sig.Cancel()

	wait := con.rx(sig, func(adu []byte, er error) (quit bool) {
		if er != nil {
			res, err = nil, er
			return true
		}
		e := f.verify(req, adu)
		switch e {
		case nil:
			//needs check for exceptions
			_, _, res, err = f.decode(req[:copy(req[:cap(req)], adu)])
		case ErrMismatchedTransactionId:
			return false
		default:
			res, err = nil, e
		}
		return true
	})

	if err := con.tx(ctx, req); err != nil {
		sig.Cancel()
		<-wait
		return nil, err
	}

	<-wait

	select {
	case <-ctx.Done():
		return nil, context.Canceled
	default:
		return res, err
	}
}

// ReadCoils requests 1 to 2000 (quantity) contiguous coil states, starting from address.
// On success returns a bool slice with size of quantity where false=OFF and true=ON.
func (c *Client) ReadCoils(ctx cancel.Context, uid byte, address, quantity uint16) (status []bool, err error) {
	if ex := boundCheck(address, quantity, 2000); ex != 0 {
		return nil, ex
	}
	res, err := c.Request(ctx, uid, 0x01, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(byteCount(quantity)) || int(res[0]) != len(res)-1:
		return nil, SlaveDeviceFailure
	}
	return bytesToBools(quantity, res[1:]), nil
}

// ReadDiscreteInputs requests 1 to 2000 (quantity) contiguous discrete inputs, starting from address.
// On success returns a bool slice with size of quantity where false=OFF and true=ON.
func (c *Client) ReadDiscreteInputs(ctx cancel.Context, uid byte, address, quantity uint16) (status []bool, err error) {
	if ex := boundCheck(address, quantity, 2000); ex != 0 {
		return nil, ex
	}
	res, err := c.Request(ctx, uid, 0x02, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(byteCount(quantity)) || int(res[0]) != len(res)-1:
		return nil, SlaveDeviceFailure
	}
	return bytesToBools(quantity, res[1:]), nil
}

// ReadHoldingRegisters reads from 1 to 125 (quantity) contiguous holding registers starting at address.
// On success returns a byte slice with the response data which is 2*quantity in length.
func (c *Client) ReadHoldingRegisters(ctx cancel.Context, uid byte, address, quantity uint16) (values []byte, err error) {
	if ex := boundCheck(address, quantity, 125); ex != 0 {
		return nil, ex
	}
	res, err := c.Request(ctx, uid, 0x03, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(quantity)*2 || int(res[0]) != len(res)-1:
		return nil, SlaveDeviceFailure
	}
	return res[1:], nil
}

// ReadInputRegisters reads from 1 to 125 (quantity) contiguous input registers starting at address.
// On success returns a byte slice with the response data which is 2*quantity in length.
func (c *Client) ReadInputRegisters(ctx cancel.Context, uid byte, address, quantity uint16) (values []byte, err error) {
	if ex := boundCheck(address, quantity, 125); ex != 0 {
		return nil, ex
	}
	res, err := c.Request(ctx, uid, 0x04, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(quantity)*2 || int(res[0]) != len(res)-1:
		return nil, SlaveDeviceFailure
	}
	return res[1:], nil
}

// WriteSingleCoil sets the output of the coil at address to ON=true or OFF=false.
func (c *Client) WriteSingleCoil(ctx cancel.Context, uid byte, address uint16, status bool) (err error) {
	res, err := c.Request(ctx, uid, 0x05, put(4, address, status))
	switch {
	case err != nil:
		return err
	case len(res) != 4 || binary.BigEndian.Uint16(res) != address:
		return SlaveDeviceFailure
	}
	return nil
}

// WriteSingleRegister writes value to a single holding register at address.
func (c *Client) WriteSingleRegister(ctx cancel.Context, uid byte, address, value uint16) (err error) {
	res, err := c.Request(ctx, uid, 0x06, put(4, address, value))
	switch {
	case err != nil:
		return err
	case len(res) != 4 || binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != value:
		return SlaveDeviceFailure
	}
	return nil
}

// WriteMultipleCoils sets the state of all coils starting at address to the value of status, where false=OFF and true=ON.
// Status needs to be of length 1 to 1968.
func (c *Client) WriteMultipleCoils(ctx cancel.Context, uid byte, address uint16, status ...bool) (err error) {
	quantity := uint16(len(status))
	if ex := boundCheck(address, quantity, 1968); ex != 0 {
		return ex
	}
	res, err := c.Request(ctx, uid, 0x0F, put(5+byteCount(quantity), address, quantity, byte(byteCount(quantity)), status))
	switch {
	case err != nil:
		return err
	case binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != quantity:
		return SlaveDeviceFailure
	}
	return nil
}

// WriteMultipleRegisters writes the values to the holding registers at address.
// Values must be a multiple of 2 and in the range of 2 to 246
func (c *Client) WriteMultipleRegisters(ctx cancel.Context, uid byte, address uint16, values []byte) (err error) {
	l := len(values)
	if l%2 != 0 {
		return IllegalDataValue
	}
	quantity := uint16(l) / 2
	if ex := boundCheck(address, quantity, 246); ex != 0 {
		return ex
	}
	res, err := c.Request(ctx, uid, 0x10, put(5+l, address, quantity, byte(l), values))
	switch {
	case err != nil:
		return err
	case binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != quantity:
		return SlaveDeviceFailure
	}
	return nil
}

// ReadWriteMultipleRegisters reads a contiguous block of holding registers (rQuantity) from rAddress.
// Also the values are written at wAddress.
func (c *Client) ReadWriteMultipleRegisters(ctx cancel.Context, uid byte, rAddress, rQuantity, wAddress uint16, values []byte) (res []byte, err error) {
	l := len(values)
	if l%2 != 0 {
		return nil, IllegalDataValue
	}
	wQuantity := uint16(l) / 2
	if ex := boundCheck(rAddress, rQuantity, 125); ex != 0 {
		return nil, ex
	}
	if ex := boundCheck(wAddress, wQuantity, 121); ex != 0 {
		return nil, ex
	}
	res, err = c.Request(ctx, uid, 0x17, put(9+l, rAddress, rQuantity, wAddress, wQuantity, byte(l), values))
	switch {
	case err != nil:
		return nil, err
	case 2*rQuantity != uint16(res[0]):
		fmt.Println(res)
		return nil, SlaveDeviceFailure
	}
	return res[1:], nil
}
