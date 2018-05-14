package listener

import (
	"bytes"
	"context"
	"fmt"
	"io"
	golog "log"
	"net"
	"sync"
	"time"
)


var (
	debugNET = true
)

func log(args ...interface{}) {
	if debugNET {
		golog.Println(args...)
	}
}


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
		prefix: `client`,
		once: new(sync.Once),
		ctx:ctx,
		raddr:address,
		readBuff: new(bytes.Buffer),
		read:make(chan []byte,0),
		write:make(chan []byte,0),
		accepted:make(chan struct{},0),
	}
	err := listener.send(conn)
	if err != nil {
		return nil, err
	}

	<-conn.accepted

	return conn,nil
}

func ListenChannel(ctx context.Context, address string) (*ChanListener,error) {
	lis := &ChanListener{
		ctx: ctx,
		once: new(sync.Once),
		addr: ChanAddr(address),
		conns: make(chan *ChanConn,0),
	}

	err := registryChannelListener(lis)
	if err != nil {
		return nil, err
	}

	return lis,nil
}

type ChanConn struct {
	prefix string
	once *sync.Once
	raddr string
	ctx context.Context
	readBuff *bytes.Buffer
	read chan []byte
	write chan []byte
	accepted chan struct{}
	closed bool
}

func (c ChanConn) Read(b []byte) (n int, err error) {
	defer func () {
		log(`{`,c.prefix,`}`,"READ \nERR",fmt.Sprint(err),"\n", fmt.Sprintf("%s",b))
	}()

	if c.closed {
		return 0,io.EOF
	}

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
	defer func () {
		log(`{`,c.prefix,`}`,"WRITE ","\n","ERR",fmt.Sprint(err), "\n",fmt.Sprintf("%s",b))
	}()

	if c.closed {
		return 0, io.EOF
	}

	select {
	case c.write <- b:
		return len(b), nil
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	}
}

func (c *ChanConn) Close() error {
	c.once.Do(func () {
		log("{",c.prefix,"} closed")
		c.closed = true
		close(c.read)
		close(c.write)
	})
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
	log("{ server } <-")

	lisConn := &ChanConn{
		prefix: `server`,
		ctx: conn.ctx,
		once: new(sync.Once),
		raddr: conn.raddr,
		readBuff: new(bytes.Buffer),
		read:make(chan []byte,0),
		write:make(chan []byte,0),
	}

	start := make(chan struct{})
	go func () {
		close(start)
		for {
			select {
			case d, ok  := <- conn.write:
				if !ok {
					log("client connection was closed")
					return
				}
				if !lisConn.closed {
					lisConn.read <- d
				}
				log("{ client } >> { server }")
			case d, ok  := <- lisConn.write:
				if !ok {
					log("server connection was closed")
					return
				}
				if !conn.closed {
					conn.read <- d
					log("{ server } >> { client }")
				}
			case <- lisConn.ctx.Done():
				return
			case <- conn.ctx.Done():
				return
			}
		}
	}()
	<- start
	close(conn.accepted)
	return lisConn, nil
}

func (c ChanListener) send(conn *ChanConn) error {
	select {
	case c.conns <- conn:
		log("{ client } ->")
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

