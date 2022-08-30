package modbus

import (
	"log"
	"net"

	"github.com/GoAethereal/cancel"
)

// Config are used to configure a modbus client or server
type Config struct {
	// Mode defines the communication framing
	// valid modes are:
	//	- tcp
	//	- rtu	(ToDo)
	//	- ascii	(ToDo)
	Mode string
	// Kind specifies the underlying network layer
	// valid kinds are:
	//	- tcp
	//	- udp 		(ToDo)
	//	- serial	(ToDo)
	Kind string
	// Endpoint used for connecting to (client) or listening on (server)
	Endpoint string
	// Unit identifier used
	UnitID byte
}

// Verify validates the modbus.Options, thereby checking for invalid parameter.
// If the options are valid no error (nil) is returned.
func (cfg *Config) Verify() error {
	switch cfg.Mode {
	case "tcp" /*, "rtu", "ascii"*/ :
	default:
		return ErrInvalidParameter
	}

	switch cfg.Kind {
	case "tcp" /*, "udp", "serial"*/ :
	default:
		return ErrInvalidParameter
	}

	return nil
}

// framer creates a new modbus framer from the given configuration.
func (cfg Config) framer(_ cancel.Context) (framer, error) {
	switch cfg.Mode {
	case "tcp":
		return &tcp{unitId: cfg.UnitID}, nil
	}
	return nil, ErrInvalidParameter
}

func (cfg Config) connection(ctx cancel.Context) (connection, error) {
	switch cfg.Kind {
	case "tcp":
		ctx, cancel := cancel.Promote(ctx)
		defer cancel()
		con, err := new(net.Dialer).DialContext(ctx, cfg.Kind, cfg.Endpoint)
		if err != nil {
			log.Println("connection failed")
			return nil, err
		}

		return (&network{con: con, buf: make([]byte, 260)}).init()
	}
	return nil, ErrInvalidParameter
}

// listen creates a new listener on the configured endpoint.
// If successful a acceptor function will be returned.
// The function will block until a new connection is established or an error occurs.
func (cfg Config) listen(ctx cancel.Context) (fn func() (connection, error), err error) {
	switch cfg.Kind {
	case "tcp":
		l, err := net.Listen(cfg.Kind, cfg.Endpoint)
		if err != nil {
			return nil, err
		}
		// start the watch-dog which will stop the listener when the context is canceled
		go func() {
			<-ctx.Done()
			l.Close()
		}()
		fn = func() (connection, error) {
			con, err := l.Accept()
			if err != nil {
				return nil, err
			}
			return (&network{con: con, buf: make([]byte, 256)}).init()
		}

	}
	return fn, nil
}
