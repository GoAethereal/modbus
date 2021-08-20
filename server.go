package modbus

import (
	"context"
	"net"
	"sync"
)

// Server is the go implementation of a modbus slave.
// Once serving it will listen for incomming requests and forward them to the modbus.Handler h.
// Generally the intended use is as follows:
//	ctx := context.TODO()
//	opts := modbus.Options{
//		Mode:     "tcp",
//		Kind:     "tcp",
//		Endpoint: "localhost:502",
//	}
//	h := &modbus.Mux{/*define individual handlers*/}
//	s := modbus.Server{}
//
//	log.Fatal(s.Serve(ctx,opts,h))
type Server struct {
	mu sync.Mutex
	f  framer
}

// Serve starts the modbus server and listens for incomming requests.
// The Handler h is called for each inbound message.
// h must be safe for use by multiple go routines.
func (s *Server) Serve(ctx context.Context, opts Options, h Handler) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = opts.Verify(); err != nil {
		return err
	}
	if s.f, err = frame(opts.Mode); err != nil {
		return err
	}
	l, err := net.Listen(opts.Kind, opts.Endpoint)
	if err != nil {
		return err
	}
	var wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		l.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
			conn, err := l.Accept()
			if err != nil {
				continue
			}
			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				c := &network{mu: make(mutex, 2), conn: conn}
				c.mu.unlock()
				s.handle(ctx, c, h)
			}(conn)
		}
	}
}

func (s *Server) handle(ctx context.Context, c connection, h Handler) {
	defer c.close()
	var wg sync.WaitGroup

	_, wait := c.listen(ctx, func(adu []byte, err error) (quit bool) {
		if err != nil {
			return true
		}
		buf := s.f.buffer()
		buf = buf[:copy(buf, adu)]
		wg.Add(1)
		go func(adu []byte) {
			defer wg.Done()
			var res []byte
			var ex Exception
			code, req, err := s.f.decode(adu)

			switch {
			case err != nil:
				return
			case code < 0x80:
				res, ex = h.Handle(ctx, code, req)
			default:
				ex = ExIllegalFunction
			}

			switch {
			case ex != nil:
				code |= 0x80
				res = []byte{ex.Code()}
			case len(res) > 252:
				code |= 0x80
				res = []byte{ExSlaveDeviceFailure.Code()}
			}

			res, _ = s.f.reply(code, res, adu)
			if err := c.write(ctx, res); err != nil {
				return
			}
		}(buf)
		return false
	})

	c.read(ctx, s.f.buffer())
	<-wait
	wg.Wait()
}
