package modbus

import (
	"container/list"
	"context"
	"net"
	"sync"
	"time"
)

type connection interface {
	// close stops the connection.
	// All running reads and writes are canceled.
	close() error
	// read continuously reads from the connection
	// and broadcasts incoming data to all attached listeners
	read(ctx context.Context, buf []byte) (err error)
	// write sends the given adu to the connected endpoint.
	write(ctx context.Context, adu []byte) (err error)
	// listen attaches the given callback function to the connection.
	// The callback will be eventually removed if the context is canceled
	// or immediately if quit=true is returned.
	listen(ctx context.Context, callback func(adu []byte, err error) (quit bool)) (cancel context.CancelFunc, done <-chan struct{})
}

type network struct {
	mu   mutex
	l    list.List
	conn net.Conn
}

var _ connection = (&network{})

type receiver struct {
	done     chan struct{}
	callback func(adu []byte, err error) (quit bool)
}

func (c *network) close() error {
	return c.conn.Close()
}

func (c *network) read(ctx context.Context, buf []byte) (err error) {
	c.conn.SetReadDeadline(time.Time{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-done:
		case <-ctx.Done():
			c.conn.SetReadDeadline(time.Unix(1, 0))
		}
	}()
	var n int
	for {
		n, err = c.conn.Read(buf)
		c.broadcast(ctx, buf[:n], err)
		if err != nil {
			close(done)
			wg.Wait()
			return err
		}
	}
}

func (c *network) broadcast(ctx context.Context, adu []byte, err error) {
	if c.mu.lock(ctx) != nil {
		return
	}
	defer c.mu.unlock()
	var n *list.Element
	for e := c.l.Front(); e != nil; e = n {
		n = e.Next()
		r := e.Value.(receiver)
		if r.callback(adu, err) {
			c.l.Remove(e)
			close(r.done)
		}
	}
}

func (c *network) write(ctx context.Context, adu []byte) (err error) {
	if err = c.mu.lock(ctx); err != nil {
		return err
	}
	defer c.mu.unlock()
	var wg sync.WaitGroup
	c.conn.SetWriteDeadline(time.Time{})
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-done:
		case <-ctx.Done():
			c.conn.SetWriteDeadline(time.Unix(1, 0))
		}
	}()
	_, err = c.conn.Write(adu)
	close(done)
	wg.Wait()
	return err
}

func (c *network) listen(ctx context.Context, callback func(adu []byte, err error) (quit bool)) (cancel context.CancelFunc, done <-chan struct{}) {
	if c.mu.lock(ctx) != nil {
		return nil, nil
	}
	defer c.mu.unlock()
	ctx, cancel = context.WithCancel(ctx)
	r := receiver{done: make(chan struct{}), callback: callback}
	e := c.l.PushFront(r)
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			c.mu.lock(context.Background())
			defer c.mu.unlock()
			select {
			case <-done:
			default:
				c.l.Remove(e)
				close(r.done)
			}
		}
	}()
	return cancel, r.done
}
