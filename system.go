package main

import (
	XML "encoding/xml"
	"fmt"
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
	// fmt.Printf("RRRR multicall args:%#v\n", args)

	for i := range args.Methods {
		// fmt.Printf("RRRR hello0\n")
		// if !gRPC.HasMethod(args.Methods[i].MethodName) {
		// 	reply.Results = append(reply.Results, gorilla_xml.FaultInvalidMethodName)
		// 	continue
		// }
		codec := gRPC.NewCodec(r)

		rawxml, err := XML.Marshal(args.Methods[i])
		// fmt.Printf("RRRR rawxml:%s\n", rawxml)
		if err != nil {
			reply.Results = append(reply.Results, gorilla_xml.FaultDecode)
			continue
		}
		codecReq := codec.NewRequest(rawxml, err)

		fault := gorilla_xml.FaultInternalError
		errCode, errString, errValue, rpl := gRPC.Serve(r, codecReq)
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

		fmt.Printf("reply:%v\n", rpl)

		reply.Results = append(reply.Results, rpl)
	}

	fmt.Printf("reply results:%#v\n", reply)

	return nil
}
