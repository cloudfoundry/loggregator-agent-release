package v2

import (
	"log"
	"net"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"

	"google.golang.org/grpc"
)

type Server struct {
	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server
	rx      *Receiver
	opts    []grpc.ServerOption
}

func NewServer(addr string, rx *Receiver, opts ...grpc.ServerOption) *Server {
	return &Server{
		addr: addr,
		rx:   rx,
		opts: opts,
	}
}

func (s *Server) Start() {
	var err error
	s.lis, err = net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("grpc bound to: %s", s.lis.Addr())

	s.grpcSrv = grpc.NewServer(s.opts...)
	loggregator_v2.RegisterIngressServer(s.grpcSrv, s.rx)

	if err := s.grpcSrv.Serve(s.lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *Server) Stop() {
	s.grpcSrv.Stop()
	s.lis.Close()
}
