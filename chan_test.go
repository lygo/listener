package listener

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/helloworld/helloworld"
)

func TestChanConnHttp(t *testing.T) {
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


// server is used to implement helloworld.GreeterServer.
type server struct{}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}

func TestChanConnGrpc(t *testing.T) {
	lis, err := ListenChannel(context.Background(),"my-service:0")
	if err != nil {
		t.Fatalf("unexpected error  - %s",err)
	}
	defer lis.Close()

	srv := grpc.NewServer()
	pb.RegisterGreeterServer(srv,new(server))


	start := make(chan struct{})
	go func () {
		close(start)
		srv.Serve(lis)
	}()

	<-start

	conn, err := grpc.DialContext(context.Background(),"my-service:0",
		grpc.WithInsecure(),
		grpc.WithDialer(
		func (addr string, _ time.Duration) (net.Conn,error) {
			return DialChannel(context.Background(),"channel",addr)
		}),
	)

	if err != nil {
		t.Errorf("unexpected error %s",err)
	}

	resp , err := pb.NewGreeterClient(conn).SayHello(context.Background(),&pb.HelloRequest{
		Name:"Aloxa",
	})
	if err != nil {
		t.Errorf("unexpected error %s",err)
	} else {
		if resp.Message != "Hello Aloxa" {
			t.Errorf("messaage resp 'Hello Aloxa' != %s", resp.Message)
		}
	}
}
