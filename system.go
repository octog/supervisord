package main

import (
	"fmt"
	"net/http"

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
	fmt.Printf("multicall args:%#v\n", args)

	// for i := range args.Methods {
	// 	method := args.Methods[i]
	// }

	return nil
}
