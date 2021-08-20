package modbus

import (
	"context"
	"encoding/binary"
)

// Client is the go implementation of a modbus master.
// Generally the intended use is as follows:
//	ctx := context.TODO()
//	opts := modbus.Options{
//		Mode:     "tcp",
//		Kind:     "tcp",
//		Endpoint: "localhost:502",
//	}
//	c := modbus.Client{}
//
//	if err := c.Connect(ctx, opts); err != nil {
//		log.Fatal(err)
//	}
//	defer c.Disconnect()
//
//	//use the client`s read/write methods like c.ReadCoils, etc
type Client struct {
	f framer
	c connection
}

// Connect initializes the underlying connection and payload mode.
// If all given options are valid the enpoint will be dialed in.
func (c *Client) Connect(ctx context.Context, opts Options) (err error) {
	c.Disconnect()
	if err = opts.Verify(); err != nil {
		return err
	}
	if c.f, err = frame(opts.Mode); err != nil {
		return err
	}
	if c.c, err = dial(ctx, opts.Kind, opts.Endpoint); err != nil {
		return err
	}
	go c.c.read(context.Background(), c.f.buffer())
	return nil
}

// Disconnect shuts down the connection.
// All running requests will be canceled as a result.
func (c *Client) Disconnect() error {
	if c.c != nil {
		return c.c.close()
	}
	return nil
}

// Request encodes the request into a valid application data unit and sends it to the clients endpoint.
// Only function codes below 0x80 are accepted.
// The method will return a nil response and an error if something went wrong.
func (c *Client) Request(ctx context.Context, code byte, req []byte) (res []byte, err error) {
	if code == 0 || code >= 0x80 {
		return nil, ExIllegalFunction
	}
	if req, err = c.f.encode(code, req); err != nil {
		return nil, err
	}

	cancel, wait := c.c.listen(ctx, func(adu []byte, er error) (quit bool) {
		if er != nil {
			res, err = nil, er
			return true
		}
		e := c.f.verify(req, adu)
		switch e {
		case nil:
			//needs check for exceptions
			_, res, err = c.f.decode(req[:copy(req[:cap(req)], adu)])
		case ErrMissmatchedTransactionId:
			return false
		default:
			res, err = nil, e
		}
		return true
	})

	if err := c.c.write(ctx, req); err != nil {
		cancel()
		<-wait
		return nil, err
	}

	<-wait

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return res, err
	}
}

// ReadCoils requests 1 to 2000 (quantity) contiguous coil states, starting from address.
// On success returns a bool slice with size of quantity where false=OFF and true=ON.
func (c *Client) ReadCoils(ctx context.Context, address, quantity uint16) (status []bool, err error) {
	switch {
	case quantity < 1 || quantity > 2000:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	res, err := c.Request(ctx, 0x01, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(byteCount(quantity)) || int(res[0]) != len(res)-1:
		return nil, ExSlaveDeviceFailure
	}
	return bytesToBools(quantity, res[1:]), nil
}

// ReadDiscreteInputs requests 1 to 2000 (quantity) contiguous discrete inputs, starting from address.
// On success returns a bool slice with size of quantity where false=OFF and true=ON.
func (c *Client) ReadDiscreteInputs(ctx context.Context, address, quantity uint16) (status []bool, err error) {
	switch {
	case quantity < 1 || quantity > 2000:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	res, err := c.Request(ctx, 0x02, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(byteCount(quantity)) || int(res[0]) != len(res)-1:
		return nil, ExSlaveDeviceFailure
	}
	return bytesToBools(quantity, res[1:]), nil
}

// ReadHoldingRegisters reads from 1 to 125 (quantity) contiguous holding registers starting at address.
// On success returns a byte slice with the response data which is 2*quantity in length.
func (c *Client) ReadHoldingRegisters(ctx context.Context, address, quantity uint16) (values []byte, err error) {
	switch {
	case quantity < 1 || quantity > 125:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	res, err := c.Request(ctx, 0x03, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(quantity)*2 || int(res[0]) != len(res)-1:
		return nil, ExSlaveDeviceFailure
	}
	return res[1:], nil
}

// ReadInputRegisters reads from 1 to 125 (quantity) contiguous input registers starting at address.
// On success returns a byte slice with the response data which is 2*quantity in length.
func (c *Client) ReadInputRegisters(ctx context.Context, address, quantity uint16) (values []byte, err error) {
	switch {
	case quantity < 1 || quantity > 125:
		return nil, ExIllegalDataValue
	case int(address)+int(quantity) > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	res, err := c.Request(ctx, 0x04, put(4, address, quantity))
	switch {
	case err != nil:
		return nil, err
	case len(res) != 1+int(quantity)*2 || int(res[0]) != len(res)-1:
		return nil, ExSlaveDeviceFailure
	}
	return res[1:], nil
}

// WriteSingleCoil sets the output of the coil at address to ON=true or OFF=false.
func (c *Client) WriteSingleCoil(ctx context.Context, address uint16, status bool) (err error) {
	res, err := c.Request(ctx, 0x05, put(4, address, status))
	switch {
	case err != nil:
		return err
	case len(res) != 4 || binary.BigEndian.Uint16(res) != address:
		return ExSlaveDeviceFailure
	}
	return nil
}

// WriteSingleRegister writes value to a single holding register at address.
func (c *Client) WriteSingleRegister(ctx context.Context, address, value uint16) (err error) {
	res, err := c.Request(ctx, 0x06, put(4, address, value))
	switch {
	case err != nil:
		return err
	case len(res) != 4 || binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != value:
		return ExSlaveDeviceFailure
	}
	return nil
}

// WriteMultipleCoils sets the state of all coils starting at address to the value of status, where false=OFF and true=ON.
// Status needs to be of length 1 to 1968.
func (c *Client) WriteMultipleCoils(ctx context.Context, address uint16, status ...bool) (err error) {
	l := len(status)
	switch {
	case l < 1 || l > 1968:
		return ExIllegalDataValue
	case int(address)+l > 0xFFFF:
		return ExIllegalDataAddress
	}
	quantity := uint16(l)
	res, err := c.Request(ctx, 0x0F, put(5+byteCount(quantity), address, quantity, byte(byteCount(quantity)), status))
	switch {
	case err != nil:
		return err
	case binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != quantity:
		return ExSlaveDeviceFailure
	}
	return nil
}

// WriteMultipleRegisters writes the values to the holding registers at address.
// Values must be a multiple of 2 and in the range of 2 to 246
func (c *Client) WriteMultipleRegisters(ctx context.Context, address uint16, values []byte) (err error) {
	l := len(values)
	switch {
	case l%2 != 0 || l == 0 || l > 246:
		return ExIllegalDataValue
	case int(address)+l > 0xFFFF:
		return ExIllegalDataAddress
	}
	quantity := uint16(l)
	res, err := c.Request(ctx, 0x10, put(5+l, address, quantity, byte(l), values))
	switch {
	case err != nil:
		return err
	case binary.BigEndian.Uint16(res) != address || binary.BigEndian.Uint16(res[2:]) != quantity:
		return ExSlaveDeviceFailure
	}
	return nil
}

// ReadWriteMultipleRegisters reads a contiguous block of holding registers (rQuantity) from rAddress.
// Also the values are written at wAddress.
func (c *Client) ReadWriteMultipleRegisters(ctx context.Context, rAddress, rQuantity, wAddress uint16, values []byte) (res []byte, err error) {
	l := len(values)
	switch {
	case rQuantity < 1 || rQuantity > 125 || l%2 != 0 || l == 0 || l/2 > 121:
		return nil, ExIllegalDataValue
	case int(rAddress)+int(rQuantity) > 0xFFFF || int(wAddress)+l/2 > 0xFFFF:
		return nil, ExIllegalDataAddress
	}
	wQuantity := uint16(l) / 2
	res, err = c.Request(ctx, 0x17, put(9+l, rAddress, rQuantity, wAddress, wQuantity, byte(l), values))
	switch {
	case err != nil:
		return nil, err
	case rQuantity != 2*uint16(res[0]):
		return nil, ExSlaveDeviceFailure
	}
	return res[1:], nil
}
