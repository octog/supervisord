package main

import "net/http"

type System struct{}

func NewSystem() *System {
	return &System{}
}

func (s *System) ListMethods(r *http.Request, args *struct{}, reply *struct{ Methods []string }) error {
	reply.Methods = xmlCodec.Methods()

	return nil
}
