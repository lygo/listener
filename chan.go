package listener

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)


var (
	dnsOfChannels = map[string]*ChanListener{}
	dnsLock  = new(sync.RWMutex)
)


func registryChannelListener(lis *ChanListener) error {
	dnsLock.Lock()
	defer dnsLock.Unlock()

	addr := lis.Addr().String()
	_, ok := dnsOfChannels[addr]
	if ok {
		return fmt.Errorf("cannot bind to address %s: already listen",addr)
	}

	dnsOfChannels[addr] = lis
	return nil
}

func unregistryChannelListener(addr string)  {
	dnsLock.Lock()
	defer dnsLock.Unlock()

	delete(dnsOfChannels,addr)
}

func getChannelListener(addr string) (lis *ChanListener, ok bool) {
	dnsLock.RLock()
	defer dnsLock.RUnlock()

	lis, ok = dnsOfChannels[addr]
	return
}


type ChanAddr string

func (c ChanAddr) Network() string {
	return `channel`
}

func (c ChanAddr) String() string {
	return string(c)
}

func DialChannel(ctx context.Context,network, address string)  (*ChanConn,error) {
	if ctx == nil {
		panic("nil context")
	}

	if network != "channel" {
		return nil, &net.OpError{
			Op: "dial",
			Net:"channel",
			Addr:ChanAddr(address),
			Err: fmt.Errorf("unexpected network %s",network),
		}
	}

	listener , ok := getChannelListener(address)
	if !ok {
		return nil, &net.OpError{
			Op: "dial",
			Net:"channel",
			Addr:ChanAddr(address),
			Err: fmt.Errorf("cannot resolve address %s",address),
		}
	}

	conn := &ChanConn{
		ctx:ctx,
		raddr:address,
		readBuff: new(bytes.Buffer),
		read:make(chan []byte,0),
		write:make(chan []byte,1),
	}
	err := listener.send(conn)
	if err != nil {
		return nil, err
	}

	return conn,nil
}

func ListenChannel(ctx context.Context, address string) (*ChanListener,error) {
	lis := &ChanListener{
		ctx: ctx,
		once: new(sync.Once),
		addr: ChanAddr(address),
		conns: make(chan *ChanConn,1),
	}

	err := registryChannelListener(lis)
	if err != nil {
		return nil, err
	}

	return lis,nil
}

type ChanConn struct {
	raddr string
	ctx context.Context
	readBuff *bytes.Buffer
	read chan []byte
	write chan []byte
}

func (c ChanConn) Read(b []byte) (n int, err error) {

	if c.readBuff.Len() > 0  {
		n, err = c.readBuff.Read(b)
		return
	}
	select {
	case rb, ok  := <-c.read:
		if !ok {
			return 0, io.EOF
		}
		if c.readBuff.Len() == 0 {
			n, err = c.readBuff.Write(rb)
			if err != nil {
				return
			}
		}
		n, err = c.readBuff.Read(b)
		return
	case <-c.ctx.Done():
		err = c.ctx.Err()
		return
	}
}

func (c ChanConn) Write(b []byte) (n int, err error) {
	select {
	case c.write <- b:
		return len(b), nil
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	}
}

func (c ChanConn) Close() error {
	close(c.read)
	close(c.write)
	return nil
}

func (c ChanConn) LocalAddr() net.Addr {
	return ChanAddr(fmt.Sprintf("l%p",c.write))
}

func (c ChanConn) RemoteAddr() net.Addr {
	return ChanAddr(c.raddr)
}

func (ChanConn) SetDeadline(t time.Time) error {
	//TODO: implement
	//println("call SetDeadline",t.String())
	return nil
}

func (ChanConn) SetReadDeadline(t time.Time) error {
	//TODO: implement
	//println("call SetReadDeadline",t.String())
	return nil
}

func (ChanConn) SetWriteDeadline(t time.Time) error {
	//TODO: implement
	//println("call SetWriteDeadline",t.String())
	return nil
}

type ChanListener struct {
	once *sync.Once
	ctx context.Context
	addr ChanAddr
	conns chan *ChanConn
}

func (c ChanListener) Accept() (net.Conn, error) {
	conn, ok := <- c.conns
	if !ok {
		return nil, io.EOF
	}

	lisConn := &ChanConn{
		ctx: conn.ctx,
		raddr: conn.raddr,
		readBuff: new(bytes.Buffer),
		read:make(chan []byte,1),
		write:make(chan []byte,0),
	}

	p := <- conn.write
	lisConn.read <- p

	start := make(chan struct{})
	go func () {
		close(start)
		select {
		case d, ok  := <- lisConn.write:
			if !ok {
				return
			}
			conn.read <- d
		case <- conn.ctx.Done():
			return
		}
	}()

	<-start

	return lisConn, nil
}

func (c ChanListener) send(conn *ChanConn) error {
	select {
	case c.conns <- conn:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	case <-conn.ctx.Done():
		return conn.ctx.Err()
	}
}

func (c ChanListener) Close() error {
	c.once.Do(func () {
		unregistryChannelListener(c.Addr().String())
		close(c.conns)
	})

	return nil
}

func (c ChanListener) Addr() net.Addr {
	return c.addr
}

