package modbus

// Options are used to configure a modbus client or server
type Options struct {
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
}

// Verify validates the modbus.Options, thereby checking for invalid parameter.
// If the options are valid no error (nil) is returned.
func (o *Options) Verify() error {
	switch o.Mode {
	case "tcp" /*, "rtu", "ascii"*/ :
	default:
		return ErrInvalidParameter
	}

	switch o.Kind {
	case "tcp" /*, "udp", "serial"*/ :
	default:
		return ErrInvalidParameter
	}

	return nil
}
