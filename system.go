package main

import (
	"net/http"

	gorilla_xml "github.com/AlexStocks/gorilla-xmlrpc/xml"
	"github.com/AlexStocks/supervisord/types"
)

type System struct{}

func NewSystem() *System {
	return &System{}
}

func (s *System) ListMethods(r *http.Request, args *struct{}, reply *struct{ Methods []string }) error {
	reply.Methods = xmlCodec.Methods()

	return nil
}

func (s *System) Multicall(r *http.Request, args *types.MulticallArgs, reply *types.MulticallResults) error {
	for i := range args.Methods {
		method := args.Methods[i]
		codec := gRPC.NewCodec(r)
		method.MethodName = codec.GetMethodName(method.MethodName)
		if len(method.MethodName) == 0 {
			reply.Results = append(reply.Results, gorilla_xml.FaultInvalidMethodName)
			continue
		}

		fault := gorilla_xml.FaultInternalError
		errCode, errString, errValue, rpl := gRPC.ServeApiMethod(r, method.MethodName, method.Params)
		if errCode != 0 || len(errString) != 0 {
			if errCode != 0 {
				fault.Code = errCode
			}
			if len(errString) != 0 {
				fault.String = errString
			}
			reply.Results = append(reply.Results, fault)
			continue
		}

		// Cast the result to error if needed.
		errInter := errValue[0].Interface()
		if errInter != nil {
			fault.String = errInter.(error).Error()
			reply.Results = append(reply.Results, fault)
			continue
		}

		reply.Results = append(reply.Results, rpl.Elem().Interface())
	}

	return nil
}
