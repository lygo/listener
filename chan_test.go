package listener

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestChanConn_Read(t *testing.T) {
    lis, err := ListenChannel(context.Background(),"my-service:0")
    if err != nil {
    	t.Fatalf("unexpected error  - %s",err)
	}
	defer lis.Close()

    mux := new(http.ServeMux)

	mux.HandleFunc("/",func (w http.ResponseWriter, r *http.Request){
		w.Write([]byte(`hello world!`))
	})


    start := make(chan struct{})
    go func () {
    	close(start)
    	http.Serve(lis,mux)
	}()

    <-start

    cli := http.Client{
    	Transport:&http.Transport{
			DialContext: func (ctx context.Context,net, addr string) (net.Conn,error){
				return DialChannel(ctx,"channel",addr)
			},
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	resp, err := cli.Get("http://my-service:0/")
	if err != nil {
		t.Errorf("unexpected error %s",err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status code %d", resp.StatusCode )
	}
}
