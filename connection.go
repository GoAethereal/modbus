package modbus

import (
	"container/list"
	"net"
	"sync"
	"time"

	"github.com/GoAethereal/cancel"
)

type connection interface {
	ready() bool
	// close stops the connection.
	// All running reads and writes are canceled.
	close()
	// tx sends the given adu to the connected endpoint.
	tx(ctx cancel.Context, adu []byte) (err error)
	// read attaches the given callback function to the connection.
	// The callback will be eventually removed if the context is canceled
	// or immediately if quit=true is returned.
	rx(ctx cancel.Context, callback func(adu []byte, err error) (quit bool)) (done <-chan struct{})
}

type network struct {
	mtx sync.Mutex
	ctx cancel.Signal
	con net.Conn
	buf []byte
	l   list.List
}

func (c *network) ready() bool {
	select {
	case <-c.ctx.Done():
		return false
	default:
		return true
	}
}

func (c *network) close() {
	c.ctx.Cancel()
}

func (c *network) init() (connection, error) {
	go func() {
		c.con.SetReadDeadline(time.Time{})
		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()
		defer c.ctx.Cancel()
		go func() {
			defer wg.Done()
			<-c.ctx.Done()
			c.con.SetReadDeadline(time.Unix(1, 0))
		}()
		var (
			n   int
			err error
		)
		for err == nil {
			n, err = c.con.Read(c.buf)
			c.broadcast(c.buf[:n], err)
		}
	}()
	return c, nil
}

func (c *network) broadcast(adu []byte, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
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

type receiver struct {
	done     chan struct{}
	callback func(adu []byte, err error) (quit bool)
}

func (c *network) tx(ctx cancel.Context, adu []byte) (err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	var wg sync.WaitGroup
	c.con.SetWriteDeadline(time.Time{})
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-done:
		case <-ctx.Done():
			c.con.SetWriteDeadline(time.Unix(1, 0))
		}
	}()
	_, err = c.con.Write(adu)
	close(done)
	wg.Wait()
	return err
}

func (c *network) rx(ctx cancel.Context, callback func(adu []byte, err error) (quit bool)) (done <-chan struct{}) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	r := receiver{done: make(chan struct{}), callback: callback}
	e := c.l.PushFront(r)
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			c.mtx.Lock()
			defer c.mtx.Unlock()
			select {
			case <-done:
			default:
				c.l.Remove(e)
				close(r.done)
			}
		}
	}()
	return r.done
}
