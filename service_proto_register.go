package cucumber

import "google.golang.org/grpc"

// ServiceProtoRegister allows to register Proto Buffer Server implementation to GRPC server
type ServiceProtoRegister interface {
	RegisterProtoServer(*grpc.Server)
}
